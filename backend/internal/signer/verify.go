package signer

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
)

// Statuses of a failed verification.
const (
	StatusBroken   = "BROKEN"   // a signed head does not hold
	StatusUnsigned = "UNSIGNED" // the chain has entries but no signed head at all
	StatusStale    = "STALE"    // entries after the newest signed head, which is older than the limit
)

// HeadBreak is the first problem found when checking a site's signed heads.
type HeadBreak struct {
	Seq    int64
	Reason string
	Status string
}

// VerifyOptions tunes Verify. The zero value checks signatures and the chain only.
type VerifyOptions struct {
	// Local are heads from the signer's own record (LoadHighWater): checked
	// like database rows, and reported when signed_heads lacks them.
	Local []SignedHead
	// MaxAge > 0 also fails when the chain has entries after the newest signed
	// head and that head is older than MaxAge. A quiet site is never stale: the
	// signer signs a head once, however long ago.
	MaxAge time.Duration
	Now    time.Time // zero = time.Now()
}

// VerifyHeads is Verify with default options.
func VerifyHeads(ctx context.Context, db querier, siteID string, pins []string) (*HeadBreak, error) {
	return Verify(ctx, db, siteID, pins, VerifyOptions{})
}

// Verify checks a site's signed heads against its chain: each signature must
// be valid under a trusted key (see verifyKeyring; pins are the key
// fingerprints the client holds out of band), must not postdate its key's
// retirement, and the chain entry at that seq must still have the signed hash.
// The chain must also reach the highest signed seq, and a chain with entries
// must have at least one signed head. It returns the first problem (lowest
// seq), or nil. It catches a past that was rewritten and re-chained, which
// chain_first_break alone accepts.
func Verify(ctx context.Context, db querier, siteID string, pins []string, opts VerifyOptions) (*HeadBreak, error) {
	siteID, err := canonSite(siteID)
	if err != nil {
		return nil, err
	}
	ring, err := loadKeyring(ctx, db)
	if err != nil {
		return nil, err
	}
	trust := verifyKeyring(ring, pins)

	rows, err := db.Query(ctx, `SELECT `+headCols+` FROM signed_heads WHERE site_id = $1::uuid ORDER BY seq, id`, siteID)
	if err != nil {
		return nil, err
	}
	type entry struct {
		SignedHead
		onlyLocal bool
	}
	var heads []entry
	inDB := map[string]bool{}
	for rows.Next() {
		h, err := scanHead(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		heads = append(heads, entry{SignedHead: *h})
		inDB[fmt.Sprintf("%d|%s|%x", h.Seq, h.KeyID, h.EntryHash)] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, h := range opts.Local {
		if h.SiteID == siteID && !inDB[fmt.Sprintf("%d|%s|%x", h.Seq, h.KeyID, h.EntryHash)] {
			heads = append(heads, entry{SignedHead: h, onlyLocal: true})
		}
	}
	sort.SliceStable(heads, func(i, j int) bool { return heads[i].Seq < heads[j].Seq })

	var maxSeq int64
	var newest *SignedHead
	for i, h := range heads {
		if r := headProblem(h.SignedHead, trust); r != "" {
			return &HeadBreak{Seq: h.Seq, Reason: r, Status: StatusBroken}, nil
		}
		var cur []byte
		err := db.QueryRow(ctx, `SELECT entry_hash FROM record_chain WHERE site_id = $1::uuid AND seq = $2`, siteID, h.Seq).Scan(&cur)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			return &HeadBreak{Seq: h.Seq, Reason: "signed entry is missing from the chain", Status: StatusBroken}, nil
		case err != nil:
			return nil, err
		case string(cur) != string(h.EntryHash):
			return &HeadBreak{Seq: h.Seq, Reason: "chain entry differs from the signed head (past rewritten)", Status: StatusBroken}, nil
		case h.onlyLocal:
			return &HeadBreak{Seq: h.Seq, Reason: "head recorded by the signer is missing from signed_heads (deleted)", Status: StatusBroken}, nil
		}
		if h.Seq >= maxSeq {
			maxSeq = h.Seq
			newest = &heads[i].SignedHead
		}
	}
	var last int64
	if err := db.QueryRow(ctx, `SELECT COALESCE(max(seq), 0) FROM record_chain WHERE site_id = $1::uuid`, siteID).Scan(&last); err != nil {
		return nil, err
	}
	switch {
	case newest == nil && last > 0:
		return &HeadBreak{Seq: 1, Status: StatusUnsigned, Reason: fmt.Sprintf("chain has %d entries and no signed head", last)}, nil
	case newest != nil && last < maxSeq:
		return &HeadBreak{Seq: last + 1, Status: StatusBroken, Reason: fmt.Sprintf("chain ends at %d, before the signed head %d", last, maxSeq)}, nil
	case newest != nil && last > maxSeq && opts.MaxAge > 0:
		now := opts.Now
		if now.IsZero() {
			now = time.Now()
		}
		if age := now.Sub(newest.SignedAt); age > opts.MaxAge {
			return &HeadBreak{Seq: maxSeq + 1, Status: StatusStale,
				Reason: fmt.Sprintf("newest signed head (seq %d) is %s old, limit %s, and the chain is at %d", maxSeq, age.Round(time.Second), opts.MaxAge, last)}, nil
		}
	}
	return nil, nil
}

func headProblem(h SignedHead, trust map[string]*keyTrust) string {
	kt := trust[h.KeyID]
	switch {
	case kt == nil:
		return "signed with a key that is not in the keyring"
	case !kt.trusted:
		return "signing key not trusted: " + kt.why
	case !verify(ed25519.PublicKey(kt.entry.PublicKey), HeadPayload(h.SiteID, h.Seq, h.EntryHash, h.SignedAt, h.KeyID), h.Signature):
		return "signature does not verify"
	case !kt.retiredAt.IsZero() && h.SignedAt.After(kt.retiredAt.Add(time.Second)):
		return "signed after its key was retired"
	}
	return ""
}
