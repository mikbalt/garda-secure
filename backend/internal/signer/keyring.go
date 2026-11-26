package signer

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"time"
)

// Keyring entry kinds.
const (
	KindGenesis  = "genesis"
	KindRotation = "rotation"
	KindNewEpoch = "new_epoch"
)

// KeyringEntry is one row of the keyring table.
type KeyringEntry struct {
	ID         int64
	KeyID      string
	Epoch      int
	Kind       string
	PublicKey  []byte
	PrevKeyID  string
	Reason     string
	CreatedAt  time.Time
	NewKeySig  []byte
	PrevKeySig []byte
}

func (e KeyringEntry) payload() []byte {
	return KeyPayload(e.Kind, e.Epoch, e.KeyID, e.PublicKey, e.PrevKeyID, e.CreatedAt)
}

func loadKeyring(ctx context.Context, db querier) ([]KeyringEntry, error) {
	rows, err := db.Query(ctx, `SELECT id, key_id, epoch, kind, public_key, COALESCE(prev_key_id, ''), reason, created_at, new_key_sig, prev_key_sig
		FROM keyring ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("load keyring: %w", err)
	}
	defer rows.Close()
	var out []KeyringEntry
	for rows.Next() {
		var e KeyringEntry
		if err := rows.Scan(&e.ID, &e.KeyID, &e.Epoch, &e.Kind, &e.PublicKey, &e.PrevKeyID, &e.Reason, &e.CreatedAt, &e.NewKeySig, &e.PrevKeySig); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func insertKeyring(ctx context.Context, db querier, e KeyringEntry) error {
	_, err := db.Exec(ctx, `INSERT INTO keyring (key_id, epoch, kind, public_key, prev_key_id, reason, created_at, new_key_sig, prev_key_sig)
		VALUES ($1, $2, $3, $4, NULLIF($5, ''), $6, $7, $8, $9)`,
		e.KeyID, e.Epoch, e.Kind, e.PublicKey, e.PrevKeyID, e.Reason, e.CreatedAt, e.NewKeySig, e.PrevKeySig)
	return err
}

func newEntry(kind string, epoch int, key ed25519.PrivateKey, prev, reason string, now time.Time) KeyringEntry {
	pub := pubOf(key)
	e := KeyringEntry{KeyID: KeyID(pub), Epoch: epoch, Kind: kind, PublicKey: pub, PrevKeyID: prev,
		Reason: reason, CreatedAt: now.UTC().Truncate(time.Microsecond)}
	e.NewKeySig = ed25519.Sign(key, e.payload())
	return e
}

// Bootstrap makes sure the signer's key is the current key of the keyring. An
// empty keyring gets the genesis entry (epoch 1). A key that is not the
// current one is refused: rotating or starting a new epoch is an explicit
// operator step (Rotate, NewEpoch), never done silently.
func (s *Signer) Bootstrap(ctx context.Context) error {
	ring, err := loadKeyring(ctx, s.DB)
	if err != nil {
		return err
	}
	id := KeyID(pubOf(s.Key))
	if len(ring) == 0 {
		return insertKeyring(ctx, s.DB, newEntry(KindGenesis, 1, s.Key, "", "", s.Now()))
	}
	if cur := ring[len(ring)-1]; cur.KeyID != id {
		return fmt.Errorf("%w: key %s is not the current key (%s); rotate or start a new epoch first", ErrKeyNotInRing, id, cur.KeyID)
	}
	return nil
}

// Rotate adds newKey to the current epoch. Both keys sign the entry: the new
// key proves possession, the old key hands over. oldKey must be the current key.
func Rotate(ctx context.Context, db DB, oldKey, newKey ed25519.PrivateKey, now time.Time) error {
	ring, err := loadKeyring(ctx, db)
	if err != nil {
		return err
	}
	if len(ring) == 0 {
		return errors.New("rotate: keyring is empty, start the signer once to create the genesis key")
	}
	cur := ring[len(ring)-1]
	if cur.KeyID != KeyID(pubOf(oldKey)) {
		return fmt.Errorf("%w: old key %s is not the current key (%s)", ErrKeyNotInRing, KeyID(pubOf(oldKey)), cur.KeyID)
	}
	e := newEntry(KindRotation, cur.Epoch, newKey, cur.KeyID, "", now)
	e.PrevKeySig = ed25519.Sign(oldKey, e.payload())
	return insertKeyring(ctx, db, e)
}

// NewEpoch starts epoch n+1 with newKey after a key was lost or compromised.
// The old key does not sign: it cannot be trusted (or is gone). Clients must
// pin the new key's fingerprint out of band before they trust new heads.
func NewEpoch(ctx context.Context, db DB, newKey ed25519.PrivateKey, reason string, now time.Time) error {
	ring, err := loadKeyring(ctx, db)
	if err != nil {
		return err
	}
	if len(ring) == 0 {
		return errors.New("new epoch: keyring is empty, start the signer once to create the genesis key")
	}
	cur := ring[len(ring)-1]
	if cur.KeyID == KeyID(pubOf(newKey)) {
		return errors.New("new epoch: the new key is the current key")
	}
	return insertKeyring(ctx, db, newEntry(KindNewEpoch, cur.Epoch+1, newKey, cur.KeyID, reason, now))
}

// keyTrust is what a verifier concluded about one key of the keyring.
type keyTrust struct {
	entry     KeyringEntry
	trusted   bool
	why       string    // set when not trusted
	retiredAt time.Time // zero: current key; heads signed after it are refused
}

// verifyKeyring decides which keys to trust. Roots (genesis, new_epoch) are
// trusted when their fingerprint is pinned; with no pins at all the genesis
// entry is trusted on first use. A rotation is trusted when both signatures
// check out and the key it replaces is trusted.
func verifyKeyring(ring []KeyringEntry, pins []string) map[string]*keyTrust {
	pinned := map[string]bool{}
	for _, p := range pins {
		pinned[p] = true
	}
	// A handover counts only when both signatures check out. A key that more
	// than one such handover leads away from is a fork: the history of who
	// succeeded whom is ambiguous, so none of its successors is trusted
	// (unless a client pinned one explicitly).
	byKey := map[string]KeyringEntry{}
	for _, e := range ring {
		byKey[e.KeyID] = e
	}
	successors := map[string]int{}
	for _, e := range ring {
		if e.Kind != KindRotation {
			continue
		}
		if prev, ok := byKey[e.PrevKeyID]; ok && KeyID(ed25519.PublicKey(e.PublicKey)) == e.KeyID &&
			verify(ed25519.PublicKey(e.PublicKey), e.payload(), e.NewKeySig) &&
			verify(ed25519.PublicKey(prev.PublicKey), e.payload(), e.PrevKeySig) {
			successors[e.PrevKeyID]++
		}
	}
	out := map[string]*keyTrust{}
	maxEpoch := 0
	for i, e := range ring {
		kt := &keyTrust{entry: e}
		out[e.KeyID] = kt
		pub := ed25519.PublicKey(e.PublicKey)
		switch {
		case KeyID(pub) != e.KeyID:
			kt.why = "key id is not the fingerprint of the public key"
		case !verify(pub, e.payload(), e.NewKeySig):
			kt.why = "keyring entry is not signed by its own key"
		case e.Epoch < maxEpoch || (e.Kind != KindRotation && e.Epoch == maxEpoch && i > 0):
			kt.why = "epoch does not advance"
		case pinned[e.KeyID]:
			kt.trusted = true // a fingerprint the client holds out of band
		case e.Kind == KindRotation:
			prev := out[e.PrevKeyID]
			switch {
			case prev == nil || !prev.trusted:
				kt.why = "not reachable from a pinned key (rotation from a key that is not trusted)"
			case successors[e.PrevKeyID] > 1:
				kt.why = fmt.Sprintf("forked handover: %s has more than one successor", e.PrevKeyID)
			case prev.entry.Epoch != e.Epoch:
				kt.why = "rotation changes the epoch"
			case !verify(ed25519.PublicKey(prev.entry.PublicKey), e.payload(), e.PrevKeySig):
				kt.why = "rotation is not signed by the previous key"
			default:
				kt.trusted = true
			}
		case len(pins) == 0 && e.Kind == KindGenesis && i == 0:
			kt.trusted = true // trust on first use, only when the client pinned nothing
		default:
			kt.why = fmt.Sprintf("epoch %d key is not pinned: re-pin %s out of band", e.Epoch, e.KeyID)
		}
		if e.Epoch > maxEpoch {
			maxEpoch = e.Epoch
		}
	}
	// A key stops being valid for new heads when its successor appears.
	for _, e := range ring {
		for _, k := range out {
			if k.entry.KeyID == e.KeyID || !k.retiredAt.IsZero() {
				continue
			}
			if (e.Kind == KindRotation && e.PrevKeyID == k.entry.KeyID) || (e.Kind == KindNewEpoch && e.Epoch > k.entry.Epoch) {
				k.retiredAt = e.CreatedAt
			}
		}
	}
	return out
}
