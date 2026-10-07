package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/securepatrol/backend/internal/model"
)

// ── Incidents ────────────────────────────────────────────────────────────────

const incidentColumns = `id, site_id, guard_id, device_id, type, severity, description,
	photo_object_keys, location, status, occurred_at, created_at, updated_at,
	COALESCE((SELECT g.name FROM guards g WHERE g.id = incidents.guard_id), '')`

func scanIncident(row pgx.Row) (model.Incident, error) {
	var i model.Incident
	err := row.Scan(&i.ID, &i.SiteID, &i.GuardID, &i.DeviceID, &i.Type, &i.Severity, &i.Description,
		&i.PhotoObjectKeys, &i.Location, &i.Status, &i.OccurredAt, &i.CreatedAt, &i.UpdatedAt, &i.GuardName)
	if i.PhotoObjectKeys == nil {
		i.PhotoObjectKeys = []string{}
	}
	return i, err
}

func scanIncidentRows(rows pgx.Rows) (model.Incident, error) { return scanIncident(rows) }

// IncidentFilter narrows ListIncidents. Empty fields are ignored.
type IncidentFilter struct {
	SiteID   string
	From, To *time.Time
	Type     string
	Severity string
	Status   string
}

// ListIncidents returns the incidents of a site matching the filter, newest first.
func ListIncidents(ctx context.Context, db DBTX, f IncidentFilter) ([]model.Incident, error) {
	rows, err := db.Query(ctx, `
		SELECT `+incidentColumns+` FROM incidents
		WHERE site_id = $1
		  AND ($2::timestamptz IS NULL OR occurred_at >= $2)
		  AND ($3::timestamptz IS NULL OR occurred_at <  $3)
		  AND ($4 = '' OR type = $4)
		  AND ($5 = '' OR severity = $5)
		  AND ($6 = '' OR status = $6)
		ORDER BY occurred_at DESC
		LIMIT 1000`, f.SiteID, f.From, f.To, f.Type, f.Severity, f.Status)
	if err != nil {
		return nil, fmt.Errorf("ListIncidents: %w", err)
	}
	return collect(rows, scanIncidentRows)
}

// CreateIncident inserts an incident; idempotent on a client-supplied ID.
func CreateIncident(ctx context.Context, db DBTX, i model.Incident, clientID *string) (*model.Incident, bool, error) {
	out, err := scanIncident(db.QueryRow(ctx, `
		INSERT INTO incidents
		  (id, site_id, guard_id, device_id, type, severity, description, photo_object_keys,
		   location, status, occurred_at)
		VALUES (COALESCE($1::uuid, gen_random_uuid()), $2, $3, $4, $5, $6, $7, $8, $9, 'open', $10)
		ON CONFLICT (id) DO NOTHING
		RETURNING `+incidentColumns,
		clientID, i.SiteID, i.GuardID, i.DeviceID, i.Type, i.Severity, i.Description,
		i.PhotoObjectKeys, i.Location, i.OccurredAt))
	if err == nil {
		return &out, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) || clientID == nil {
		return nil, false, fmt.Errorf("CreateIncident: %w", err)
	}
	existing, err := GetIncident(ctx, db, *clientID)
	return existing, false, err
}

// GetIncident retrieves an incident by ID.
func GetIncident(ctx context.Context, db DBTX, id string) (*model.Incident, error) {
	i, err := scanIncident(db.QueryRow(ctx, `SELECT `+incidentColumns+` FROM incidents WHERE id = $1`, id))
	if err != nil {
		return nil, mapNoRows(err)
	}
	return &i, nil
}

// UpdateIncidentStatus moves an incident through its workflow.
func UpdateIncidentStatus(ctx context.Context, db DBTX, id, status string) (*model.Incident, error) {
	i, err := scanIncident(db.QueryRow(ctx, `
		UPDATE incidents SET status = $2, updated_at = now() WHERE id = $1
		RETURNING `+incidentColumns, id, status))
	if err != nil {
		return nil, mapNoRows(err)
	}
	return &i, nil
}

// ── Amendments ───────────────────────────────────────────────────────────────

// CreateAmendment stores a correction to an incident.
func CreateAmendment(ctx context.Context, db DBTX, incidentID string, changes []byte, reason, actor string) (*model.IncidentAmendment, error) {
	var a model.IncidentAmendment
	err := db.QueryRow(ctx, `
		INSERT INTO incident_amendments (incident_id, changes, reason, amended_by)
		VALUES ($1, $2, $3, $4)
		RETURNING id, incident_id, changes, reason, amended_by, created_at`,
		incidentID, changes, reason, actor,
	).Scan(&a.ID, &a.IncidentID, &a.Changes, &a.Reason, &a.AmendedBy, &a.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("CreateAmendment: %w", err)
	}
	return &a, nil
}

// ListAmendments returns the amendments of an incident, oldest first.
func ListAmendments(ctx context.Context, db DBTX, incidentID string) ([]model.IncidentAmendment, error) {
	rows, err := db.Query(ctx, `
		SELECT id, incident_id, changes, reason, amended_by, created_at
		FROM incident_amendments WHERE incident_id = $1 ORDER BY created_at`, incidentID)
	if err != nil {
		return nil, fmt.Errorf("ListAmendments: %w", err)
	}
	return collect(rows, func(r pgx.Rows) (model.IncidentAmendment, error) {
		var a model.IncidentAmendment
		err := r.Scan(&a.ID, &a.IncidentID, &a.Changes, &a.Reason, &a.AmendedBy, &a.CreatedAt)
		return a, err
	})
}

// ── SOS ──────────────────────────────────────────────────────────────────────

const sosColumns = `id, guard_id, site_id, device_id, fired_at, received_at, gps_lat, gps_lng,
	acknowledged_by, acknowledged_at,
	COALESCE((SELECT g.name FROM guards g WHERE g.id = sos_alerts.guard_id), '')`

func scanSos(row pgx.Row) (model.SosAlert, error) {
	var a model.SosAlert
	err := row.Scan(&a.ID, &a.GuardID, &a.SiteID, &a.DeviceID, &a.FiredAt, &a.ReceivedAt,
		&a.GPSLat, &a.GPSLng, &a.AcknowledgedBy, &a.AcknowledgedAt, &a.GuardName)
	return a, err
}

func scanSosRows(rows pgx.Rows) (model.SosAlert, error) { return scanSos(rows) }

// CreateSosAlert inserts an SOS alert. Tablets retry SOS every few seconds
// until confirmed, so a client-supplied ID makes the retries collapse into
// one alert (created=false on repeats).
func CreateSosAlert(ctx context.Context, db DBTX, a model.SosAlert, clientID *string) (*model.SosAlert, bool, error) {
	out, err := scanSos(db.QueryRow(ctx, `
		INSERT INTO sos_alerts (id, guard_id, site_id, device_id, fired_at, gps_lat, gps_lng)
		VALUES (COALESCE($1::uuid, gen_random_uuid()), $2, $3, $4, $5, $6, $7)
		ON CONFLICT (id) DO NOTHING
		RETURNING `+sosColumns,
		clientID, a.GuardID, a.SiteID, a.DeviceID, a.FiredAt, a.GPSLat, a.GPSLng))
	if err == nil {
		return &out, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) || clientID == nil {
		return nil, false, fmt.Errorf("CreateSosAlert: %w", err)
	}
	existing, err := GetSosAlert(ctx, db, *clientID)
	return existing, false, err
}

// GetSosAlert retrieves an SOS alert by ID.
func GetSosAlert(ctx context.Context, db DBTX, id string) (*model.SosAlert, error) {
	a, err := scanSos(db.QueryRow(ctx, `SELECT `+sosColumns+` FROM sos_alerts WHERE id = $1`, id))
	if err != nil {
		return nil, mapNoRows(err)
	}
	return &a, nil
}

// ListSosAlerts returns the SOS alerts of a site, newest first.
func ListSosAlerts(ctx context.Context, db DBTX, siteID string, unackOnly bool) ([]model.SosAlert, error) {
	rows, err := db.Query(ctx, `
		SELECT `+sosColumns+` FROM sos_alerts
		WHERE site_id = $1 AND (NOT $2 OR acknowledged_at IS NULL)
		ORDER BY fired_at DESC
		LIMIT 200`, siteID, unackOnly)
	if err != nil {
		return nil, fmt.Errorf("ListSosAlerts: %w", err)
	}
	return collect(rows, scanSosRows)
}

// AcknowledgeSosAlert marks an alert acknowledged; already-acknowledged alerts
// keep their original acknowledger.
func AcknowledgeSosAlert(ctx context.Context, db DBTX, id, actor string) (*model.SosAlert, error) {
	a, err := scanSos(db.QueryRow(ctx, `
		UPDATE sos_alerts
		SET acknowledged_by = COALESCE(acknowledged_by, $2),
		    acknowledged_at = COALESCE(acknowledged_at, now())
		WHERE id = $1
		RETURNING `+sosColumns, id, actor))
	if err != nil {
		return nil, mapNoRows(err)
	}
	return &a, nil
}

// ── Defect reports ───────────────────────────────────────────────────────────

const defectColumns = `id, site_id, guard_id, device_id, defect_type, location, description,
	photo_object_key, status, created_at, updated_at,
	COALESCE((SELECT g.name FROM guards g WHERE g.id = defect_reports.guard_id), '')`

func scanDefect(row pgx.Row) (model.DefectReport, error) {
	var d model.DefectReport
	err := row.Scan(&d.ID, &d.SiteID, &d.GuardID, &d.DeviceID, &d.DefectType, &d.Location,
		&d.Description, &d.PhotoObjectKey, &d.Status, &d.CreatedAt, &d.UpdatedAt, &d.GuardName)
	return d, err
}

func scanDefectRows(rows pgx.Rows) (model.DefectReport, error) { return scanDefect(rows) }

// CreateDefect inserts a defect report; idempotent on a client-supplied ID.
func CreateDefect(ctx context.Context, db DBTX, d model.DefectReport, clientID *string) (*model.DefectReport, bool, error) {
	out, err := scanDefect(db.QueryRow(ctx, `
		INSERT INTO defect_reports
		  (id, site_id, guard_id, device_id, defect_type, location, description, photo_object_key, created_at)
		VALUES (COALESCE($1::uuid, gen_random_uuid()), $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (id) DO NOTHING
		RETURNING `+defectColumns,
		clientID, d.SiteID, d.GuardID, d.DeviceID, d.DefectType, d.Location, d.Description,
		d.PhotoObjectKey, d.CreatedAt))
	if err == nil {
		return &out, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) || clientID == nil {
		return nil, false, fmt.Errorf("CreateDefect: %w", err)
	}
	existing, err := GetDefect(ctx, db, *clientID)
	return existing, false, err
}

// GetDefect retrieves a defect report by ID.
func GetDefect(ctx context.Context, db DBTX, id string) (*model.DefectReport, error) {
	d, err := scanDefect(db.QueryRow(ctx, `SELECT `+defectColumns+` FROM defect_reports WHERE id = $1`, id))
	if err != nil {
		return nil, mapNoRows(err)
	}
	return &d, nil
}

// ListDefects returns the defect reports of a site, optionally by status.
func ListDefects(ctx context.Context, db DBTX, siteID, status string) ([]model.DefectReport, error) {
	rows, err := db.Query(ctx, `
		SELECT `+defectColumns+` FROM defect_reports
		WHERE site_id = $1 AND ($2 = '' OR status = $2)
		ORDER BY created_at DESC
		LIMIT 500`, siteID, status)
	if err != nil {
		return nil, fmt.Errorf("ListDefects: %w", err)
	}
	return collect(rows, scanDefectRows)
}

// UpdateDefectStatus moves a defect through Open → In Progress → Resolved.
func UpdateDefectStatus(ctx context.Context, db DBTX, id, status string) (*model.DefectReport, error) {
	d, err := scanDefect(db.QueryRow(ctx, `
		UPDATE defect_reports SET status = $2, updated_at = now() WHERE id = $1
		RETURNING `+defectColumns, id, status))
	if err != nil {
		return nil, mapNoRows(err)
	}
	return &d, nil
}

// ── Visitor logs ─────────────────────────────────────────────────────────────

const visitorColumns = `id, site_id, guard_id, device_id, visitor_name, id_type, id_number, plate,
	host_name, entry_at, exit_at, purpose, photo_object_key, created_at,
	COALESCE((SELECT g.name FROM guards g WHERE g.id = visitor_logs.guard_id), '')`

func scanVisitor(row pgx.Row) (model.VisitorLog, error) {
	var v model.VisitorLog
	err := row.Scan(&v.ID, &v.SiteID, &v.GuardID, &v.DeviceID, &v.VisitorName, &v.IDType, &v.IDNumber,
		&v.Plate, &v.HostName, &v.EntryAt, &v.ExitAt, &v.Purpose, &v.PhotoObjectKey, &v.CreatedAt, &v.GuardName)
	return v, err
}

func scanVisitorRows(rows pgx.Rows) (model.VisitorLog, error) { return scanVisitor(rows) }

// CreateVisitor inserts a visitor log; idempotent on a client-supplied ID.
func CreateVisitor(ctx context.Context, db DBTX, v model.VisitorLog, clientID *string) (*model.VisitorLog, bool, error) {
	out, err := scanVisitor(db.QueryRow(ctx, `
		INSERT INTO visitor_logs
		  (id, site_id, guard_id, device_id, visitor_name, id_type, id_number, plate, host_name,
		   entry_at, exit_at, purpose, photo_object_key)
		VALUES (COALESCE($1::uuid, gen_random_uuid()), $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		ON CONFLICT (id) DO NOTHING
		RETURNING `+visitorColumns,
		clientID, v.SiteID, v.GuardID, v.DeviceID, v.VisitorName, v.IDType, v.IDNumber, v.Plate,
		v.HostName, v.EntryAt, v.ExitAt, v.Purpose, v.PhotoObjectKey))
	if err == nil {
		return &out, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) || clientID == nil {
		return nil, false, fmt.Errorf("CreateVisitor: %w", err)
	}
	existing, err := GetVisitor(ctx, db, *clientID)
	return existing, false, err
}

// GetVisitor retrieves a visitor log by ID.
func GetVisitor(ctx context.Context, db DBTX, id string) (*model.VisitorLog, error) {
	v, err := scanVisitor(db.QueryRow(ctx, `SELECT `+visitorColumns+` FROM visitor_logs WHERE id = $1`, id))
	if err != nil {
		return nil, mapNoRows(err)
	}
	return &v, nil
}

// ListVisitors returns the visitor logs of a site with entry in [from, to).
func ListVisitors(ctx context.Context, db DBTX, siteID string, from, to time.Time) ([]model.VisitorLog, error) {
	rows, err := db.Query(ctx, `
		SELECT `+visitorColumns+` FROM visitor_logs
		WHERE site_id = $1 AND COALESCE(entry_at, created_at) >= $2 AND COALESCE(entry_at, created_at) < $3
		ORDER BY COALESCE(entry_at, created_at) DESC
		LIMIT 1000`, siteID, from, to)
	if err != nil {
		return nil, fmt.Errorf("ListVisitors: %w", err)
	}
	return collect(rows, scanVisitorRows)
}

// SetVisitorExit records the exit time; an existing exit time is kept.
func SetVisitorExit(ctx context.Context, db DBTX, id string, at time.Time) (*model.VisitorLog, error) {
	v, err := scanVisitor(db.QueryRow(ctx, `
		UPDATE visitor_logs SET exit_at = COALESCE(exit_at, $2) WHERE id = $1
		RETURNING `+visitorColumns, id, at))
	if err != nil {
		return nil, mapNoRows(err)
	}
	return &v, nil
}
