package service

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"time"

	"github.com/securepatrol/backend/internal/model"
	"github.com/securepatrol/backend/internal/repository"
)

// Patrol schedules and "patrol not started" expectations (D-035).
const (
	// ExpectationHorizon is how far ahead scheduled patrols are materialised.
	ExpectationHorizon = 48 * time.Hour
	// LateStartMax bounds how long after its due time a late session still
	// satisfies a missed patrol (and never past the next window's start).
	LateStartMax = 12 * time.Hour
	// MaxSchedulesPerRoute caps the windows of one route.
	MaxSchedulesPerRoute = 20
	// DefaultGraceMinutes applies when a window gives none.
	DefaultGraceMinutes = 15
	minEveryMinutes     = 15
	maxEveryMinutes     = 720
	maxGraceMinutes     = 180
)

// ScheduleInput is one window as sent by the client (grace and active
// optional).
type ScheduleInput struct {
	Weekdays     []int16 `json:"weekdays"`
	StartTime    string  `json:"start_time"`
	EndTime      *string `json:"end_time"`
	EveryMinutes *int    `json:"every_minutes"`
	GraceMinutes *int    `json:"grace_minutes"`
	Active       *bool   `json:"active"`
}

// ValidateSchedules checks and normalises a route's schedule windows.
func ValidateSchedules(in []ScheduleInput) ([]model.RouteSchedule, error) {
	if len(in) > MaxSchedulesPerRoute {
		return nil, BadRequest("too_many_schedules", "a route can have at most %d schedule windows", MaxSchedulesPerRoute)
	}
	out := make([]model.RouteSchedule, 0, len(in))
	for i, s := range in {
		n := i + 1
		if _, err := parseClock(s.StartTime); err != nil {
			return nil, BadRequest("invalid_start_time", "schedule %d: start_time must be HH:MM", n).WithField("start_time")
		}
		days := slices.Clone(s.Weekdays)
		slices.Sort(days)
		days = slices.Compact(days)
		if len(days) == 0 || days[0] < 1 || days[len(days)-1] > 7 {
			return nil, BadRequest("invalid_weekdays", "schedule %d: weekdays must be 1 (Monday) … 7 (Sunday)", n).WithField("weekdays")
		}
		rs := model.RouteSchedule{Weekdays: days, StartTime: s.StartTime, GraceMinutes: DefaultGraceMinutes, Active: s.Active == nil || *s.Active}
		if s.GraceMinutes != nil {
			rs.GraceMinutes = *s.GraceMinutes
		}
		if rs.GraceMinutes < 0 || rs.GraceMinutes > maxGraceMinutes {
			return nil, BadRequest("invalid_grace", "schedule %d: grace_minutes must be 0–%d", n, maxGraceMinutes).WithField("grace_minutes")
		}
		if s.EndTime != nil && *s.EndTime != "" {
			if _, err := parseClock(*s.EndTime); err != nil {
				return nil, BadRequest("invalid_end_time", "schedule %d: end_time must be HH:MM", n).WithField("end_time")
			}
			end := *s.EndTime
			rs.EndTime = &end
		}
		if s.EveryMinutes != nil {
			every := *s.EveryMinutes
			if every < minEveryMinutes || every > maxEveryMinutes {
				return nil, BadRequest("invalid_every", "schedule %d: every_minutes must be %d–%d", n, minEveryMinutes, maxEveryMinutes).WithField("every_minutes")
			}
			if rs.EndTime == nil {
				return nil, BadRequest("end_time_required", "schedule %d: end_time is required with every_minutes", n).WithField("end_time")
			}
			if rs.GraceMinutes*2 > every {
				return nil, BadRequest("grace_too_long", "schedule %d: grace_minutes must be at most half of every_minutes", n).WithField("grace_minutes")
			}
			rs.EveryMinutes = &every
		}
		out = append(out, rs)
	}
	return out, nil
}

// Due is one materialised patrol time.
type Due struct {
	At, GraceUntil time.Time
}

// ScheduleOccurrences returns the due times of a window in (from, to],
// computed in loc.
func ScheduleOccurrences(s model.RouteSchedule, loc *time.Location, from, to time.Time) ([]Due, error) {
	start, err := parseClock(s.StartTime)
	if err != nil {
		return nil, err
	}
	var end clock
	if s.EveryMinutes != nil {
		if s.EndTime == nil {
			return nil, errors.New("schedule without end_time")
		}
		if end, err = parseClock(*s.EndTime); err != nil {
			return nil, err
		}
	}
	grace := time.Duration(s.GraceMinutes) * time.Minute
	var out []Due
	add := func(t time.Time) {
		if t.After(from) && !t.After(to) {
			out = append(out, Due{At: t.UTC(), GraceUntil: t.Add(grace).UTC()})
		}
	}
	// A window starting the day before `from` may reach into the range.
	f := from.In(loc)
	day := time.Date(f.Year(), f.Month(), f.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, -1)
	t := to.In(loc)
	last := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
	for ; !day.After(last); day = day.AddDate(0, 0, 1) {
		if !slices.Contains(s.Weekdays, isoWeekday(day)) {
			continue
		}
		first := time.Date(day.Year(), day.Month(), day.Day(), start.h, start.m, 0, 0, loc)
		if s.EveryMinutes == nil {
			add(first)
			continue
		}
		endDay := day
		if end.minutes() <= start.minutes() {
			endDay = day.AddDate(0, 0, 1) // crosses midnight (equal = 24 h)
		}
		stop := time.Date(endDay.Year(), endDay.Month(), endDay.Day(), end.h, end.m, 0, 0, loc)
		step := time.Duration(*s.EveryMinutes) * time.Minute
		for at := first; at.Before(stop); at = at.Add(step) {
			add(at)
		}
	}
	return out, nil
}

// MaterialiseRoute stores the route's scheduled patrols due in (now, now +
// ExpectationHorizon]; existing ones are kept (idempotent). Inactive routes
// and windows produce nothing.
func MaterialiseRoute(ctx context.Context, db repository.DBTX, route *model.Route, site *model.Site, now time.Time) (int, error) {
	if !route.Active {
		return 0, nil
	}
	schedules, err := repository.ListRouteSchedules(ctx, db, route.ID)
	if err != nil {
		return 0, err
	}
	loc := site.Location()
	type item struct {
		due        Due
		scheduleID string
	}
	byDue := map[int64]item{}
	for _, s := range schedules {
		if !s.Active {
			continue
		}
		occ, err := ScheduleOccurrences(s, loc, now, now.Add(ExpectationHorizon))
		if err != nil {
			return 0, fmt.Errorf("schedule %s: %w", s.ID, err)
		}
		for _, d := range occ {
			// Two windows due at the same time: one patrol, longest grace.
			if prev, ok := byDue[d.At.Unix()]; !ok || d.GraceUntil.After(prev.due.GraceUntil) {
				byDue[d.At.Unix()] = item{d, s.ID}
			}
		}
	}
	keys := make([]int64, 0, len(byDue))
	for k := range byDue {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	added := 0
	for _, k := range keys {
		it := byDue[k]
		sid := it.scheduleID
		ok, err := repository.InsertExpectation(ctx, db, model.PatrolExpectation{
			RouteID: route.ID, SiteID: route.SiteID, ScheduleID: &sid, DueAt: it.due.At, GraceUntil: it.due.GraceUntil,
		})
		if err != nil {
			return added, err
		}
		if ok {
			added++
		}
	}
	return added, nil
}

// SyncRouteExpectations rebuilds a route's future scheduled patrols after its
// schedule or active flag changed: pending, unmatched patrols due after now
// are replaced; an inactive route's pending patrols are cancelled.
func SyncRouteExpectations(ctx context.Context, db repository.DBTX, route *model.Route, site *model.Site) (int, error) {
	now := Now()
	if !route.Active {
		_, err := repository.CancelPendingExpectations(ctx, db, route.ID)
		return 0, err
	}
	if _, err := repository.DeleteFuturePendingExpectations(ctx, db, route.ID, now); err != nil {
		return 0, err
	}
	return MaterialiseRoute(ctx, db, route, site, now)
}

// MaterialiseAll runs MaterialiseRoute for every scheduled route; a failing
// route does not stop the others (errors are joined).
func MaterialiseAll(ctx context.Context, db repository.DBTX) (int, error) {
	ids, err := repository.ListScheduledRouteIDs(ctx, db)
	if err != nil {
		return 0, err
	}
	now := Now()
	sites := map[string]*model.Site{}
	total := 0
	var errs []error
	for _, id := range ids {
		n, err := func() (int, error) {
			route, err := repository.GetRoute(ctx, db, id)
			if err != nil {
				return 0, err
			}
			site, ok := sites[route.SiteID]
			if !ok {
				if site, err = repository.GetSite(ctx, db, route.SiteID); err != nil {
					return 0, err
				}
				sites[route.SiteID] = site
			}
			return MaterialiseRoute(ctx, db, route, site, now)
		}()
		total += n
		if err != nil {
			errs = append(errs, fmt.Errorf("route %s: %w", id, err))
		}
	}
	if len(errs) > 0 {
		return total, fmt.Errorf("%d/%d routes failed: %w", len(errs), len(ids), errors.Join(errs...))
	}
	return total, nil
}

// MatchSession links a session to the scheduled patrol it satisfies, by its
// event time started_at: the open patrol whose window [due - grace, due +
// grace] contains it (nearest due time), else a missed/overdue patrol it
// started late for (late_start). Sessions synced from an offline tablet
// match retroactively. Returns nil when nothing matches.
func MatchSession(ctx context.Context, db repository.DBTX, s *model.PatrolSession) (*model.PatrolExpectation, error) {
	for attempt := 0; attempt < 3; attempt++ {
		late := false
		e, err := repository.FindOnTimeExpectation(ctx, db, s.RouteID, s.StartedAt)
		if errors.Is(err, repository.ErrNotFound) {
			late = true
			e, err = repository.FindLateExpectation(ctx, db, s.RouteID, s.StartedAt, LateStartMax)
		}
		if errors.Is(err, repository.ErrNotFound) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		ok, err := repository.SetExpectationStarted(ctx, db, e.ID, s.ID, late)
		if err != nil {
			return nil, err
		}
		if ok {
			e.Status, e.SessionID, e.LateStart = "started", &s.ID, late
			return e, nil
		}
		// Lost a race (taken by another session, or this session matched
		// concurrently): look again.
	}
	return nil, nil
}
