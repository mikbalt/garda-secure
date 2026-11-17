package repository

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/securepatrol/backend/internal/model"
)

const siteColumns = `id, name, client_id, keycloak_realm, address, timezone, config_json, active, created_at`

func scanSite(row pgx.Row) (model.Site, error) {
	var s model.Site
	var rawCfg []byte
	err := row.Scan(&s.ID, &s.Name, &s.ClientID, &s.KeycloakRealm, &s.Address, &s.Timezone, &rawCfg, &s.Active, &s.CreatedAt)
	s.ConfigJSON = json.RawMessage(rawCfg)
	return s, err
}

func scanSiteRows(rows pgx.Rows) (model.Site, error) { return scanSite(rows) }

// ListSitesByRealm returns the sites belonging to one Keycloak realm (client);
// deactivated sites only with includeInactive.
func ListSitesByRealm(ctx context.Context, db DBTX, realm string, includeInactive bool) ([]model.Site, error) {
	rows, err := db.Query(ctx,
		`SELECT `+siteColumns+` FROM sites WHERE keycloak_realm = $1 AND ($2 OR active) ORDER BY name`,
		realm, includeInactive)
	if err != nil {
		return nil, fmt.Errorf("ListSitesByRealm: %w", err)
	}
	return collect(rows, scanSiteRows)
}

// ListAllSites returns every active site; used by background jobs.
func ListAllSites(ctx context.Context, db DBTX) ([]model.Site, error) {
	rows, err := db.Query(ctx, `SELECT `+siteColumns+` FROM sites WHERE active ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("ListAllSites: %w", err)
	}
	return collect(rows, scanSiteRows)
}

// GetSite retrieves a site by ID.
func GetSite(ctx context.Context, db DBTX, id string) (*model.Site, error) {
	s, err := scanSite(db.QueryRow(ctx, `SELECT `+siteColumns+` FROM sites WHERE id = $1`, id))
	if err != nil {
		return nil, mapNoRows(err)
	}
	return &s, nil
}

// CreateSite inserts a new site.
func CreateSite(ctx context.Context, db DBTX, s model.Site) (*model.Site, error) {
	cfg := s.ConfigJSON
	if len(cfg) == 0 {
		cfg = json.RawMessage("{}")
	}
	out, err := scanSite(db.QueryRow(ctx, `
		INSERT INTO sites (name, client_id, keycloak_realm, address, timezone, config_json)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING `+siteColumns,
		s.Name, s.ClientID, s.KeycloakRealm, s.Address, s.Timezone, []byte(cfg)))
	if err != nil {
		return nil, fmt.Errorf("CreateSite: %w", err)
	}
	return &out, nil
}

// UpdateSite replaces the editable fields of a site.
func UpdateSite(ctx context.Context, db DBTX, s model.Site) (*model.Site, error) {
	cfg := s.ConfigJSON
	if len(cfg) == 0 {
		cfg = json.RawMessage("{}")
	}
	out, err := scanSite(db.QueryRow(ctx, `
		UPDATE sites SET name = $2, client_id = $3, address = $4, timezone = $5, config_json = $6
		WHERE id = $1
		RETURNING `+siteColumns,
		s.ID, s.Name, s.ClientID, s.Address, s.Timezone, []byte(cfg)))
	if err != nil {
		return nil, mapNoRows(err)
	}
	return &out, nil
}

// SetSiteActive activates or deactivates a site (soft delete).
func SetSiteActive(ctx context.Context, db DBTX, id string, active bool) (*model.Site, error) {
	out, err := scanSite(db.QueryRow(ctx,
		`UPDATE sites SET active = $2 WHERE id = $1 RETURNING `+siteColumns, id, active))
	if err != nil {
		return nil, mapNoRows(err)
	}
	return &out, nil
}

// CountFutureShifts counts the shifts of a site that start after now.
func CountFutureShifts(ctx context.Context, db DBTX, siteID string) (int, error) {
	var n int
	err := db.QueryRow(ctx,
		`SELECT count(*) FROM shifts WHERE site_id = $1 AND start_time > now()`, siteID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("CountFutureShifts: %w", err)
	}
	return n, nil
}

// CountActiveDevices counts the approved (active) tablets assigned to a site.
func CountActiveDevices(ctx context.Context, db DBTX, siteID string) (int, error) {
	var n int
	err := db.QueryRow(ctx,
		`SELECT count(*) FROM devices WHERE site_id = $1 AND status = 'active'`, siteID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("CountActiveDevices: %w", err)
	}
	return n, nil
}
