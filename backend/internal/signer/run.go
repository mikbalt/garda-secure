package signer

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// dueReason says whether a site's head should be signed now, and why. A head
// is signed when it moved since the last signature and either a local day
// ended since then (day_end) or the last signature is an hour old (hourly).
func dueReason(now time.Time, loc *time.Location, headSeq int64, lastSeq *int64, lastAt time.Time) string {
	if lastSeq == nil {
		return ReasonHourly
	}
	if headSeq <= *lastSeq {
		return ""
	}
	n := now.In(loc)
	midnight := time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, loc)
	if lastAt.Before(midnight) {
		return ReasonDayEnd
	}
	if now.Sub(lastAt) >= time.Hour {
		return ReasonHourly
	}
	return ""
}

// SignDue signs every site whose head is due and returns how many it signed.
// A site that fails (for instance a broken chain) does not stop the others;
// the errors are returned together.
func (s *Signer) SignDue(ctx context.Context) (int, error) {
	rows, err := s.DB.Query(ctx, `
		SELECT h.site_id::text, h.last_seq, COALESCE(si.timezone, 'UTC'), ls.seq, ls.signed_at
		FROM record_chain_heads h
		LEFT JOIN sites si ON si.id = h.site_id
		LEFT JOIN LATERAL (SELECT seq, signed_at FROM signed_heads WHERE site_id = h.site_id ORDER BY seq DESC LIMIT 1) ls ON true
		WHERE h.last_seq >= 1`)
	if err != nil {
		return 0, err
	}
	type cand struct{ site, reason string }
	var todo []cand
	now := s.Now()
	for rows.Next() {
		var site, tz string
		var seq int64
		var lastSeq *int64
		var lastAt *time.Time
		if err := rows.Scan(&site, &seq, &tz, &lastSeq, &lastAt); err != nil {
			rows.Close()
			return 0, err
		}
		loc, err := time.LoadLocation(tz)
		if err != nil {
			loc = time.UTC
		}
		var at time.Time
		if lastAt != nil {
			at = *lastAt
		}
		if r := dueReason(now, loc, seq, lastSeq, at); r != "" {
			todo = append(todo, cand{site, r})
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	var errs []error
	n := 0
	for _, c := range todo {
		if _, err := s.SignSite(ctx, c.site, c.reason); err != nil {
			errs = append(errs, fmt.Errorf("site %s: %w", c.site, err))
			continue
		}
		n++
	}
	return n, errors.Join(errs...)
}
