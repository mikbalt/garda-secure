package chainproof

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Querier is satisfied by a pgx pool or transaction.
type Querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// TooLargeError means the range needs more entries than allowed (or the
// database gave up on it): use the offline bundle or a smaller range.
type TooLargeError struct{ Entries, Limit int64 }

func (e *TooLargeError) Error() string {
	return fmt.Sprintf("the range needs %d chain entries, the limit here is %d", e.Entries, e.Limit)
}

// IsTimeout reports a statement_timeout cancel (SQLSTATE 57014).
func IsTimeout(err error) bool {
	var pe *pgconn.PgError
	return errors.As(err, &pe) && pe.Code == "57014"
}

// Export reads the bundle of a site for from <= created_at < to. Run it in one
// read-only repeatable-read transaction so entries, heads and tip agree.
//
// The bundle starts at the newest signed head before the range (the
// checkpoint, anchor_seq; none = the site's first entry) and ends at the first
// signed head at or after the range, or at the chain's end. Each end is thus
// vouched for by a signature, and the cost is the distance between two heads,
// not the history of the site. maxEntries bounds the entry count.
func Export(ctx context.Context, db Querier, siteID, timezone string, from, to, now time.Time, maxEntries int64) (*Bundle, error) {
	id, err := uuid.Parse(siteID)
	if err != nil {
		return nil, errors.New("site id is not a uuid")
	}
	siteID = id.String()
	b := &Bundle{Manifest: Manifest{SiteID: siteID, SiteTimezone: timezone,
		From: from.UTC().Format(time.RFC3339), To: to.UTC().Format(time.RFC3339), ExportedAt: now.UTC().Format(time.RFC3339Nano)}}

	var tipHash []byte
	err = db.QueryRow(ctx, `SELECT last_seq, last_hash FROM record_chain_heads WHERE site_id = $1::uuid`, siteID).Scan(&b.Manifest.ChainTipSeq, &tipHash)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	b.Manifest.ChainTipHash = hex.EncodeToString(tipHash)

	var lo, hi *int64
	if err := db.QueryRow(ctx, `SELECT
			(SELECT min(seq) FROM record_chain WHERE site_id = $1::uuid AND created_at >= $2),
			(SELECT max(seq) FROM record_chain WHERE site_id = $1::uuid AND created_at < $3)`, siteID, from, to).Scan(&lo, &hi); err != nil {
		return nil, err
	}
	if lo == nil || hi == nil || *lo > *hi {
		return b.withKeyring(ctx, db) // nothing in the range
	}

	var anchor int64
	if err := db.QueryRow(ctx, `SELECT COALESCE(max(seq), 0) FROM signed_heads WHERE site_id = $1::uuid AND seq < $2`, siteID, *lo).Scan(&anchor); err != nil {
		return nil, err
	}
	end := b.Manifest.ChainTipSeq
	var firstSigned *int64
	if err := db.QueryRow(ctx, `SELECT min(seq) FROM signed_heads WHERE site_id = $1::uuid AND seq >= $2`, siteID, *hi).Scan(&firstSigned); err != nil {
		return nil, err
	}
	if firstSigned != nil && *firstSigned < end {
		end = *firstSigned
	}
	if n := end - anchor; n > maxEntries {
		return nil, &TooLargeError{Entries: n, Limit: maxEntries}
	}
	b.Manifest.AnchorSeq, b.Manifest.FirstSeq, b.Manifest.LastSeq = anchor, anchor+1, end

	// The record text is read only for the newest entry of a record: older
	// states are not kept anywhere, so their hashes can only be checked as links.
	rows, err := db.Query(ctx, `
		SELECT c.seq, c.kind, c.record_id::text, encode(c.record_hash, 'hex'), encode(c.prev_hash, 'hex'),
		       encode(c.entry_hash, 'hex'), c.hash_version, c.origin, c.created_at, n.next_seq,
		       CASE WHEN n.next_seq IS NULL THEN chain_record_canonical(c.kind, c.record_id) END
		FROM record_chain c
		LEFT JOIN LATERAL (SELECT min(x.seq) AS next_seq FROM record_chain x
		                   WHERE x.site_id = c.site_id AND x.kind = c.kind AND x.record_id = c.record_id AND x.seq > c.seq) n ON true
		WHERE c.site_id = $1::uuid AND c.seq > $2 AND c.seq <= $3
		ORDER BY c.seq`, siteID, anchor, end)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var e Entry
		var created time.Time
		var next *int64
		var rec *string
		if err := rows.Scan(&e.Seq, &e.Kind, &e.RecordID, &e.RecordHash, &e.PrevHash, &e.EntryHash, &e.HashVersion, &e.Origin, &created, &next, &rec); err != nil {
			rows.Close()
			return nil, err
		}
		e.CreatedAt = created.UTC().Format(tsLayout)
		switch {
		case next != nil:
			e.RecordState, e.NextSeq = StateSuperseded, *next
		case rec == nil:
			e.RecordState = StateDeleted
		default:
			e.RecordState, e.Record = StateLatest, *rec
		}
		b.Entries = append(b.Entries, e)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	hrows, err := db.Query(ctx, `SELECT site_id::text, seq, encode(entry_hash, 'hex'), signed_at, reason, key_id, encode(signature, 'hex')
		FROM signed_heads WHERE site_id = $1::uuid AND seq >= $2 AND seq <= $3 ORDER BY seq, id`, siteID, anchor, end)
	if err != nil {
		return nil, err
	}
	for hrows.Next() {
		var h Head
		var at time.Time
		if err := hrows.Scan(&h.SiteID, &h.Seq, &h.EntryHash, &at, &h.Reason, &h.KeyID, &h.Signature); err != nil {
			hrows.Close()
			return nil, err
		}
		h.SignedAt = at.UTC().Format(time.RFC3339Nano)
		b.Heads = append(b.Heads, h)
	}
	hrows.Close()
	if err := hrows.Err(); err != nil {
		return nil, err
	}
	return b.withKeyring(ctx, db)
}

func (b *Bundle) withKeyring(ctx context.Context, db Querier) (*Bundle, error) {
	rows, err := db.Query(ctx, `SELECT key_id, epoch, kind, encode(public_key, 'hex'), COALESCE(prev_key_id, ''), reason, created_at,
		encode(new_key_sig, 'hex'), COALESCE(encode(prev_key_sig, 'hex'), '') FROM keyring ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var k Key
		var at time.Time
		if err := rows.Scan(&k.KeyID, &k.Epoch, &k.Kind, &k.PublicKey, &k.PrevKeyID, &k.Reason, &at, &k.NewKeySig, &k.PrevKeySig); err != nil {
			return nil, err
		}
		k.CreatedAt = at.UTC().Format(time.RFC3339Nano)
		b.Keyring = append(b.Keyring, k)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	b.Manifest.SiteID = strings.ToLower(b.Manifest.SiteID)
	return b, nil
}
