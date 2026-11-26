package repository

import (
	"context"
	"fmt"
	"time"
)

// FlagKinds are the record kinds that carry plausibility flags.
var FlagKinds = []string{"attendance", "scan", "session", "incident", "sos", "defect", "visitor"}

// flagSource lists every flagged record as (kind, record_id, site_id,
// guard_id, event_at, flags). It only reads the record tables.
const flagSource = `
	SELECT 'attendance' AS kind, id, site_id, guard_id, COALESCE(clock_in_at, created_at) AS event_at, flags FROM attendance_records
	UNION ALL SELECT 'scan', cs.id, r.site_id, cs.guard_id, cs.scanned_at, cs.flags
		FROM checkpoint_scans cs JOIN routes r ON r.id = cs.route_id
	UNION ALL SELECT 'session', id, site_id, guard_id, started_at, flags FROM patrol_sessions
	UNION ALL SELECT 'incident', id, site_id, guard_id, COALESCE(occurred_at, created_at), flags FROM incidents
	UNION ALL SELECT 'sos', id, site_id, guard_id, fired_at, flags FROM sos_alerts
	UNION ALL SELECT 'defect', id, site_id, guard_id, created_at, flags FROM defect_reports
	UNION ALL SELECT 'visitor', id, site_id, guard_id, COALESCE(entry_at, created_at), flags FROM visitor_logs`

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
	SiteID   string
	GuardID  string
	From, To *time.Time
	Flag     string
	Reviewed *bool
	Limit    int
	Offset   int
}

const flagSelect = `
	WITH f AS (SELECT s.kind, s.id AS record_id, s.site_id, s.guard_id, s.event_at, u.flag
	           FROM (` + flagSource + `) s CROSS JOIN LATERAL unnest(s.flags) AS u(flag))
	SELECT f.kind, f.record_id::text, f.flag, f.site_id::text, st.name, f.guard_id::text, g.name, f.event_at,
	       fr.reviewed_by, fr.reviewed_at, fr.note
	FROM f
	JOIN sites st ON st.id = f.site_id
	JOIN guards g ON g.id = f.guard_id
	LEFT JOIN flag_reviews fr ON fr.kind = f.kind AND fr.record_id = f.record_id AND fr.flag = f.flag`

// ListFlags returns flags newest first. qr_unsigned (a legacy plain QR, not a
// finding) is listed only when asked for with Flag.
func ListFlags(ctx context.Context, db DBTX, f FlagFilter) ([]FlagRow, error) {
	rows, err := db.Query(ctx, flagSelect+`
		WHERE f.site_id = ANY($1::uuid[])
		  AND ($2 = '' OR f.site_id::text = $2)
		  AND ($3 = '' OR f.guard_id::text = $3)
		  AND ($4::timestamptz IS NULL OR f.event_at >= $4)
		  AND ($5::timestamptz IS NULL OR f.event_at <  $5)
		  AND (($6 = '' AND f.flag <> 'qr_unsigned') OR f.flag = $6)
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
	rows, err := db.Query(ctx, flagSelect+` WHERE f.kind = $1 AND f.record_id::text = $2 AND f.flag = $3`, kind, recordID, flag)
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

// ReviewFlag stores the review mark of a flag. The first review wins: a
// repeat keeps the original reviewer, time and note. Only flag_reviews is
// written, never the flagged record.
func ReviewFlag(ctx context.Context, db DBTX, f FlagRow, by, note string) error {
	_, err := db.Exec(ctx, `
		INSERT INTO flag_reviews (kind, record_id, flag, site_id, reviewed_by, note)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (kind, record_id, flag) DO NOTHING`,
		f.Kind, f.RecordID, f.Flag, f.SiteID, by, note)
	if err != nil {
		return fmt.Errorf("ReviewFlag: %w", err)
	}
	return nil
}
