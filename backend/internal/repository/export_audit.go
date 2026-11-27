package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// Kinds and results of an export audit row (migration 015, TAQ-58).
const (
	ExportBundle    = "bundle"
	ExportProofPack = "proof_pack"

	ExportOK       = "ok"
	ExportTooLarge = "too_large"
	ExportFailed   = "failed"
)

// ExportAudit is one export: who, which site and range, when, how big, how it
// ended. It never holds content of the export.
type ExportAudit struct {
	ID        int64     `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	SiteID    string    `json:"site_id"`
	Actor     string    `json:"actor"`
	Kind      string    `json:"kind"`
	From      time.Time `json:"range_from"`
	To        time.Time `json:"range_to"` // exclusive
	Bytes     int64     `json:"bytes"`
	Result    string    `json:"result"`
	Signing   string    `json:"signing"`
	Detail    string    `json:"detail"`
}

// ExportAuditNew is what a caller records; the realm is the site's.
type ExportAuditNew struct {
	Realm, SiteID, Actor, ActorSub, Kind string
	From, To                             time.Time
	Bytes                                int64
	Result, Signing, Detail              string
}

// InsertExportAudit appends one row.
func InsertExportAudit(ctx context.Context, db DBTX, n ExportAuditNew) error {
	_, err := db.Exec(ctx, `INSERT INTO export_audit (realm, site_id, actor, actor_sub, kind, range_from, range_to, bytes, result, signing, detail)
		VALUES ($1, $2::uuid, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		n.Realm, n.SiteID, n.Actor, n.ActorSub, n.Kind, n.From, n.To, n.Bytes, n.Result, n.Signing, truncate(n.Detail, 500))
	if err != nil {
		return fmt.Errorf("InsertExportAudit: %w", err)
	}
	return nil
}

func truncate(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

// ListExportAudit returns the exports of one site of a realm, newest first.
func ListExportAudit(ctx context.Context, db DBTX, realm, siteID string, limit, offset int) ([]ExportAudit, error) {
	rows, err := db.Query(ctx, `SELECT id, created_at, site_id::text, actor, kind, range_from, range_to, bytes, result, signing, detail
		FROM export_audit WHERE realm = $1 AND site_id = $2::uuid
		ORDER BY created_at DESC, id DESC LIMIT $3 OFFSET $4`, realm, siteID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("ListExportAudit: %w", err)
	}
	return collect(rows, func(r pgx.Rows) (ExportAudit, error) {
		var a ExportAudit
		err := r.Scan(&a.ID, &a.CreatedAt, &a.SiteID, &a.Actor, &a.Kind, &a.From, &a.To, &a.Bytes, &a.Result, &a.Signing, &a.Detail)
		return a, err
	})
}
