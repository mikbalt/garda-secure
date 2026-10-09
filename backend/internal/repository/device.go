package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/securepatrol/backend/internal/model"
)

const deviceColumns = `id, keycloak_realm, keycloak_user_id, name, site_id, status,
	approved_by, approved_at, last_seen_at, created_at, updated_at`

func scanDevice(row pgx.Row) (model.Device, error) {
	var d model.Device
	err := row.Scan(&d.ID, &d.KeycloakRealm, &d.KeycloakUserID, &d.Name, &d.SiteID, &d.Status,
		&d.ApprovedBy, &d.ApprovedAt, &d.LastSeenAt, &d.CreatedAt, &d.UpdatedAt)
	return d, err
}

func scanDeviceRows(rows pgx.Rows) (model.Device, error) { return scanDevice(rows) }

// RegisterDevice creates a pending device for a Keycloak device account, or
// returns the existing one (renaming it only while still pending).
func RegisterDevice(ctx context.Context, db DBTX, realm, userID, name string) (*model.Device, error) {
	d, err := scanDevice(db.QueryRow(ctx, `
		INSERT INTO devices (keycloak_realm, keycloak_user_id, name)
		VALUES ($1, $2, $3)
		ON CONFLICT (keycloak_realm, keycloak_user_id) DO UPDATE
		  SET name = CASE WHEN devices.status = 'pending' THEN EXCLUDED.name ELSE devices.name END,
		      updated_at = now()
		RETURNING `+deviceColumns, realm, userID, name))
	if err != nil {
		return nil, fmt.Errorf("RegisterDevice: %w", err)
	}
	return &d, nil
}

// GetDeviceByIdentity looks a device up by its Keycloak account.
func GetDeviceByIdentity(ctx context.Context, db DBTX, realm, userID string) (*model.Device, error) {
	d, err := scanDevice(db.QueryRow(ctx,
		`SELECT `+deviceColumns+` FROM devices WHERE keycloak_realm = $1 AND keycloak_user_id = $2`,
		realm, userID))
	if err != nil {
		return nil, mapNoRows(err)
	}
	return &d, nil
}

// GetDevice retrieves a device by ID.
func GetDevice(ctx context.Context, db DBTX, id string) (*model.Device, error) {
	d, err := scanDevice(db.QueryRow(ctx, `SELECT `+deviceColumns+` FROM devices WHERE id = $1`, id))
	if err != nil {
		return nil, mapNoRows(err)
	}
	return &d, nil
}

// ListDevicesByRealm returns every device registered in a realm.
func ListDevicesByRealm(ctx context.Context, db DBTX, realm string) ([]model.Device, error) {
	rows, err := db.Query(ctx,
		`SELECT `+deviceColumns+` FROM devices WHERE keycloak_realm = $1 ORDER BY status, name`, realm)
	if err != nil {
		return nil, fmt.Errorf("ListDevicesByRealm: %w", err)
	}
	return collect(rows, scanDeviceRows)
}

// UpdateDevice sets name, site and status; approval metadata is stamped the
// first time a device becomes active.
func UpdateDevice(ctx context.Context, db DBTX, id, name string, siteID *string, status, actor string) (*model.Device, error) {
	d, err := scanDevice(db.QueryRow(ctx, `
		UPDATE devices
		SET name = $2, site_id = $3, status = $4, updated_at = now(),
		    approved_by = CASE WHEN $4 = 'active' AND approved_at IS NULL THEN $5 ELSE approved_by END,
		    approved_at = CASE WHEN $4 = 'active' AND approved_at IS NULL THEN now() ELSE approved_at END
		WHERE id = $1
		RETURNING `+deviceColumns, id, name, siteID, status, actor))
	if err != nil {
		return nil, mapNoRows(err)
	}
	return &d, nil
}

// TouchDevice records that a device was just seen. Writes are throttled to
// one per minute to avoid a row update on every request. A gap of more than
// 3 minutes starts a new online period (online_since), which the SLA check
// uses to tell "offline" from "missed" (audit F2).
func TouchDevice(ctx context.Context, db DBTX, id string) error {
	_, err := db.Exec(ctx, `
		UPDATE devices SET last_seen_at = now(),
		       online_since = CASE
		         WHEN online_since IS NULL OR last_seen_at IS NULL
		              OR last_seen_at < now() - interval '3 minutes' THEN now()
		         ELSE online_since END
		WHERE id = $1 AND (last_seen_at IS NULL OR last_seen_at < now() - interval '1 minute')`, id)
	return err
}
