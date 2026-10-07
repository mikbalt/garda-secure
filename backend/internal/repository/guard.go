// Package repository provides data-access functions for guard records.
package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/securepatrol/backend/internal/model"
)

// ListGuards returns all guards, optionally filtered by site assignment.
func ListGuards(ctx context.Context, pool *pgxpool.Pool, siteID string) ([]model.Guard, error) {
	query := `SELECT id, keycloak_user_id, name, employee_id, role, status, created_at, updated_at FROM guards WHERE 1=1`
	args := []any{}
	if siteID != "" {
		query += fmt.Sprintf(" AND id IN (SELECT DISTINCT guard_id FROM shifts WHERE site_id = $%d)", len(args)+1)
		args = append(args, siteID)
	}
	query += " ORDER BY name"

	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("ListGuards query: %w", err)
	}
	defer rows.Close()

	var guards []model.Guard
	for rows.Next() {
		var g model.Guard
		if err := rows.Scan(&g.ID, &g.KeycloakUserID, &g.Name, &g.EmployeeID, &g.Role, &g.Status, &g.CreatedAt, &g.UpdatedAt); err != nil {
			return nil, fmt.Errorf("ListGuards scan: %w", err)
		}
		guards = append(guards, g)
	}
	return guards, rows.Err()
}

// GetGuardByID retrieves a single guard by ID.
func GetGuardByID(ctx context.Context, pool *pgxpool.Pool, id string) (*model.Guard, error) {
	var g model.Guard
	err := pool.QueryRow(ctx,
		`SELECT id, keycloak_user_id, name, employee_id, role, status, created_at, updated_at FROM guards WHERE id = $1`,
		id,
	).Scan(&g.ID, &g.KeycloakUserID, &g.Name, &g.EmployeeID, &g.Role, &g.Status, &g.CreatedAt, &g.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("GetGuardByID: %w", err)
	}
	return &g, nil
}

// CreateGuard inserts a new guard record.
func CreateGuard(ctx context.Context, pool *pgxpool.Pool, g model.Guard) (*model.Guard, error) {
	err := pool.QueryRow(ctx,
		`INSERT INTO guards (keycloak_user_id, name, employee_id, role, status)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, keycloak_user_id, name, employee_id, role, status, created_at, updated_at`,
		g.KeycloakUserID, g.Name, g.EmployeeID, g.Role, g.Status,
	).Scan(&g.ID, &g.KeycloakUserID, &g.Name, &g.EmployeeID, &g.Role, &g.Status, &g.CreatedAt, &g.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("CreateGuard: %w", err)
	}
	return &g, nil
}

// UpdateGuard updates the specified fields of a guard.
// Allowed keys in fields: "name", "role", "status", "employee_id", "keycloak_user_id".
func UpdateGuard(ctx context.Context, pool *pgxpool.Pool, id string, fields map[string]any) (*model.Guard, error) {
	allowed := map[string]bool{
		"name": true, "role": true, "status": true,
		"employee_id": true, "keycloak_user_id": true,
	}

	setClauses := []string{}
	args := []any{}
	for k, v := range fields {
		if !allowed[k] {
			continue
		}
		args = append(args, v)
		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", k, len(args)))
	}
	if len(setClauses) == 0 {
		return GetGuardByID(ctx, pool, id)
	}

	args = append(args, id)
	query := fmt.Sprintf(
		`UPDATE guards SET %s, updated_at = now() WHERE id = $%d
		 RETURNING id, keycloak_user_id, name, employee_id, role, status, created_at, updated_at`,
		strings.Join(setClauses, ", "), len(args),
	)

	var g model.Guard
	err := pool.QueryRow(ctx, query, args...).
		Scan(&g.ID, &g.KeycloakUserID, &g.Name, &g.EmployeeID, &g.Role, &g.Status, &g.CreatedAt, &g.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("UpdateGuard: %w", err)
	}
	return &g, nil
}
