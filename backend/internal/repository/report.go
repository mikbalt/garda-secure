package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// SessionSummaryRow is one patrol session with completion counts.
type SessionSummaryRow struct {
	SessionID        string     `json:"session_id"`
	RouteID          string     `json:"route_id"`
	RouteName        string     `json:"route_name"`
	GuardID          string     `json:"guard_id"`
	GuardName        string     `json:"guard_name"`
	StartedAt        time.Time  `json:"started_at"`
	EndedAt          *time.Time `json:"ended_at,omitempty"`
	Status           string     `json:"status"`
	TotalCheckpoints int        `json:"total_checkpoints"`
	ScannedCount     int        `json:"scanned_count"`
	SLABreaches      int        `json:"sla_breaches"`
	FlaggedScans     int        `json:"flagged_scans"` // scans with plausibility flags
	CompletionPct    float64    `json:"completion_pct"`
}

// SessionSummaries returns the patrol sessions of a site started in [from, to).
// Only scans of checkpoints that belong to the route count towards completion;
// the route is the session's snapshot (F9), else the current route.
func SessionSummaries(ctx context.Context, db DBTX, siteID string, from, to time.Time) ([]SessionSummaryRow, error) {
	rows, err := db.Query(ctx, `
		SELECT ps.id, r.id, r.name, g.id, g.name, ps.started_at, ps.ended_at, ps.status,
		       jsonb_array_length(COALESCE(ps.route_snapshot, r.checkpoints)) AS total,
		       (SELECT COUNT(DISTINCT cs.checkpoint_id) FROM checkpoint_scans cs
		         WHERE cs.session_id = ps.id
		           AND cs.checkpoint_id IN (SELECT cp->>'id'
		                FROM jsonb_array_elements(COALESCE(ps.route_snapshot, r.checkpoints)) cp)) AS scanned,
		       (SELECT COUNT(*) FROM sla_breaches b WHERE b.session_id = ps.id) AS breaches,
		       (SELECT COUNT(*) FROM checkpoint_scans cs WHERE cs.session_id = ps.id AND cardinality(cs.flags) > 0) AS flagged
		FROM patrol_sessions ps
		JOIN routes r ON r.id = ps.route_id
		JOIN guards g ON g.id = ps.guard_id
		WHERE ps.site_id = $1 AND ps.started_at >= $2 AND ps.started_at < $3
		ORDER BY ps.started_at DESC`, siteID, from, to)
	if err != nil {
		return nil, fmt.Errorf("SessionSummaries: %w", err)
	}
	return collect(rows, func(r pgx.Rows) (SessionSummaryRow, error) {
		var s SessionSummaryRow
		err := r.Scan(&s.SessionID, &s.RouteID, &s.RouteName, &s.GuardID, &s.GuardName, &s.StartedAt,
			&s.EndedAt, &s.Status, &s.TotalCheckpoints, &s.ScannedCount, &s.SLABreaches, &s.FlaggedScans)
		if s.TotalCheckpoints > 0 {
			s.CompletionPct = round1(float64(s.ScannedCount) / float64(s.TotalCheckpoints) * 100)
		}
		return s, err
	})
}

// MonthlyRecapRow is the per-guard payroll reference for a period.
type MonthlyRecapRow struct {
	GuardID           string  `json:"guard_id"`
	GuardName         string  `json:"guard_name"`
	EmployeeID        *string `json:"employee_id,omitempty"`
	ScheduledShifts   int     `json:"scheduled_shifts"`
	Present           int     `json:"present"`
	Late              int     `json:"late"`
	Absent            int     `json:"absent"`
	EarlyOut          int     `json:"early_out"`
	Pending           int     `json:"pending"`
	TotalHours        float64 `json:"total_hours"`
	PatrolSessions    int     `json:"patrol_sessions"`
	PatrolCompletePct float64 `json:"patrol_completion_pct"`
}

// MonthlyRecap aggregates attendance and patrol completion per guard for the
// shifts of a site starting in [from, to).
func MonthlyRecap(ctx context.Context, db DBTX, siteID string, from, to time.Time) ([]MonthlyRecapRow, error) {
	rows, err := db.Query(ctx, `
		WITH att AS (
		  SELECT s.guard_id,
		         COUNT(*)                                            AS scheduled,
		         COUNT(*) FILTER (WHERE a.status = 'present')        AS present,
		         COUNT(*) FILTER (WHERE a.status = 'late')           AS late,
		         COUNT(*) FILTER (WHERE a.status = 'absent')         AS absent,
		         COUNT(*) FILTER (WHERE a.early_out_minutes > 0)     AS early_out,
		         COUNT(*) FILTER (WHERE a.id IS NULL)                AS pending,
		         COALESCE(SUM(EXTRACT(EPOCH FROM (a.clock_out_at - a.clock_in_at))) / 3600, 0)::float8 AS hours
		  FROM shifts s
		  LEFT JOIN attendance_records a ON a.shift_id = s.id
		  WHERE s.site_id = $1 AND s.start_time >= $2 AND s.start_time < $3
		  GROUP BY s.guard_id
		),
		pat AS (
		  SELECT ps.guard_id,
		         COUNT(*) AS sessions,
		         AVG(LEAST(1.0,
		             (SELECT COUNT(DISTINCT cs.checkpoint_id) FROM checkpoint_scans cs WHERE cs.session_id = ps.id)::numeric
		             / NULLIF(jsonb_array_length(COALESCE(ps.route_snapshot, r.checkpoints)), 0))) * 100 AS pct
		  FROM patrol_sessions ps
		  JOIN routes r ON r.id = ps.route_id
		  WHERE ps.site_id = $1 AND ps.started_at >= $2 AND ps.started_at < $3
		  GROUP BY ps.guard_id
		)
		SELECT g.id, g.name, g.employee_id,
		       COALESCE(att.scheduled, 0), COALESCE(att.present, 0), COALESCE(att.late, 0),
		       COALESCE(att.absent, 0), COALESCE(att.early_out, 0), COALESCE(att.pending, 0),
		       COALESCE(att.hours, 0), COALESCE(pat.sessions, 0), COALESCE(pat.pct, 0)::float8
		FROM guards g
		LEFT JOIN att ON att.guard_id = g.id
		LEFT JOIN pat ON pat.guard_id = g.id
		WHERE att.guard_id IS NOT NULL OR pat.guard_id IS NOT NULL
		ORDER BY g.name`, siteID, from, to)
	if err != nil {
		return nil, fmt.Errorf("MonthlyRecap: %w", err)
	}
	return collect(rows, func(r pgx.Rows) (MonthlyRecapRow, error) {
		var m MonthlyRecapRow
		err := r.Scan(&m.GuardID, &m.GuardName, &m.EmployeeID, &m.ScheduledShifts, &m.Present, &m.Late,
			&m.Absent, &m.EarlyOut, &m.Pending, &m.TotalHours, &m.PatrolSessions, &m.PatrolCompletePct)
		m.TotalHours = round1(m.TotalHours)
		m.PatrolCompletePct = round1(m.PatrolCompletePct)
		return m, err
	})
}

// GuardKPIRow holds performance metrics for one guard over a period.
type GuardKPIRow struct {
	GuardID           string  `json:"guard_id"`
	GuardName         string  `json:"guard_name"`
	AttendanceRatePct float64 `json:"attendance_rate_pct"` // (present + late) / decided shifts
	OnTimeRatePct     float64 `json:"on_time_rate_pct"`    // present / (present + late)
	PatrolCompletePct float64 `json:"patrol_completion_pct"`
	IncidentsFiled    int     `json:"incidents_filed"`
	SLABreaches       int     `json:"sla_breaches"`
}

// GuardKPIs computes per-guard KPIs for a site over [from, to).
func GuardKPIs(ctx context.Context, db DBTX, siteID string, from, to time.Time) ([]GuardKPIRow, error) {
	recap, err := MonthlyRecap(ctx, db, siteID, from, to)
	if err != nil {
		return nil, err
	}
	rows, err := db.Query(ctx, `
		SELECT g.id,
		       (SELECT COUNT(*) FROM incidents i
		         WHERE i.guard_id = g.id AND i.site_id = $1 AND i.occurred_at >= $2 AND i.occurred_at < $3),
		       (SELECT COUNT(*) FROM sla_breaches b JOIN patrol_sessions ps ON ps.id = b.session_id
		         WHERE ps.guard_id = g.id AND ps.site_id = $1 AND ps.started_at >= $2 AND ps.started_at < $3)
		FROM guards g
		WHERE g.id = ANY($4)`, siteID, from, to, guardIDs(recap))
	if err != nil {
		return nil, fmt.Errorf("GuardKPIs: %w", err)
	}
	type counts struct{ incidents, breaches int }
	byGuard := map[string]counts{}
	_, err = collect(rows, func(r pgx.Rows) (struct{}, error) {
		var id string
		var c counts
		err := r.Scan(&id, &c.incidents, &c.breaches)
		byGuard[id] = c
		return struct{}{}, err
	})
	if err != nil {
		return nil, fmt.Errorf("GuardKPIs scan: %w", err)
	}

	out := make([]GuardKPIRow, 0, len(recap))
	for _, m := range recap {
		k := GuardKPIRow{GuardID: m.GuardID, GuardName: m.GuardName, PatrolCompletePct: m.PatrolCompletePct}
		if decided := m.Present + m.Late + m.Absent; decided > 0 {
			k.AttendanceRatePct = round1(float64(m.Present+m.Late) / float64(decided) * 100)
		}
		if attended := m.Present + m.Late; attended > 0 {
			k.OnTimeRatePct = round1(float64(m.Present) / float64(attended) * 100)
		}
		k.IncidentsFiled = byGuard[m.GuardID].incidents
		k.SLABreaches = byGuard[m.GuardID].breaches
		out = append(out, k)
	}
	return out, nil
}

// DashboardSummary is the headline data for the web dashboard.
type DashboardSummary struct {
	ShiftsToday         int            `json:"shifts_today"`
	AttendedToday       int            `json:"attended_today"`
	AbsentToday         int            `json:"absent_today"`
	ActivePatrols       int            `json:"active_patrols"`
	OpenIncidents       int            `json:"open_incidents"`
	UnackedSOS          int            `json:"sos_alerts_unacknowledged"`
	OpenDefects         int            `json:"open_defects"`
	PendingReview       int            `json:"attendance_pending_review"`
	WeeklyAttendance    []DailyCount   `json:"weekly_attendance"`
	IncidentsBySeverity map[string]int `json:"incidents_by_severity"`
}

// DailyCount is attendance for one day (site-local date).
type DailyCount struct {
	Date    string `json:"date"`
	Present int    `json:"present"`
	Late    int    `json:"late"`
	Absent  int    `json:"absent"`
}

// GetDashboardSummary computes the dashboard for a site. dayFrom/dayTo bound
// today in site time; weekFrom is the start of the 7-day window; tz is the
// site's IANA time zone used to bucket days.
func GetDashboardSummary(ctx context.Context, db DBTX, siteID string, dayFrom, dayTo, weekFrom time.Time, tz string) (*DashboardSummary, error) {
	d := &DashboardSummary{WeeklyAttendance: []DailyCount{}, IncidentsBySeverity: map[string]int{}}
	err := db.QueryRow(ctx, `
		SELECT
		  (SELECT COUNT(*) FROM shifts s WHERE s.site_id = $1 AND s.start_time >= $2 AND s.start_time < $3),
		  (SELECT COUNT(*) FROM shifts s JOIN attendance_records a ON a.shift_id = s.id
		    WHERE s.site_id = $1 AND s.start_time >= $2 AND s.start_time < $3 AND a.status IN ('present', 'late')),
		  (SELECT COUNT(*) FROM shifts s JOIN attendance_records a ON a.shift_id = s.id
		    WHERE s.site_id = $1 AND s.start_time >= $2 AND s.start_time < $3 AND a.status = 'absent'),
		  (SELECT COUNT(*) FROM patrol_sessions WHERE site_id = $1 AND status = 'in_progress'),
		  (SELECT COUNT(*) FROM incidents WHERE site_id = $1 AND status <> 'resolved'),
		  (SELECT COUNT(*) FROM sos_alerts WHERE site_id = $1 AND acknowledged_at IS NULL),
		  (SELECT COUNT(*) FROM defect_reports WHERE site_id = $1 AND status <> 'resolved'),
		  (SELECT COUNT(*) FROM attendance_records WHERE site_id = $1 AND needs_review)`,
		siteID, dayFrom, dayTo,
	).Scan(&d.ShiftsToday, &d.AttendedToday, &d.AbsentToday, &d.ActivePatrols, &d.OpenIncidents,
		&d.UnackedSOS, &d.OpenDefects, &d.PendingReview)
	if err != nil {
		return nil, fmt.Errorf("GetDashboardSummary: %w", err)
	}

	rows, err := db.Query(ctx, `
		SELECT to_char(s.start_time AT TIME ZONE $4, 'YYYY-MM-DD') AS day,
		       COUNT(*) FILTER (WHERE a.status = 'present'),
		       COUNT(*) FILTER (WHERE a.status = 'late'),
		       COUNT(*) FILTER (WHERE a.status = 'absent')
		FROM shifts s
		LEFT JOIN attendance_records a ON a.shift_id = s.id
		WHERE s.site_id = $1 AND s.start_time >= $2 AND s.start_time < $3
		GROUP BY day ORDER BY day`, siteID, weekFrom, dayTo, tz)
	if err != nil {
		return nil, fmt.Errorf("GetDashboardSummary weekly: %w", err)
	}
	d.WeeklyAttendance, err = collect(rows, func(r pgx.Rows) (DailyCount, error) {
		var c DailyCount
		err := r.Scan(&c.Date, &c.Present, &c.Late, &c.Absent)
		return c, err
	})
	if err != nil {
		return nil, err
	}

	rows, err = db.Query(ctx, `
		SELECT eff_severity, COUNT(*) FROM incidents_effective
		WHERE site_id = $1 AND status <> 'resolved' GROUP BY eff_severity`, siteID)
	if err != nil {
		return nil, fmt.Errorf("GetDashboardSummary severity: %w", err)
	}
	_, err = collect(rows, func(r pgx.Rows) (struct{}, error) {
		var sev string
		var n int
		err := r.Scan(&sev, &n)
		d.IncidentsBySeverity[sev] = n
		return struct{}{}, err
	})
	if err != nil {
		return nil, err
	}
	return d, nil
}

func guardIDs(rows []MonthlyRecapRow) []string {
	ids := make([]string, len(rows))
	for i, r := range rows {
		ids[i] = r.GuardID
	}
	return ids
}

func round1(v float64) float64 {
	if v < 0 {
		return -float64(int64(-v*10+0.5)) / 10
	}
	return float64(int64(v*10+0.5)) / 10
}
