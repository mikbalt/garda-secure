package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/securepatrol/backend/internal/model"
)

// ListIncidents returns incidents filtered by siteID, status, and type.
func ListIncidents(ctx context.Context, pool *pgxpool.Pool, siteID, status, incType string) ([]model.Incident, error) {
	query := `SELECT id, site_id, guard_id, type, severity, description,
	                 photo_object_keys, location, status, created_at, updated_at
	          FROM incidents WHERE 1=1`
	args := []any{}

	if siteID != "" {
		args = append(args, siteID)
		query += fmt.Sprintf(" AND site_id = $%d", len(args))
	}
	if status != "" {
		args = append(args, status)
		query += fmt.Sprintf(" AND status = $%d", len(args))
	}
	if incType != "" {
		args = append(args, incType)
		query += fmt.Sprintf(" AND type = $%d", len(args))
	}
	query += " ORDER BY created_at DESC"

	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("ListIncidents: %w", err)
	}
	defer rows.Close()

	var incidents []model.Incident
	for rows.Next() {
		var i model.Incident
		if err := rows.Scan(&i.ID, &i.SiteID, &i.GuardID, &i.Type, &i.Severity,
			&i.Description, &i.PhotoObjectKeys, &i.Location, &i.Status,
			&i.CreatedAt, &i.UpdatedAt); err != nil {
			return nil, fmt.Errorf("ListIncidents scan: %w", err)
		}
		incidents = append(incidents, i)
	}
	return incidents, rows.Err()
}

// CreateIncident inserts a new incident record.
func CreateIncident(ctx context.Context, pool *pgxpool.Pool, inc model.Incident) (*model.Incident, error) {
	err := pool.QueryRow(ctx,
		`INSERT INTO incidents (site_id, guard_id, type, severity, description, photo_object_keys, location, status)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 RETURNING id, site_id, guard_id, type, severity, description,
		           photo_object_keys, location, status, created_at, updated_at`,
		inc.SiteID, inc.GuardID, inc.Type, inc.Severity,
		inc.Description, inc.PhotoObjectKeys, inc.Location, inc.Status,
	).Scan(&inc.ID, &inc.SiteID, &inc.GuardID, &inc.Type, &inc.Severity,
		&inc.Description, &inc.PhotoObjectKeys, &inc.Location, &inc.Status,
		&inc.CreatedAt, &inc.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("CreateIncident: %w", err)
	}
	return &inc, nil
}

// GetIncidentByID retrieves a single incident.
func GetIncidentByID(ctx context.Context, pool *pgxpool.Pool, id string) (*model.Incident, error) {
	var i model.Incident
	err := pool.QueryRow(ctx,
		`SELECT id, site_id, guard_id, type, severity, description,
		        photo_object_keys, location, status, created_at, updated_at
		 FROM incidents WHERE id = $1`,
		id,
	).Scan(&i.ID, &i.SiteID, &i.GuardID, &i.Type, &i.Severity,
		&i.Description, &i.PhotoObjectKeys, &i.Location, &i.Status,
		&i.CreatedAt, &i.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("GetIncidentByID: %w", err)
	}
	return &i, nil
}

// UpdateIncidentStatus updates the status of an incident.
func UpdateIncidentStatus(ctx context.Context, pool *pgxpool.Pool, id, status string) error {
	_, err := pool.Exec(ctx,
		`UPDATE incidents SET status = $1, updated_at = now() WHERE id = $2`,
		status, id,
	)
	return err
}

// CreateSosAlert inserts a new SOS alert.
func CreateSosAlert(ctx context.Context, pool *pgxpool.Pool, alert model.SosAlert) (*model.SosAlert, error) {
	err := pool.QueryRow(ctx,
		`INSERT INTO sos_alerts (guard_id, site_id, fired_at, gps_lat, gps_lng)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, guard_id, site_id, fired_at, gps_lat, gps_lng,
		           acknowledged_by, acknowledged_at`,
		alert.GuardID, alert.SiteID, time.Now().UTC(), alert.GPSLat, alert.GPSLng,
	).Scan(&alert.ID, &alert.GuardID, &alert.SiteID, &alert.FiredAt,
		&alert.GPSLat, &alert.GPSLng, &alert.AcknowledgedBy, &alert.AcknowledgedAt)
	if err != nil {
		return nil, fmt.Errorf("CreateSosAlert: %w", err)
	}
	return &alert, nil
}

// ListSosAlerts returns SOS alerts for a site, optionally only unacknowledged.
func ListSosAlerts(ctx context.Context, pool *pgxpool.Pool, siteID string, unackOnly bool) ([]model.SosAlert, error) {
	query := `SELECT id, guard_id, site_id, fired_at, gps_lat, gps_lng,
	                 acknowledged_by, acknowledged_at
	          FROM sos_alerts WHERE site_id = $1`
	if unackOnly {
		query += " AND acknowledged_at IS NULL"
	}
	query += " ORDER BY fired_at DESC"

	rows, err := pool.Query(ctx, query, siteID)
	if err != nil {
		return nil, fmt.Errorf("ListSosAlerts: %w", err)
	}
	defer rows.Close()

	var alerts []model.SosAlert
	for rows.Next() {
		var a model.SosAlert
		if err := rows.Scan(&a.ID, &a.GuardID, &a.SiteID, &a.FiredAt,
			&a.GPSLat, &a.GPSLng, &a.AcknowledgedBy, &a.AcknowledgedAt); err != nil {
			return nil, fmt.Errorf("ListSosAlerts scan: %w", err)
		}
		alerts = append(alerts, a)
	}
	return alerts, rows.Err()
}

// AcknowledgeSosAlert marks an SOS alert as acknowledged.
func AcknowledgeSosAlert(ctx context.Context, pool *pgxpool.Pool, id, acknowledgedBy string) error {
	_, err := pool.Exec(ctx,
		`UPDATE sos_alerts SET acknowledged_by = $1, acknowledged_at = now() WHERE id = $2`,
		acknowledgedBy, id,
	)
	return err
}
