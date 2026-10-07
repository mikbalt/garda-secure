// Package repository provides data-access functions for site records.
package repository

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/securepatrol/backend/internal/model"
)

// ListSites returns all sites ordered by name.
func ListSites(ctx context.Context, pool *pgxpool.Pool) ([]model.Site, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, name, client_id, keycloak_realm, address, timezone, config_json, created_at
		FROM sites
		ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("ListSites query: %w", err)
	}
	defer rows.Close()

	return scanSiteRows(rows)
}

// GetSiteByID retrieves a single site by UUID.
func GetSiteByID(ctx context.Context, pool *pgxpool.Pool, id string) (*model.Site, error) {
	var s model.Site
	var rawCfg []byte
	err := pool.QueryRow(ctx, `
		SELECT id, name, client_id, keycloak_realm, address, timezone, config_json, created_at
		FROM sites WHERE id = $1`, id,
	).Scan(&s.ID, &s.Name, &s.ClientID, &s.KeycloakRealm, &s.Address, &s.Timezone, &rawCfg, &s.CreatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("GetSiteByID: %w", err)
	}
	s.ConfigJSON = json.RawMessage(rawCfg)
	return &s, nil
}

// CreateSite inserts a new site and returns the full record.
func CreateSite(ctx context.Context, pool *pgxpool.Pool, s model.Site) (*model.Site, error) {
	cfg := s.ConfigJSON
	if len(cfg) == 0 {
		cfg = json.RawMessage("{}")
	}
	tz := s.Timezone
	if tz == "" {
		tz = "UTC"
	}

	var rawCfg []byte
	err := pool.QueryRow(ctx, `
		INSERT INTO sites (name, client_id, keycloak_realm, address, timezone, config_json)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, name, client_id, keycloak_realm, address, timezone, config_json, created_at`,
		s.Name, s.ClientID, s.KeycloakRealm, s.Address, tz, cfg,
	).Scan(&s.ID, &s.Name, &s.ClientID, &s.KeycloakRealm, &s.Address, &s.Timezone, &rawCfg, &s.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("CreateSite: %w", err)
	}
	s.ConfigJSON = json.RawMessage(rawCfg)
	return &s, nil
}

func scanSiteRows(rows pgx.Rows) ([]model.Site, error) {
	var sites []model.Site
	for rows.Next() {
		var s model.Site
		var rawCfg []byte
		if err := rows.Scan(&s.ID, &s.Name, &s.ClientID, &s.KeycloakRealm, &s.Address, &s.Timezone, &rawCfg, &s.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan site row: %w", err)
		}
		s.ConfigJSON = json.RawMessage(rawCfg)
		sites = append(sites, s)
	}
	return sites, rows.Err()
}
