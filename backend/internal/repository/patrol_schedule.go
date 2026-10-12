package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/securepatrol/backend/internal/model"
)

// ── Route schedules (D-035) ──────────────────────────────────────────────────

const routeScheduleColumns = `id, route_id, weekdays, to_char(start_time, 'HH24:MI'), to_char(end_time, 'HH24:MI'),
	every_minutes, grace_minutes, active, created_at, updated_at`

func scanRouteSchedule(row pgx.Row) (model.RouteSchedule, error) {
	var s model.RouteSchedule
	err := row.Scan(&s.ID, &s.RouteID, &s.Weekdays, &s.StartTime, &s.EndTime, &s.EveryMinutes,
		&s.GraceMinutes, &s.Active, &s.CreatedAt, &s.UpdatedAt)
	return s, err
}

func scanRouteScheduleRows(rows pgx.Rows) (model.RouteSchedule, error) {
	return scanRouteSchedule(rows)
}

// ListRouteSchedules returns the schedule windows of a route.
func ListRouteSchedules(ctx context.Context, db DBTX, routeID string) ([]model.RouteSchedule, error) {
	rows, err := db.Query(ctx, `
		SELECT `+routeScheduleColumns+` FROM route_schedules
		WHERE route_id = $1 ORDER BY start_time, created_at, id`, routeID)
	if err != nil {
		return nil, fmt.Errorf("ListRouteSchedules: %w", err)
	}
	return collect(rows, scanRouteScheduleRows)
}

// ReplaceRouteSchedules replaces every schedule window of a route; call it
// in a transaction.
func ReplaceRouteSchedules(ctx context.Context, db DBTX, routeID string, in []model.RouteSchedule) ([]model.RouteSchedule, error) {
	if _, err := db.Exec(ctx, `DELETE FROM route_schedules WHERE route_id = $1`, routeID); err != nil {
		return nil, fmt.Errorf("ReplaceRouteSchedules delete: %w", err)
	}
	out := []model.RouteSchedule{}
	for _, s := range in {
		row, err := scanRouteSchedule(db.QueryRow(ctx, `
			INSERT INTO route_schedules (route_id, weekdays, start_time, end_time, every_minutes, grace_minutes, active)
			VALUES ($1, $2, $3::time, $4::time, $5, $6, $7)
			RETURNING `+routeScheduleColumns,
			routeID, s.Weekdays, s.StartTime, s.EndTime, s.EveryMinutes, s.GraceMinutes, s.Active))
		if err != nil {
			return nil, fmt.Errorf("ReplaceRouteSchedules insert: %w", err)
		}
		out = append(out, row)
	}
	return out, nil
}

// ListScheduledRouteIDs returns the active routes that have an active
// schedule window.
func ListScheduledRouteIDs(ctx context.Context, db DBTX) ([]string, error) {
	rows, err := db.Query(ctx, `
		SELECT DISTINCT r.id::text FROM routes r
		JOIN route_schedules s ON s.route_id = r.id
		WHERE r.active AND s.active`)
	if err != nil {
		return nil, fmt.Errorf("ListScheduledRouteIDs: %w", err)
	}
	return collect(rows, func(r pgx.Rows) (string, error) {
		var id string
		err := r.Scan(&id)
		return id, err
	})
}

// ── Patrol expectations ──────────────────────────────────────────────────────

const expectationColumns = `e.id, e.route_id, COALESCE(r.name, ''), e.site_id, e.schedule_id, e.due_at, e.grace_until,
	e.status, e.session_id, e.late_start, e.notified_at, e.matched_at, e.created_at`

const expectationFrom = ` FROM patrol_expectations e LEFT JOIN routes r ON r.id = e.route_id `

func scanExpectation(row pgx.Row) (model.PatrolExpectation, error) {
	var e model.PatrolExpectation
	err := row.Scan(&e.ID, &e.RouteID, &e.RouteName, &e.SiteID, &e.ScheduleID, &e.DueAt, &e.GraceUntil,
		&e.Status, &e.SessionID, &e.LateStart, &e.NotifiedAt, &e.MatchedAt, &e.CreatedAt)
	return e, err
}

func scanExpectationRows(rows pgx.Rows) (model.PatrolExpectation, error) {
	return scanExpectation(rows)
}

// InsertExpectation stores a scheduled patrol once per (route, due time);
// returns false when it already existed.
func InsertExpectation(ctx context.Context, db DBTX, e model.PatrolExpectation) (bool, error) {
	tag, err := db.Exec(ctx, `
		INSERT INTO patrol_expectations (route_id, site_id, schedule_id, due_at, grace_until)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (route_id, due_at) DO NOTHING`, e.RouteID, e.SiteID, e.ScheduleID, e.DueAt, e.GraceUntil)
	if err != nil {
		return false, fmt.Errorf("InsertExpectation: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// DeleteFuturePendingExpectations removes a route's pending, unmatched
// expectations due after `after` (schedule changed: they are regenerated).
func DeleteFuturePendingExpectations(ctx context.Context, db DBTX, routeID string, after time.Time) (int64, error) {
	tag, err := db.Exec(ctx, `
		DELETE FROM patrol_expectations
		WHERE route_id = $1 AND status = 'pending' AND session_id IS NULL AND due_at > $2`, routeID, after)
	if err != nil {
		return 0, fmt.Errorf("DeleteFuturePendingExpectations: %w", err)
	}
	return tag.RowsAffected(), nil
}

// CancelPendingExpectations cancels every pending expectation of a route
// (route deactivated).
func CancelPendingExpectations(ctx context.Context, db DBTX, routeID string) (int64, error) {
	tag, err := db.Exec(ctx, `
		UPDATE patrol_expectations SET status = 'cancelled', updated_at = now()
		WHERE route_id = $1 AND status = 'pending'`, routeID)
	if err != nil {
		return 0, fmt.Errorf("CancelPendingExpectations: %w", err)
	}
	return tag.RowsAffected(), nil
}

// ListOverdueExpectationIDs returns pending expectations whose grace period
// ended before now, oldest first.
func ListOverdueExpectationIDs(ctx context.Context, db DBTX, now time.Time, limit int) ([]string, error) {
	rows, err := db.Query(ctx, `
		SELECT id::text FROM patrol_expectations
		WHERE status = 'pending' AND grace_until < $1
		ORDER BY grace_until LIMIT $2`, now, limit)
	if err != nil {
		return nil, fmt.Errorf("ListOverdueExpectationIDs: %w", err)
	}
	return collect(rows, func(r pgx.Rows) (string, error) {
		var id string
		err := r.Scan(&id)
		return id, err
	})
}

// LockExpectation loads an expectation and locks it for the transaction.
func LockExpectation(ctx context.Context, db DBTX, id string) (*model.PatrolExpectation, error) {
	e, err := scanExpectation(db.QueryRow(ctx, `SELECT `+expectationColumns+expectationFrom+
		`WHERE e.id = $1 FOR UPDATE OF e`, id))
	if err != nil {
		return nil, mapNoRows(err)
	}
	return &e, nil
}

// GetExpectation retrieves an expectation by ID.
func GetExpectation(ctx context.Context, db DBTX, id string) (*model.PatrolExpectation, error) {
	e, err := scanExpectation(db.QueryRow(ctx, `SELECT `+expectationColumns+expectationFrom+`WHERE e.id = $1`, id))
	if err != nil {
		return nil, mapNoRows(err)
	}
	return &e, nil
}

// unmatchedOpen is the condition for an expectation a session may satisfy.
const unmatchedOpen = `e.session_id IS NULL AND e.status IN ('pending', 'missed')`

// FindOnTimeExpectation returns the open expectation of a route whose window
// [due - grace, grace_until] contains startedAt, nearest due time first.
func FindOnTimeExpectation(ctx context.Context, db DBTX, routeID string, startedAt time.Time) (*model.PatrolExpectation, error) {
	e, err := scanExpectation(db.QueryRow(ctx, `SELECT `+expectationColumns+expectationFrom+`
		WHERE e.route_id = $1 AND `+unmatchedOpen+`
		  AND $2 >= e.due_at - (e.grace_until - e.due_at) AND $2 <= e.grace_until
		ORDER BY abs(extract(epoch FROM ($2 - e.due_at))), e.due_at
		LIMIT 1`, routeID, startedAt))
	if err != nil {
		return nil, mapNoRows(err)
	}
	return &e, nil
}

// FindLateExpectation returns the most recent open expectation of a route
// whose grace period ended before startedAt (a late patrol), as long as
// startedAt is within maxLate of its due time and before the window of the
// route's next expectation opens.
func FindLateExpectation(ctx context.Context, db DBTX, routeID string, startedAt time.Time, maxLate time.Duration) (*model.PatrolExpectation, error) {
	e, err := scanExpectation(db.QueryRow(ctx, `SELECT `+expectationColumns+expectationFrom+`
		WHERE e.route_id = $1 AND `+unmatchedOpen+`
		  AND e.grace_until < $2 AND $2 < e.due_at + $3::interval
		  AND NOT EXISTS (
		    SELECT 1 FROM patrol_expectations n
		    WHERE n.route_id = e.route_id AND n.due_at > e.due_at AND n.status <> 'cancelled'
		      AND n.due_at - (n.grace_until - n.due_at) <= $2)
		ORDER BY e.due_at DESC
		LIMIT 1`, routeID, startedAt, maxLate))
	if err != nil {
		return nil, mapNoRows(err)
	}
	return &e, nil
}

// SetExpectationStarted links a session to an open expectation. It returns
// false when the expectation was taken meanwhile or the session already
// satisfies another one.
func SetExpectationStarted(ctx context.Context, db DBTX, id, sessionID string, late bool) (bool, error) {
	tag, err := db.Exec(ctx, `
		UPDATE patrol_expectations e
		SET status = 'started', session_id = $2, late_start = $3, matched_at = now(), updated_at = now()
		WHERE e.id = $1 AND `+unmatchedOpen+`
		  AND NOT EXISTS (SELECT 1 FROM patrol_expectations o WHERE o.session_id = $2)`, id, sessionID, late)
	if IsUniqueViolation(err) {
		return false, nil // matched concurrently to another expectation
	}
	if err != nil {
		return false, fmt.Errorf("SetExpectationStarted: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// SetExpectationStatus sets a pending expectation to missed or cancelled;
// notified stamps notified_at.
func SetExpectationStatus(ctx context.Context, db DBTX, id, status string, notified bool) error {
	_, err := db.Exec(ctx, `
		UPDATE patrol_expectations
		SET status = $2, updated_at = now(), notified_at = CASE WHEN $3 THEN now() ELSE notified_at END
		WHERE id = $1 AND status = 'pending'`, id, status, notified)
	if err != nil {
		return fmt.Errorf("SetExpectationStatus: %w", err)
	}
	return nil
}

// FindSessionForExpectation returns the ID of a session of the route started
// in the expectation's window that satisfies no expectation yet (nearest to
// the due time).
func FindSessionForExpectation(ctx context.Context, db DBTX, e model.PatrolExpectation) (string, error) {
	var id string
	err := db.QueryRow(ctx, `
		SELECT ps.id::text FROM patrol_sessions ps
		WHERE ps.route_id = $1 AND ps.started_at >= $2 AND ps.started_at <= $3
		  AND NOT EXISTS (SELECT 1 FROM patrol_expectations x WHERE x.session_id = ps.id)
		ORDER BY abs(extract(epoch FROM (ps.started_at - $4))), ps.started_at
		LIMIT 1`, e.RouteID, e.DueAt.Add(-e.Grace()), e.GraceUntil, e.DueAt).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("FindSessionForExpectation: %w", err)
	}
	return id, nil
}

// ListUnmatchedSessions returns sessions of scheduled routes created since
// `since` that satisfy no expectation (for the matching sweep).
func ListUnmatchedSessions(ctx context.Context, db DBTX, since time.Time, limit int) ([]model.PatrolSession, error) {
	rows, err := db.Query(ctx, `
		SELECT `+sessionColumns+` FROM patrol_sessions ps
		WHERE ps.created_at >= $1
		  AND NOT EXISTS (SELECT 1 FROM patrol_expectations x WHERE x.session_id = ps.id)
		  AND EXISTS (SELECT 1 FROM patrol_expectations y WHERE y.route_id = ps.route_id AND y.status IN ('pending', 'missed'))
		ORDER BY ps.created_at LIMIT $2`, since, limit)
	if err != nil {
		return nil, fmt.Errorf("ListUnmatchedSessions: %w", err)
	}
	return collect(rows, scanSessionRows)
}

// ListPatrolsDue returns up to limit open expectations of a site that are
// due before `until` and whose due time is after `since` (upcoming and
// overdue patrols for the tablet), earliest first.
func ListPatrolsDue(ctx context.Context, db DBTX, siteID string, since, until time.Time, limit int) ([]model.PatrolExpectation, error) {
	rows, err := db.Query(ctx, `SELECT `+expectationColumns+expectationFrom+`
		WHERE e.site_id = $1 AND `+unmatchedOpen+` AND e.due_at >= $2 AND e.due_at <= $3
		ORDER BY e.due_at LIMIT $4`, siteID, since, until, limit)
	if err != nil {
		return nil, fmt.Errorf("ListPatrolsDue: %w", err)
	}
	return collect(rows, scanExpectationRows)
}

// ── Schedule compliance reports ──────────────────────────────────────────────

// ComplianceCounts are the outcomes of scheduled patrols.
type ComplianceCounts struct {
	Expected      int      `json:"expected"`        // scheduled patrols (not cancelled)
	StartedOnTime int      `json:"started_on_time"` // started within [due - grace, due + grace]
	Late          int      `json:"late"`            // started after the grace period
	Missed        int      `json:"missed"`          // not started
	Pending       int      `json:"pending"`         // not decided yet (grace not over)
	CompliancePct *float64 `json:"compliance_pct"`  // started_on_time / decided; null when nothing is decided
}

func (c *ComplianceCounts) finish() {
	if decided := c.StartedOnTime + c.Late + c.Missed; decided > 0 {
		v := round1(float64(c.StartedOnTime) / float64(decided) * 100)
		c.CompliancePct = &v
	}
}

// ComplianceRow is the schedule compliance of one route on one site-local day.
type ComplianceRow struct {
	Date      string `json:"date"` // YYYY-MM-DD in the site's time zone (by due time)
	RouteID   string `json:"route_id"`
	RouteName string `json:"route_name"`
	ComplianceCounts
}

const complianceAgg = `
	COUNT(*) FILTER (WHERE e.status <> 'cancelled'),
	COUNT(*) FILTER (WHERE e.status = 'started' AND NOT e.late_start),
	COUNT(*) FILTER (WHERE e.status = 'started' AND e.late_start),
	COUNT(*) FILTER (WHERE e.status = 'missed'),
	COUNT(*) FILTER (WHERE e.status = 'pending')`

// PatrolCompliance returns per-route, per-day schedule compliance for the
// expectations of a site due in [from, to); tz buckets the days.
func PatrolCompliance(ctx context.Context, db DBTX, siteID string, from, to time.Time, tz string) ([]ComplianceRow, error) {
	rows, err := db.Query(ctx, `
		SELECT to_char(e.due_at AT TIME ZONE $4, 'YYYY-MM-DD') AS day, e.route_id::text, COALESCE(r.name, ''),`+complianceAgg+`
		FROM patrol_expectations e LEFT JOIN routes r ON r.id = e.route_id
		WHERE e.site_id = $1 AND e.due_at >= $2 AND e.due_at < $3
		GROUP BY day, e.route_id, r.name
		ORDER BY day, r.name`, siteID, from, to, tz)
	if err != nil {
		return nil, fmt.Errorf("PatrolCompliance: %w", err)
	}
	return collect(rows, func(r pgx.Rows) (ComplianceRow, error) {
		var c ComplianceRow
		err := r.Scan(&c.Date, &c.RouteID, &c.RouteName, &c.Expected, &c.StartedOnTime, &c.Late, &c.Missed, &c.Pending)
		c.finish()
		return c, err
	})
}

// PatrolComplianceTotals sums the schedule compliance of a site for
// expectations due in [from, to).
func PatrolComplianceTotals(ctx context.Context, db DBTX, siteID string, from, to time.Time) (ComplianceCounts, error) {
	var c ComplianceCounts
	err := db.QueryRow(ctx, `SELECT `+complianceAgg+`
		FROM patrol_expectations e
		WHERE e.site_id = $1 AND e.due_at >= $2 AND e.due_at < $3`, siteID, from, to).
		Scan(&c.Expected, &c.StartedOnTime, &c.Late, &c.Missed, &c.Pending)
	if err != nil {
		return c, fmt.Errorf("PatrolComplianceTotals: %w", err)
	}
	c.finish()
	return c, nil
}

// GuardScheduleStat is how a guard's sessions satisfied scheduled patrols.
type GuardScheduleStat struct {
	Started int // expectations satisfied by the guard's sessions
	OnTime  int // of those, started within the grace period
}

// GuardScheduleStats returns, per guard, the scheduled patrols of a site due
// in [from, to) that the guard's sessions satisfied. Missed patrols are not
// attributable to a guard.
func GuardScheduleStats(ctx context.Context, db DBTX, siteID string, from, to time.Time) (map[string]GuardScheduleStat, error) {
	rows, err := db.Query(ctx, `
		SELECT ps.guard_id::text, COUNT(*), COUNT(*) FILTER (WHERE NOT e.late_start)
		FROM patrol_expectations e JOIN patrol_sessions ps ON ps.id = e.session_id
		WHERE e.site_id = $1 AND e.status = 'started' AND e.due_at >= $2 AND e.due_at < $3
		GROUP BY ps.guard_id`, siteID, from, to)
	if err != nil {
		return nil, fmt.Errorf("GuardScheduleStats: %w", err)
	}
	out := map[string]GuardScheduleStat{}
	_, err = collect(rows, func(r pgx.Rows) (struct{}, error) {
		var id string
		var s GuardScheduleStat
		err := r.Scan(&id, &s.Started, &s.OnTime)
		out[id] = s
		return struct{}{}, err
	})
	return out, err
}
