package repository

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// AttendanceSummaryRow is one row in the daily attendance summary report.
type AttendanceSummaryRow struct {
	GuardID    string `json:"guard_id"`
	GuardName  string `json:"guard_name"`
	SiteID     string `json:"site_id"`
	Date       string `json:"date"`
	Status     string `json:"status"`
	ClockInAt  any    `json:"clock_in_at"`
	ClockOutAt any    `json:"clock_out_at"`
}

// DailyAttendanceSummary returns attendance for all guards at a site on a date.
func DailyAttendanceSummary(ctx context.Context, pool *pgxpool.Pool, siteID, date string) ([]AttendanceSummaryRow, error) {
	rows, err := pool.Query(ctx,
		`SELECT ar.guard_id, g.name, ar.site_id,
		        to_char(ar.created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD') as date,
		        ar.status, ar.clock_in_at, ar.clock_out_at
		 FROM attendance_records ar
		 JOIN guards g ON g.id = ar.guard_id
		 WHERE ar.site_id = $1
		   AND DATE(ar.created_at AT TIME ZONE 'UTC') = $2
		 ORDER BY g.name ASC`, siteID, date)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []AttendanceSummaryRow
	for rows.Next() {
		var r AttendanceSummaryRow
		if err := rows.Scan(&r.GuardID, &r.GuardName, &r.SiteID, &r.Date, &r.Status, &r.ClockInAt, &r.ClockOutAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// PatrolCompletionRow is one row in the patrol completion report.
type PatrolCompletionRow struct {
	RouteID          string  `json:"route_id"`
	RouteName        string  `json:"route_name"`
	GuardID          string  `json:"guard_id"`
	GuardName        string  `json:"guard_name"`
	Date             string  `json:"date"`
	TotalCheckpoints int     `json:"total_checkpoints"`
	ScannedCount     int     `json:"scanned_count"`
	CompletionPct    float64 `json:"completion_pct"`
}

// PatrolCompletionSummary returns patrol completion per route per guard for a site on a date.
func PatrolCompletionSummary(ctx context.Context, pool *pgxpool.Pool, siteID, date string) ([]PatrolCompletionRow, error) {
	rows, err := pool.Query(ctx,
		`SELECT r.id, r.name, cs.guard_id, g.name,
		        to_char(cs.scanned_at AT TIME ZONE 'UTC', 'YYYY-MM-DD') as date,
		        jsonb_array_length(r.checkpoints) as total,
		        COUNT(DISTINCT cs.checkpoint_id) as scanned,
		        ROUND(COUNT(DISTINCT cs.checkpoint_id)::numeric /
		              NULLIF(jsonb_array_length(r.checkpoints), 0) * 100, 1) as pct
		 FROM checkpoint_scans cs
		 JOIN routes r ON r.id = cs.route_id
		 JOIN guards g ON g.id = cs.guard_id
		 WHERE r.site_id = $1
		   AND DATE(cs.scanned_at AT TIME ZONE 'UTC') = $2
		 GROUP BY r.id, r.name, cs.guard_id, g.name, date
		 ORDER BY date DESC, r.name ASC`, siteID, date)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PatrolCompletionRow
	for rows.Next() {
		var r PatrolCompletionRow
		if err := rows.Scan(&r.RouteID, &r.RouteName, &r.GuardID, &r.GuardName,
			&r.Date, &r.TotalCheckpoints, &r.ScannedCount, &r.CompletionPct); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// GuardKPIRow holds KPI metrics per guard.
type GuardKPIRow struct {
	GuardID           string  `json:"guard_id"`
	GuardName         string  `json:"guard_name"`
	Month             string  `json:"month"`
	AttendancePct     float64 `json:"attendance_pct"`
	PatrolCompletePct float64 `json:"patrol_complete_pct"`
	IncidentsReported int     `json:"incidents_reported"`
	LateCount         int     `json:"late_count"`
	SosCount          int     `json:"sos_count"`
}

// GuardKPIReport returns KPI aggregates per guard for a site in a month.
func GuardKPIReport(ctx context.Context, pool *pgxpool.Pool, siteID, month string) ([]GuardKPIRow, error) {
	rows, err := pool.Query(ctx,
		`WITH att AS (
		   SELECT guard_id,
		          ROUND(COUNT(*) FILTER (WHERE status IN ('present','late','early_out'))::numeric
		                / NULLIF(COUNT(*),0) * 100, 1) as att_pct,
		          COUNT(*) FILTER (WHERE status = 'late') as late_cnt
		   FROM attendance_records
		   WHERE site_id = $1 AND to_char(created_at AT TIME ZONE 'UTC', 'YYYY-MM') = $2
		   GROUP BY guard_id
		 ),
		 pat AS (
		   SELECT cs.guard_id,
		          ROUND(COUNT(DISTINCT cs.checkpoint_id)::numeric /
		                NULLIF(SUM(jsonb_array_length(r.checkpoints)),0) * 100, 1) as pat_pct
		   FROM checkpoint_scans cs
		   JOIN routes r ON r.id = cs.route_id
		   WHERE r.site_id = $1 AND to_char(cs.scanned_at AT TIME ZONE 'UTC', 'YYYY-MM') = $2
		   GROUP BY cs.guard_id
		 ),
		 inc AS (
		   SELECT guard_id, COUNT(*) as inc_cnt
		   FROM incidents
		   WHERE site_id = $1 AND to_char(created_at AT TIME ZONE 'UTC', 'YYYY-MM') = $2
		   GROUP BY guard_id
		 ),
		 sos AS (
		   SELECT guard_id, COUNT(*) as sos_cnt
		   FROM sos_alerts
		   WHERE site_id = $1 AND to_char(fired_at AT TIME ZONE 'UTC', 'YYYY-MM') = $2
		   GROUP BY guard_id
		 )
		 SELECT g.id, g.name, $2,
		        COALESCE(att.att_pct, 0), COALESCE(pat.pat_pct, 0),
		        COALESCE(inc.inc_cnt, 0), COALESCE(att.late_cnt, 0), COALESCE(sos.sos_cnt, 0)
		 FROM guards g
		 LEFT JOIN att ON att.guard_id = g.id
		 LEFT JOIN pat ON pat.guard_id = g.id
		 LEFT JOIN inc ON inc.guard_id = g.id
		 LEFT JOIN sos ON sos.guard_id = g.id
		 WHERE g.status = 'active'
		 ORDER BY g.name ASC`, siteID, month)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []GuardKPIRow
	for rows.Next() {
		var r GuardKPIRow
		if err := rows.Scan(&r.GuardID, &r.GuardName, &r.Month,
			&r.AttendancePct, &r.PatrolCompletePct,
			&r.IncidentsReported, &r.LateCount, &r.SosCount); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
