package repository

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// FlagKinds are the record kinds that carry plausibility flags.
var FlagKinds = []string{"attendance", "scan", "session", "incident", "sos", "defect", "visitor"}

// flagBranch is one record table with the columns the dashboard needs.
type flagBranch struct {
	kind, from, id, site, guard, at, flags string
}

var flagBranches = []flagBranch{
	{"attendance", "attendance_records", "id", "site_id", "guard_id", "COALESCE(clock_in_at, created_at)", "flags"},
	{"scan", "checkpoint_scans cs JOIN routes r ON r.id = cs.route_id", "cs.id", "r.site_id", "cs.guard_id", "cs.scanned_at", "cs.flags"},
	{"session", "patrol_sessions", "id", "site_id", "guard_id", "started_at", "flags"},
	{"incident", "incidents", "id", "site_id", "guard_id", "COALESCE(occurred_at, created_at)", "flags"},
	{"sos", "sos_alerts", "id", "site_id", "guard_id", "fired_at", "flags"},
	{"defect", "defect_reports", "id", "site_id", "guard_id", "created_at", "flags"},
	{"visitor", "visitor_logs", "id", "site_id", "guard_id", "COALESCE(entry_at, created_at)", "flags"},
}

// flagSource lists flagged records as (kind, id, site_id, guard_id, event_at,
// flags). where adds the predicates of each branch, so sites, days and ids are
// applied on the record tables (and their indexes) before the union. It only
// reads the record tables.
func flagSource(only string, where func(b flagBranch) string) string {
	parts := []string{}
	for _, b := range flagBranches {
		if only != "" && b.kind != only {
			continue
		}
		parts = append(parts, fmt.Sprintf(`SELECT '%s' AS kind, %s AS id, %s AS site_id, %s AS guard_id, %s AS event_at, %s AS flags FROM %s WHERE cardinality(%s) > 0 AND %s`,
			b.kind, b.id, b.site, b.guard, b.at, b.flags, b.from, b.flags, where(b)))
	}
	return strings.Join(parts, "\n\tUNION ALL ")
}

// FlagRow is one flag on one record, with its review mark if any.
type FlagRow struct {
	Kind       string     `json:"kind"`
	RecordID   string     `json:"record_id"`
	Flag       string     `json:"flag"`
	SiteID     string     `json:"site_id"`
	SiteName   string     `json:"site_name"`
	GuardID    string     `json:"guard_id"`
	GuardName  string     `json:"guard_name"`
	EventAt    time.Time  `json:"event_at"`
	ReviewedBy *string    `json:"reviewed_by,omitempty"`
	ReviewedAt *time.Time `json:"reviewed_at,omitempty"`
	Note       *string    `json:"note,omitempty"`
}

// FlagFilter narrows ListFlags. SiteIDs (the caller's visible sites) is
// mandatory; the other fields are ignored when empty/nil.
type FlagFilter struct {
	SiteIDs  []string
	SiteID   *string
	GuardID  *string
	From, To *time.Time
	Flag     string
	Reviewed *bool
	Limit    int
	Offset   int
}

func flagSelect(src string) string {
	return `
	WITH f AS (SELECT s.kind, s.id AS record_id, s.site_id, s.guard_id, s.event_at, u.flag
	           FROM (` + src + `) s CROSS JOIN LATERAL unnest(s.flags) AS u(flag))
	SELECT f.kind, f.record_id::text, f.flag, f.site_id::text, st.name, f.guard_id::text, g.name, f.event_at,
	       fr.reviewed_by, fr.reviewed_at, fr.note
	FROM f
	JOIN sites st ON st.id = f.site_id
	JOIN guards g ON g.id = f.guard_id
	LEFT JOIN flag_reviews fr ON fr.kind = f.kind AND fr.record_id = f.record_id AND fr.flag = f.flag`
}

// ListFlags returns flags newest first. qr_unsigned (a legacy plain QR, not a
// finding) is listed only when asked for with Flag.
func ListFlags(ctx context.Context, db DBTX, f FlagFilter) ([]FlagRow, error) {
	src := flagSource("", func(b flagBranch) string {
		return fmt.Sprintf(`%[1]s = ANY($1::uuid[]) AND ($2::uuid IS NULL OR %[1]s = $2::uuid) AND ($3::uuid IS NULL OR %[2]s = $3::uuid)
		  AND ($4::timestamptz IS NULL OR %[3]s >= $4) AND ($5::timestamptz IS NULL OR %[3]s < $5)`, b.site, b.guard, b.at)
	})
	rows, err := db.Query(ctx, flagSelect(src)+`
		WHERE (($6 = '' AND f.flag <> 'qr_unsigned') OR f.flag = $6)
		  AND ($7::boolean IS NULL OR (fr.id IS NOT NULL) = $7)
		ORDER BY f.event_at DESC, f.record_id, f.flag
		LIMIT $8 OFFSET $9`,
		f.SiteIDs, f.SiteID, f.GuardID, f.From, f.To, f.Flag, f.Reviewed, f.Limit, f.Offset)
	if err != nil {
		return nil, fmt.Errorf("ListFlags: %w", err)
	}
	defer rows.Close()
	out := []FlagRow{}
	for rows.Next() {
		var r FlagRow
		if err := rows.Scan(&r.Kind, &r.RecordID, &r.Flag, &r.SiteID, &r.SiteName, &r.GuardID, &r.GuardName,
			&r.EventAt, &r.ReviewedBy, &r.ReviewedAt, &r.Note); err != nil {
			return nil, fmt.Errorf("ListFlags: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// GetFlag returns one flag of one record; ErrNotFound when the record does
// not carry that flag.
func GetFlag(ctx context.Context, db DBTX, kind, recordID, flag string) (*FlagRow, error) {
	src := flagSource(kind, func(b flagBranch) string { return b.id + " = $1::uuid" })
	rows, err := db.Query(ctx, flagSelect(src)+` WHERE f.flag = $2`, recordID, flag)
	if err != nil {
		return nil, fmt.Errorf("GetFlag: %w", err)
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, ErrNotFound
	}
	var r FlagRow
	if err := rows.Scan(&r.Kind, &r.RecordID, &r.Flag, &r.SiteID, &r.SiteName, &r.GuardID, &r.GuardName,
		&r.EventAt, &r.ReviewedBy, &r.ReviewedAt, &r.Note); err != nil {
		return nil, fmt.Errorf("GetFlag: %w", err)
	}
	return &r, rows.Err()
}

// ReviewFlag stores the review mark of a flag and reports whether this call
// created it. The first review wins: a repeat (or a concurrent one that lost)
// keeps the original reviewer, time and note. Only flag_reviews is written,
// never the flagged record.
func ReviewFlag(ctx context.Context, db DBTX, f FlagRow, by, note string) (created bool, err error) {
	tag, err := db.Exec(ctx, `
		INSERT INTO flag_reviews (kind, record_id, flag, site_id, reviewed_by, note)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (kind, record_id, flag) DO NOTHING`,
		f.Kind, f.RecordID, f.Flag, f.SiteID, by, note)
	if err != nil {
		return false, fmt.Errorf("ReviewFlag: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}
