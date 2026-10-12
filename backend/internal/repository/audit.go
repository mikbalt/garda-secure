package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/securepatrol/backend/internal/model"
)

const auditColumns = `id, COALESCE(user_id, ''), realm, actor, guard_id, roles, action, method, route,
	resource_type, resource_id, params, request_id, summary, before_state, after_state,
	COALESCE(ip, ''), COALESCE(user_agent, ''), COALESCE(status_code, 0), created_at`

// InsertAudit appends one audit entry. The table is append-only (trigger).
func InsertAudit(ctx context.Context, db DBTX, a model.AuditLog) error {
	_, err := db.Exec(ctx, `
		INSERT INTO audit_log
		  (user_id, realm, actor, guard_id, roles, action, method, route, resource_type, resource_id,
		   params, request_id, summary, before_state, after_state, ip, user_agent, status_code)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)`,
		a.UserID, a.Realm, a.Actor, a.GuardID, a.Roles, a.Action, a.Method, a.Route, a.ResourceType,
		a.ResourceID, nullJSON(a.Params), a.RequestID, nullJSON(a.Summary), nullJSON(a.Before),
		nullJSON(a.After), a.IP, a.UserAgent, a.StatusCode)
	if err != nil {
		return fmt.Errorf("InsertAudit: %w", err)
	}
	return nil
}

func nullJSON(b []byte) any {
	if len(b) == 0 {
		return nil
	}
	return b
}

// AuditFilter narrows ListAudit; Realm is mandatory, empty fields are ignored.
type AuditFilter struct {
	Realm        string
	From, To     *time.Time
	Actor        string // username/subject, or a guard ID
	ResourceType string // exact type or its prefix ("patrol" matches "patrol.sessions")
	ResourceID   string
	Limit        int
	Offset       int
}

// ListAudit returns a realm's audit entries, newest first.
func ListAudit(ctx context.Context, db DBTX, f AuditFilter) ([]model.AuditLog, error) {
	rows, err := db.Query(ctx, `
		SELECT `+auditColumns+` FROM audit_log
		WHERE realm = $1
		  AND ($2::timestamptz IS NULL OR created_at >= $2)
		  AND ($3::timestamptz IS NULL OR created_at <  $3)
		  AND ($4 = '' OR actor = $4 OR guard_id::text = $4)
		  AND ($5 = '' OR resource_type = $5 OR resource_type LIKE $5 || '.%')
		  AND ($6 = '' OR resource_id = $6)
		ORDER BY created_at DESC, id DESC
		LIMIT $7 OFFSET $8`,
		f.Realm, f.From, f.To, f.Actor, f.ResourceType, f.ResourceID, f.Limit, f.Offset)
	if err != nil {
		return nil, fmt.Errorf("ListAudit: %w", err)
	}
	return collect(rows, func(r pgx.Rows) (model.AuditLog, error) {
		var a model.AuditLog
		err := r.Scan(&a.ID, &a.UserID, &a.Realm, &a.Actor, &a.GuardID, &a.Roles, &a.Action, &a.Method,
			&a.Route, &a.ResourceType, &a.ResourceID, &a.Params, &a.RequestID, &a.Summary, &a.Before,
			&a.After, &a.IP, &a.UserAgent, &a.StatusCode, &a.CreatedAt)
		if a.Roles == nil {
			a.Roles = []string{}
		}
		return a, err
	})
}
