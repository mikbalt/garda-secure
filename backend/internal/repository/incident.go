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

// incidentCols selects an incident row. From the incidents_effective view
// (reads) type/severity/description/location are the amended values and the
// filed ones follow; from incidents itself (RETURNING of an insert) both are
// the filed values and amended is false.
func incidentCols(src string) string {
	eff, amended := "eff_", "amended"
	if src == "incidents" {
		eff, amended = "", "false"
	}
	return `id, site_id, guard_id, device_id, ` + eff + `type, ` + eff + `severity, ` + eff + `description,
	photo_object_keys, ` + eff + `location, status, occurred_at, created_at, updated_at, received_at, flags,
	COALESCE((SELECT g.name FROM guards g WHERE g.id = ` + src + `.guard_id), ''),
	type, severity, description, location, ` + amended
}

var (
	incidentColumns   = incidentCols("incidents_effective")
	incidentRetColumn = incidentCols("incidents")
)

func scanIncident(row pgx.Row) (model.Incident, error) {
	var i model.Incident
	var origType, origSeverity string
	var origDescription, origLocation *string
	err := row.Scan(&i.ID, &i.SiteID, &i.GuardID, &i.DeviceID, &i.Type, &i.Severity, &i.Description,
		&i.PhotoObjectKeys, &i.Location, &i.Status, &i.OccurredAt, &i.CreatedAt, &i.UpdatedAt,
		&i.ReceivedAt, &i.Flags, &i.GuardName, &origType, &origSeverity, &origDescription, &origLocation,
		&i.Amended)
	if i.PhotoObjectKeys == nil {
		i.PhotoObjectKeys = []string{}
	}
	if i.Flags == nil {
		i.Flags = []string{}
	}
	if i.Amended {
		if origType != i.Type {
			i.OriginalType = &origType
		}
		if origSeverity != i.Severity {
			i.OriginalSeverity = &origSeverity
		}
		if !equalPtr(origDescription, i.Description) {
			i.OriginalDescription = origDescription
		}
		if !equalPtr(origLocation, i.Location) {
			i.OriginalLocation = origLocation
		}
	}
	return i, err
}

func equalPtr(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func scanIncidentRows(rows pgx.Rows) (model.Incident, error) { return scanIncident(rows) }

// IncidentFilter narrows ListIncidents. Empty fields are ignored. Type and
// severity match the effective (amended) values (F7).
type IncidentFilter struct {
	SiteID   string
	From, To *time.Time
	Type     string
	Severity string
	Status   string
}

const incidentWhere = `
		WHERE site_id = $1
		  AND ($2::timestamptz IS NULL OR occurred_at >= $2)
		  AND ($3::timestamptz IS NULL OR occurred_at <  $3)
		  AND ($4 = '' OR eff_type = $4)
		  AND ($5 = '' OR eff_severity = $5)
		  AND ($6 = '' OR status = $6)`

// CountIncidents returns how many incidents match the filter.
func CountIncidents(ctx context.Context, db DBTX, f IncidentFilter) (int, error) {
	var n int
	err := db.QueryRow(ctx, `SELECT count(*) FROM incidents_effective`+incidentWhere,
		f.SiteID, f.From, f.To, f.Type, f.Severity, f.Status).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("CountIncidents: %w", err)
	}
	return n, nil
}

// ListIncidents returns one page of the incidents of a site matching the
// filter, newest first.
func ListIncidents(ctx context.Context, db DBTX, f IncidentFilter, p Page) ([]model.Incident, error) {
	rows, err := db.Query(ctx, `
		SELECT `+incidentColumns+` FROM incidents_effective`+incidentWhere+`
		ORDER BY occurred_at DESC, id
		LIMIT $7 OFFSET $8`, f.SiteID, f.From, f.To, f.Type, f.Severity, f.Status, p.limit(), p.Offset)
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
		   location, status, occurred_at, flags)
		VALUES (COALESCE($1::uuid, gen_random_uuid()), $2, $3, $4, $5, $6, $7, $8, $9, 'open', $10, $11)
		ON CONFLICT (id) DO NOTHING
		RETURNING `+incidentRetColumn,
		clientID, i.SiteID, i.GuardID, i.DeviceID, i.Type, i.Severity, i.Description,
		i.PhotoObjectKeys, i.Location, i.OccurredAt, flagsOrEmpty(i.Flags)))
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
	i, err := scanIncident(db.QueryRow(ctx, `SELECT `+incidentColumns+` FROM incidents_effective WHERE id = $1`, id))
	if err != nil {
		return nil, mapNoRows(err)
	}
	return &i, nil
}

// UpdateIncidentStatus moves an incident through its workflow.
func UpdateIncidentStatus(ctx context.Context, db DBTX, id, status string) (*model.Incident, error) {
	tag, err := db.Exec(ctx, `UPDATE incidents SET status = $2, updated_at = now() WHERE id = $1`, id, status)
	if err != nil {
		return nil, fmt.Errorf("UpdateIncidentStatus: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrNotFound
	}
	return GetIncident(ctx, db, id)
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
	acknowledged_by, acknowledged_at, flags,
	COALESCE((SELECT g.name FROM guards g WHERE g.id = sos_alerts.guard_id), '')`

func scanSos(row pgx.Row) (model.SosAlert, error) {
	var a model.SosAlert
	err := row.Scan(&a.ID, &a.GuardID, &a.SiteID, &a.DeviceID, &a.FiredAt, &a.ReceivedAt,
		&a.GPSLat, &a.GPSLng, &a.AcknowledgedBy, &a.AcknowledgedAt, &a.Flags, &a.GuardName)
	if a.Flags == nil {
		a.Flags = []string{}
	}
	return a, err
}

func scanSosRows(rows pgx.Rows) (model.SosAlert, error) { return scanSos(rows) }

// CreateSosAlert inserts an SOS alert. Tablets retry SOS every few seconds
// until confirmed, so a client-supplied ID makes the retries collapse into
// one alert (created=false on repeats).
func CreateSosAlert(ctx context.Context, db DBTX, a model.SosAlert, clientID *string) (*model.SosAlert, bool, error) {
	out, err := scanSos(db.QueryRow(ctx, `
		INSERT INTO sos_alerts (id, guard_id, site_id, device_id, fired_at, gps_lat, gps_lng, flags)
		VALUES (COALESCE($1::uuid, gen_random_uuid()), $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (id) DO NOTHING
		RETURNING `+sosColumns,
		clientID, a.GuardID, a.SiteID, a.DeviceID, a.FiredAt, a.GPSLat, a.GPSLng, flagsOrEmpty(a.Flags)))
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

// ListSosAlerts returns the latest 200 SOS alerts of a site, newest first.
func ListSosAlerts(ctx context.Context, db DBTX, siteID string, unackOnly bool) ([]model.SosAlert, error) {
	rows, _, err := ListSosAlertsPage(ctx, db, siteID, unackOnly, Page{Limit: 200})
	return rows, err
}

// ListSosAlertsPage returns one page of a site's SOS alerts, newest first,
// and how many match.
func ListSosAlertsPage(ctx context.Context, db DBTX, siteID string, unackOnly bool, p Page) ([]model.SosAlert, int, error) {
	var total int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM sos_alerts WHERE site_id = $1 AND (NOT $2 OR acknowledged_at IS NULL)`,
		siteID, unackOnly).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("ListSosAlerts count: %w", err)
	}
	rows, err := db.Query(ctx, `
		SELECT `+sosColumns+` FROM sos_alerts
		WHERE site_id = $1 AND (NOT $2 OR acknowledged_at IS NULL)
		ORDER BY fired_at DESC, id
		LIMIT $3 OFFSET $4`, siteID, unackOnly, p.limit(), p.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("ListSosAlerts: %w", err)
	}
	out, err := collect(rows, scanSosRows)
	return out, total, err
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

// ListDefects returns one page of the defect reports of a site, optionally
// by status, newest first, and how many match.
func ListDefects(ctx context.Context, db DBTX, siteID, status string, p Page) ([]model.DefectReport, int, error) {
	var total int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM defect_reports WHERE site_id = $1 AND ($2 = '' OR status = $2)`,
		siteID, status).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("ListDefects count: %w", err)
	}
	rows, err := db.Query(ctx, `
		SELECT `+defectColumns+` FROM defect_reports
		WHERE site_id = $1 AND ($2 = '' OR status = $2)
		ORDER BY created_at DESC, id
		LIMIT $3 OFFSET $4`, siteID, status, p.limit(), p.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("ListDefects: %w", err)
	}
	out, err := collect(rows, scanDefectRows)
	return out, total, err
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

// ListVisitors returns one page of the visitor logs of a site with entry in
// [from, to), newest first, and how many match.
func ListVisitors(ctx context.Context, db DBTX, siteID string, from, to time.Time, p Page) ([]model.VisitorLog, int, error) {
	var total int
	if err := db.QueryRow(ctx, `
		SELECT count(*) FROM visitor_logs
		WHERE site_id = $1 AND COALESCE(entry_at, created_at) >= $2 AND COALESCE(entry_at, created_at) < $3`,
		siteID, from, to).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("ListVisitors count: %w", err)
	}
	rows, err := db.Query(ctx, `
		SELECT `+visitorColumns+` FROM visitor_logs
		WHERE site_id = $1 AND COALESCE(entry_at, created_at) >= $2 AND COALESCE(entry_at, created_at) < $3
		ORDER BY COALESCE(entry_at, created_at) DESC, id
		LIMIT $4 OFFSET $5`, siteID, from, to, p.limit(), p.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("ListVisitors: %w", err)
	}
	out, err := collect(rows, scanVisitorRows)
	return out, total, err
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
