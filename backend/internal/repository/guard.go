package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/securepatrol/backend/internal/model"
)

const guardColumns = `id, keycloak_user_id, name, employee_id, site_id, role, pin_hash, status, created_at, updated_at`

func scanGuard(row pgx.Row) (model.Guard, error) {
	var g model.Guard
	err := row.Scan(&g.ID, &g.KeycloakUserID, &g.Name, &g.EmployeeID, &g.SiteID,
		&g.Role, &g.PinHash, &g.Status, &g.CreatedAt, &g.UpdatedAt)
	g.HasPin = g.PinHash != nil && *g.PinHash != ""
	return g, err
}

func scanGuardRows(rows pgx.Rows) (model.Guard, error) { return scanGuard(rows) }

// ListGuardsBySite returns the guards assigned to a site.
func ListGuardsBySite(ctx context.Context, db DBTX, siteID string) ([]model.Guard, error) {
	rows, err := db.Query(ctx,
		`SELECT `+guardColumns+` FROM guards WHERE site_id = $1 ORDER BY name`, siteID)
	if err != nil {
		return nil, fmt.Errorf("ListGuardsBySite: %w", err)
	}
	return collect(rows, scanGuardRows)
}

// ListGuardsByRealm returns all guards assigned to sites of a realm.
func ListGuardsByRealm(ctx context.Context, db DBTX, realm string) ([]model.Guard, error) {
	rows, err := db.Query(ctx, `
		SELECT `+prefixed("g", guardColumns)+`
		FROM guards g
		JOIN sites s ON s.id = g.site_id
		WHERE s.keycloak_realm = $1
		ORDER BY g.name`, realm)
	if err != nil {
		return nil, fmt.Errorf("ListGuardsByRealm: %w", err)
	}
	return collect(rows, scanGuardRows)
}

// GetGuard retrieves a guard by ID.
func GetGuard(ctx context.Context, db DBTX, id string) (*model.Guard, error) {
	g, err := scanGuard(db.QueryRow(ctx, `SELECT `+guardColumns+` FROM guards WHERE id = $1`, id))
	if err != nil {
		return nil, mapNoRows(err)
	}
	return &g, nil
}

// CreateGuard inserts a new guard.
func CreateGuard(ctx context.Context, db DBTX, g model.Guard) (*model.Guard, error) {
	out, err := scanGuard(db.QueryRow(ctx, `
		INSERT INTO guards (keycloak_user_id, name, employee_id, site_id, role, status)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING `+guardColumns,
		g.KeycloakUserID, g.Name, g.EmployeeID, g.SiteID, g.Role, g.Status))
	if err != nil {
		return nil, fmt.Errorf("CreateGuard: %w", err)
	}
	return &out, nil
}

// UpdateGuard replaces the editable fields of a guard.
func UpdateGuard(ctx context.Context, db DBTX, g model.Guard) (*model.Guard, error) {
	out, err := scanGuard(db.QueryRow(ctx, `
		UPDATE guards
		SET name = $2, employee_id = $3, site_id = $4, role = $5, status = $6,
		    keycloak_user_id = $7, updated_at = now()
		WHERE id = $1
		RETURNING `+guardColumns,
		g.ID, g.Name, g.EmployeeID, g.SiteID, g.Role, g.Status, g.KeycloakUserID))
	if err != nil {
		return nil, mapNoRows(err)
	}
	return &out, nil
}

// EmployeeIDTaken reports whether another guard of a realm's sites already
// uses employeeID (employee IDs are unique per client, BE-S10).
func EmployeeIDTaken(ctx context.Context, db DBTX, realm, employeeID, exceptGuardID string) (bool, error) {
	var taken bool
	err := db.QueryRow(ctx, `
		SELECT EXISTS (
		  SELECT 1 FROM guards g JOIN sites s ON s.id = g.site_id
		  WHERE s.keycloak_realm = $1 AND g.employee_id = $2 AND g.id::text <> $3)`,
		realm, employeeID, exceptGuardID).Scan(&taken)
	if err != nil {
		return false, fmt.Errorf("EmployeeIDTaken: %w", err)
	}
	return taken, nil
}

// SetGuardPinHash stores a new bcrypt PIN hash for a guard.
func SetGuardPinHash(ctx context.Context, db DBTX, id, hash string) error {
	tag, err := db.Exec(ctx,
		`UPDATE guards SET pin_hash = $2, updated_at = now() WHERE id = $1`, id, hash)
	if err != nil {
		return fmt.Errorf("SetGuardPinHash: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ListRoster returns the active guards of a site with their PIN hashes, for
// offline PIN verification on the site's tablets.
func ListRoster(ctx context.Context, db DBTX, siteID string) ([]model.RosterGuard, error) {
	rows, err := db.Query(ctx, `
		SELECT id, name, employee_id, role, pin_hash
		FROM guards
		WHERE site_id = $1 AND status = 'active'
		ORDER BY name`, siteID)
	if err != nil {
		return nil, fmt.Errorf("ListRoster: %w", err)
	}
	return collect(rows, func(r pgx.Rows) (model.RosterGuard, error) {
		var g model.RosterGuard
		err := r.Scan(&g.ID, &g.Name, &g.EmployeeID, &g.Role, &g.PinHash)
		return g, err
	})
}
