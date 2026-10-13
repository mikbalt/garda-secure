package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// Monthly recap (FR-REP-05, audit F13): attendance is counted per guard-day.
//
// A guard-day is a calendar day in the site's time zone, keyed by the
// scheduled start of the shift (an overnight shift counts on the day it
// starts, D-013). Several shifts of one guard on one day form one guard-day,
// whose status follows from the (post-override) outcome of its shifts:
//
//	absent    if any shift is absent
//	late      else if any shift is late
//	present   else if any shift is present (every decided shift is present)
//	upcoming  else (every shift still pending: future, ongoing or not yet
//	          auto-flagged absent)
//
// Early departures are counted separately (early_out_days = days with at
// least one shift left early), independent of the day status. Hours are the
// sum of clock-in to clock-out over the shifts. Shift-level counts are kept
// next to the day counts for transparency.

// RecapCounts holds the attendance and patrol figures of a recap row or of
// the site totals.
type RecapCounts struct {
	// Guard-days (payroll reference).
	ScheduledDays int `json:"scheduled_days"`
	DaysPresent   int `json:"days_present"`
	DaysLate      int `json:"days_late"`
	DaysAbsent    int `json:"days_absent"`
	DaysUpcoming  int `json:"days_upcoming"`
	EarlyOutDays  int `json:"early_out_days"`
	// Shifts (transparency; also the basis of the guard KPIs).
	ScheduledShifts int `json:"scheduled_shifts"`
	Present         int `json:"present"`
	Late            int `json:"late"`
	Absent          int `json:"absent"`
	EarlyOut        int `json:"early_out"`
	Pending         int `json:"pending"`

	TotalHours        float64 `json:"total_hours"`
	PatrolSessions    int     `json:"patrol_sessions"`
	PatrolCompletePct float64 `json:"patrol_completion_pct"`
}

// MonthlyRecapRow is the per-guard payroll reference for a period.
type MonthlyRecapRow struct {
	GuardID    string  `json:"guard_id"`
	GuardName  string  `json:"guard_name"`
	EmployeeID *string `json:"employee_id,omitempty"`
	RecapCounts
}

// MonthlyRecapTotals sums the rows of a site's recap. Patrol completion is
// the session-weighted average of the rows.
type MonthlyRecapTotals struct {
	Guards int `json:"guards"`
	RecapCounts
}

// RecapShift is one shift of the recap period with its outcome.
type RecapShift struct {
	GuardID  string
	Start    time.Time
	Status   string // present | late | absent | pending (no record yet)
	EarlyOut bool
	Hours    float64
}

// dayRank orders shift outcomes for the day status: the worst one wins.
func dayRank(status string) int {
	switch status {
	case "absent":
		return 3
	case "late":
		return 2
	case "present":
		return 1
	}
	return 0 // pending
}

// AggregateRecap counts shifts and guard-days per guard. Days are calendar
// days of the shift start in loc (the site's zone).
func AggregateRecap(shifts []RecapShift, loc *time.Location) map[string]*RecapCounts {
	type dayKey struct{ guard, day string }
	type day struct {
		rank     int
		earlyOut bool
	}
	days := map[dayKey]*day{}
	out := map[string]*RecapCounts{}
	for _, s := range shifts {
		c := out[s.GuardID]
		if c == nil {
			c = &RecapCounts{}
			out[s.GuardID] = c
		}
		c.ScheduledShifts++
		switch s.Status {
		case "present":
			c.Present++
		case "late":
			c.Late++
		case "absent":
			c.Absent++
		default:
			c.Pending++
		}
		if s.EarlyOut {
			c.EarlyOut++
		}
		c.TotalHours += s.Hours

		k := dayKey{s.GuardID, s.Start.In(loc).Format("2006-01-02")}
		d := days[k]
		if d == nil {
			d = &day{}
			days[k] = d
		}
		d.rank = max(d.rank, dayRank(s.Status))
		d.earlyOut = d.earlyOut || s.EarlyOut
	}
	for k, d := range days {
		c := out[k.guard]
		c.ScheduledDays++
		switch d.rank {
		case 3:
			c.DaysAbsent++
		case 2:
			c.DaysLate++
		case 1:
			c.DaysPresent++
		default:
			c.DaysUpcoming++
		}
		if d.earlyOut {
			c.EarlyOutDays++
		}
	}
	for _, c := range out {
		c.TotalHours = round1(c.TotalHours)
	}
	return out
}

// RecapTotals sums the rows (as displayed, so the totals match the column
// sums of an export).
func RecapTotals(rows []MonthlyRecapRow) MonthlyRecapTotals {
	t := MonthlyRecapTotals{Guards: len(rows)}
	var pctSum float64
	for _, r := range rows {
		c := &t.RecapCounts
		c.ScheduledDays += r.ScheduledDays
		c.DaysPresent += r.DaysPresent
		c.DaysLate += r.DaysLate
		c.DaysAbsent += r.DaysAbsent
		c.DaysUpcoming += r.DaysUpcoming
		c.EarlyOutDays += r.EarlyOutDays
		c.ScheduledShifts += r.ScheduledShifts
		c.Present += r.Present
		c.Late += r.Late
		c.Absent += r.Absent
		c.EarlyOut += r.EarlyOut
		c.Pending += r.Pending
		c.TotalHours += r.TotalHours
		c.PatrolSessions += r.PatrolSessions
		pctSum += r.PatrolCompletePct * float64(r.PatrolSessions)
	}
	t.TotalHours = round1(t.TotalHours)
	if t.PatrolSessions > 0 {
		t.PatrolCompletePct = round1(pctSum / float64(t.PatrolSessions))
	}
	return t
}

// RecapShifts returns the shifts of a site starting in [from, to) with their
// attendance outcome.
func RecapShifts(ctx context.Context, db DBTX, siteID string, from, to time.Time) ([]RecapShift, error) {
	rows, err := db.Query(ctx, `
		SELECT s.guard_id, s.start_time, COALESCE(a.status, 'pending'),
		       COALESCE(a.early_out_minutes, 0) > 0,
		       COALESCE(EXTRACT(EPOCH FROM (a.clock_out_at - a.clock_in_at)) / 3600, 0)::float8
		FROM shifts s
		LEFT JOIN attendance_records a ON a.shift_id = s.id
		WHERE s.site_id = $1 AND s.start_time >= $2 AND s.start_time < $3`, siteID, from, to)
	if err != nil {
		return nil, fmt.Errorf("RecapShifts: %w", err)
	}
	return collect(rows, func(r pgx.Rows) (RecapShift, error) {
		var s RecapShift
		err := r.Scan(&s.GuardID, &s.Start, &s.Status, &s.EarlyOut, &s.Hours)
		return s, err
	})
}

// MonthlyRecap aggregates attendance (per guard-day and per shift) and patrol
// completion per guard for the shifts of a site starting in [from, to); loc
// is the site's time zone that defines the days. A guard who moved between
// sites is counted at each site for the shifts scheduled there.
func MonthlyRecap(ctx context.Context, db DBTX, siteID string, loc *time.Location, from, to time.Time) ([]MonthlyRecapRow, error) {
	shifts, err := RecapShifts(ctx, db, siteID, from, to)
	if err != nil {
		return nil, err
	}
	counts := AggregateRecap(shifts, loc)
	rows, err := db.Query(ctx, `
		WITH pat AS (
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
		SELECT g.id, g.name, g.employee_id, COALESCE(pat.sessions, 0), COALESCE(pat.pct, 0)::float8
		FROM guards g
		LEFT JOIN pat ON pat.guard_id = g.id
		WHERE pat.guard_id IS NOT NULL
		   OR g.id IN (SELECT s.guard_id FROM shifts s WHERE s.site_id = $1 AND s.start_time >= $2 AND s.start_time < $3)
		ORDER BY g.name, g.id`, siteID, from, to)
	if err != nil {
		return nil, fmt.Errorf("MonthlyRecap: %w", err)
	}
	return collect(rows, func(r pgx.Rows) (MonthlyRecapRow, error) {
		var m MonthlyRecapRow
		var sessions int
		var pct float64
		if err := r.Scan(&m.GuardID, &m.GuardName, &m.EmployeeID, &sessions, &pct); err != nil {
			return m, err
		}
		if c := counts[m.GuardID]; c != nil {
			m.RecapCounts = *c
		}
		m.PatrolSessions, m.PatrolCompletePct = sessions, round1(pct)
		return m, nil
	})
}
