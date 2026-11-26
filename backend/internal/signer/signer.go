// Package signer anchors the per-site record chain heads outside the app
// (TAQ-56, D-071). The signer process runs in its own container with its own
// key and a database role that can only read the chain tables and append to
// signed_heads and keyring. It signs {site, seq, entry_hash, time, key_id} and
// refuses to go backwards or to sign a different hash for a seq it has signed.
package signer

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"hash/fnv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Why a head was signed.
const (
	ReasonHourly   = "hourly"
	ReasonDayEnd   = "day_end"
	ReasonOnDemand = "on_demand"
)

var (
	ErrLowerSeq      = errors.New("signer: head is lower than an already signed head")
	ErrConflict      = errors.New("signer: different hash at an already signed seq")
	ErrPastRewritten = errors.New("signer: chain no longer matches the last signed head")
	ErrBrokenLinks   = errors.New("signer: chain links do not hold since the last signed head")
	ErrNoChain       = errors.New("signer: site has no chain")
	ErrKeyNotInRing  = errors.New("signer: key is not the current key of the keyring")
	ErrHeadsLost     = errors.New("signer: signed heads recorded on the signer volume are missing from the database")
)

// querier is satisfied by *pgxpool.Pool and pgx.Tx (same shape as repository.DBTX,
// which this package must not import: repository calls VerifyHeads).
type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// DB is a pool: the signer needs transactions.
type DB interface {
	querier
	Begin(ctx context.Context) (pgx.Tx, error)
}

// SignedHead is one row of signed_heads.
type SignedHead struct {
	ID        int64
	SiteID    string
	Seq       int64
	EntryHash []byte
	SignedAt  time.Time
	Reason    string
	KeyID     string
	Signature []byte
}

// Signer signs chain heads with Key. State is its own record of what it signed.
type Signer struct {
	DB    DB
	Key   ed25519.PrivateKey
	Now   func() time.Time
	State *HighWater
}

// New opens the signer's high-water file at statePath (on the signer's volume).
func New(db DB, key ed25519.PrivateKey, statePath string) (*Signer, error) {
	hw, err := LoadHighWater(statePath)
	if err != nil {
		return nil, err
	}
	return &Signer{DB: db, Key: key, Now: time.Now, State: hw}, nil
}

// canonSite validates a site id and returns its canonical form (lowercase,
// hyphenated). Anything else would be signed under text that a verifier,
// rebuilding the payload from the database's uuid, can never reproduce.
func canonSite(id string) (string, error) {
	if len(id) != 36 {
		return "", fmt.Errorf("signer: site id %q is not a hyphenated uuid", id)
	}
	u, err := uuid.Parse(id)
	if err != nil {
		return "", fmt.Errorf("signer: site id %q: %w", id, err)
	}
	return u.String(), nil // lowercase
}

func lockKey(siteID string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte("signer:" + siteID))
	return int64(h.Sum64()) // #nosec G115 -- wrap-around is fine, this is only a lock id
}

const headCols = `id, site_id, seq, entry_hash, signed_at, reason, key_id, signature`

func scanHead(r pgx.Row) (*SignedHead, error) {
	var h SignedHead
	if err := r.Scan(&h.ID, &h.SiteID, &h.Seq, &h.EntryHash, &h.SignedAt, &h.Reason, &h.KeyID, &h.Signature); err != nil {
		return nil, err
	}
	return &h, nil
}

// SignSite signs the current chain head of a site and stores it. It is
// idempotent: an unchanged head returns the row that already exists. It
// refuses (and signs nothing) when
//   - the head is lower than a signed one (ErrLowerSeq),
//   - the signed seq now has another hash (ErrConflict),
//   - the entry at the last signed seq changed (ErrPastRewritten),
//   - the links since the last signed seq do not hold (ErrBrokenLinks).
func (s *Signer) SignSite(ctx context.Context, siteID, reason string) (*SignedHead, error) {
	siteID, err := canonSite(siteID)
	if err != nil {
		return nil, err
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, lockKey(siteID)); err != nil {
		return nil, err
	}

	var seq int64
	var hash []byte
	if err := s.reconcile(ctx, tx, siteID); err != nil {
		return nil, err
	}
	err = tx.QueryRow(ctx, `SELECT last_seq, last_hash FROM record_chain_heads WHERE site_id = $1::uuid`, siteID).Scan(&seq, &hash)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && seq < 1) {
		return nil, ErrNoChain
	}
	if err != nil {
		return nil, err
	}

	last, err := scanHead(tx.QueryRow(ctx, `SELECT `+headCols+` FROM signed_heads WHERE site_id = $1::uuid ORDER BY seq DESC, id DESC LIMIT 1`, siteID))
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	from := int64(1)
	if last != nil {
		switch {
		case seq < last.Seq:
			return nil, fmt.Errorf("%w: head %d, signed %d", ErrLowerSeq, seq, last.Seq)
		case seq == last.Seq && string(hash) != string(last.EntryHash):
			return nil, fmt.Errorf("%w: seq %d", ErrConflict, seq)
		case seq == last.Seq:
			// Unchanged head. Commit rather than just return: reconcile may have
			// repaired a head in this transaction (a crash before the commit).
			return last, tx.Commit(ctx)
		}
		var cur []byte
		err := tx.QueryRow(ctx, `SELECT entry_hash FROM record_chain WHERE site_id = $1::uuid AND seq = $2`, siteID, last.Seq).Scan(&cur)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && string(cur) != string(last.EntryHash)) {
			return nil, fmt.Errorf("%w: seq %d", ErrPastRewritten, last.Seq)
		}
		if err != nil {
			return nil, err
		}
		from = last.Seq
	}
	var brokenAt int64
	var why string
	err = tx.QueryRow(ctx, `SELECT seq, reason FROM chain_links_break($1::uuid, $2)`, siteID, from).Scan(&brokenAt, &why)
	if err == nil {
		return nil, fmt.Errorf("%w: seq %d: %s", ErrBrokenLinks, brokenAt, why)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}

	now := s.Now().UTC().Truncate(time.Microsecond)
	keyID := KeyID(pubOf(s.Key))
	sig := ed25519.Sign(s.Key, HeadPayload(siteID, seq, hash, now, keyID))
	// The signer's own record first: if anything fails after this line, the
	// head is on disk and the next tick stores it (reconcile).
	if err := s.State.Append(SignedHead{SiteID: siteID, Seq: seq, EntryHash: hash, SignedAt: now, Reason: reason, KeyID: keyID, Signature: sig}); err != nil {
		return nil, err
	}
	h, err := scanHead(tx.QueryRow(ctx, `INSERT INTO signed_heads (site_id, seq, entry_hash, signed_at, reason, key_id, signature)
		VALUES ($1::uuid, $2, $3, $4, $5, $6, $7) RETURNING `+headCols, siteID, seq, hash, now, reason, keyID, sig))
	if err != nil {
		return nil, fmt.Errorf("store signed head: %w", err)
	}
	return h, tx.Commit(ctx)
}

// reconcile compares the signer's own record of a site with signed_heads.
// Heads on disk that the database lacks mean the database lost or dropped
// them (or was rewritten): the signer then refuses to sign, loudly, instead of
// accepting the database's shorter memory. One exception is repaired: the
// newest head of the site, when the chain still holds exactly that entry (a
// crash between the disk write and the insert).
func (s *Signer) reconcile(ctx context.Context, tx querier, siteID string) error {
	local := s.State.ForSite(siteID)
	if len(local) == 0 {
		return nil
	}
	missing, err := missingFromDB(ctx, tx, local)
	if err != nil || len(missing) == 0 {
		return err
	}
	newest := local[len(local)-1]
	if len(missing) == 1 && missing[0].Seq == newest.Seq && string(missing[0].EntryHash) == string(newest.EntryHash) {
		var cur []byte
		err := tx.QueryRow(ctx, `SELECT entry_hash FROM record_chain WHERE site_id = $1::uuid AND seq = $2`, siteID, newest.Seq).Scan(&cur)
		var anyAtOrAbove bool
		if err == nil && string(cur) == string(newest.EntryHash) {
			if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM signed_heads WHERE site_id = $1::uuid AND seq >= $2)`, siteID, newest.Seq).Scan(&anyAtOrAbove); err != nil {
				return err
			}
			if !anyAtOrAbove {
				return insertHead(ctx, tx, newest)
			}
		}
	}
	seqs := make([]int64, len(missing))
	for i, m := range missing {
		seqs[i] = m.Seq
	}
	return fmt.Errorf("%w: site %s, seq %v (not signing; see the runbook: restore-heads, then investigate)", ErrHeadsLost, siteID, seqs)
}

func missingFromDB(ctx context.Context, db querier, local []SignedHead) ([]SignedHead, error) {
	var missing []SignedHead
	for _, h := range local {
		var ok bool
		if err := db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM signed_heads WHERE site_id = $1::uuid AND seq = $2 AND key_id = $3 AND entry_hash = $4)`,
			h.SiteID, h.Seq, h.KeyID, h.EntryHash).Scan(&ok); err != nil {
			return nil, err
		}
		if !ok {
			missing = append(missing, h)
		}
	}
	return missing, nil
}

func insertHead(ctx context.Context, db querier, h SignedHead) error {
	_, err := db.Exec(ctx, `INSERT INTO signed_heads (site_id, seq, entry_hash, signed_at, reason, key_id, signature)
		VALUES ($1::uuid, $2, $3, $4, $5, $6, $7)`, h.SiteID, h.Seq, h.EntryHash, h.SignedAt, h.Reason, h.KeyID, h.Signature)
	return err
}

// RestoreHeads re-inserts the heads on the signer's volume that signed_heads
// lacks (operator command, after the loss has been looked at). The signatures
// are the stored ones; nothing is signed. The table's own guard still refuses
// to insert below a newer head.
func (s *Signer) RestoreHeads(ctx context.Context) (int, error) {
	n := 0
	for _, h := range s.State.Heads() {
		missing, err := missingFromDB(ctx, s.DB, []SignedHead{h})
		if err != nil {
			return n, err
		}
		if len(missing) == 0 {
			continue
		}
		if err := insertHead(ctx, s.DB, h); err != nil {
			return n, fmt.Errorf("restore site %s seq %d: %w", h.SiteID, h.Seq, err)
		}
		n++
	}
	return n, nil
}
