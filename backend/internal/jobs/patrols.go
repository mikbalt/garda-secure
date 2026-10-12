package jobs

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/securepatrol/backend/internal/model"
	"github.com/securepatrol/backend/internal/notification"
	"github.com/securepatrol/backend/internal/repository"
	"github.com/securepatrol/backend/internal/service"
)

// Scheduled patrols (D-035).
const (
	// patrolNotifyWithin: patrols found missed later than this after their
	// grace period (worker outage) are recorded without a notification.
	patrolNotifyWithin = 24 * time.Hour
	// patrolBatch bounds the work of one run; the rest waits a minute.
	patrolBatch = 500
	// matchSweepWindow: sessions created this recently are re-matched to
	// scheduled patrols when the API could not (or the patrol appeared later).
	matchSweepWindow = 24 * time.Hour
)

// MaterialiseExpectations stores the scheduled patrols of the next 48 h
// (idempotent; hourly).
func (s *Scheduler) MaterialiseExpectations(ctx context.Context) error {
	n, err := service.MaterialiseAll(ctx, s.pool)
	if n > 0 {
		s.log.Info().Int("patrols", n).Msg("scheduled patrols materialised")
	}
	return err
}

// CheckPatrolExpectations matches unmatched sessions to scheduled patrols and
// marks patrols missed once their grace period is over, staging the
// patrol_not_started notification in the same transaction. A patrol whose
// tablet was offline is still missed (D-035); a session synced later
// flips it to started (late_start when it began after the grace period).
func (s *Scheduler) CheckPatrolExpectations(ctx context.Context) error {
	now := service.Now()
	sessions, err := repository.ListUnmatchedSessions(ctx, s.pool, now.Add(-matchSweepWindow), patrolBatch)
	if err != nil {
		return err
	}
	matchErr := eachIsolated(s.log, "match session to scheduled patrol", len(sessions), func(i int) error {
		_, err := service.MatchSession(ctx, s.pool, &sessions[i])
		return err
	})
	ids, err := repository.ListOverdueExpectationIDs(ctx, s.pool, now, patrolBatch)
	if err != nil {
		return errors.Join(matchErr, err)
	}
	decideErr := eachIsolated(s.log, "decide scheduled patrol", len(ids), func(i int) error {
		return s.decideExpectation(ctx, ids[i], now)
	})
	return errors.Join(matchErr, decideErr)
}

func (s *Scheduler) decideExpectation(ctx context.Context, id string, now time.Time) error {
	var staged notification.Staged
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		staged = notification.Staged{}
		e, err := repository.LockExpectation(ctx, tx, id)
		if errors.Is(err, repository.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if e.Status != "pending" {
			return nil // decided concurrently (a session arrived)
		}
		route, err := repository.GetRoute(ctx, tx, e.RouteID)
		if err != nil {
			return err
		}
		if !route.Active {
			return repository.SetExpectationStatus(ctx, tx, e.ID, "cancelled", false)
		}
		// A session in the window that was not matched yet (e.g. started
		// before the patrol was materialised).
		sid, err := repository.FindSessionForExpectation(ctx, tx, *e)
		switch {
		case err == nil:
			ok, err := repository.SetExpectationStarted(ctx, tx, e.ID, sid, false)
			if err != nil || ok {
				return err
			}
		case !errors.Is(err, repository.ErrNotFound):
			return err
		}
		notify := now.Sub(e.GraceUntil) <= patrolNotifyWithin
		if err := repository.SetExpectationStatus(ctx, tx, e.ID, "missed", notify); err != nil {
			return err
		}
		if !notify {
			s.log.Info().Str("expectation", e.ID).Time("due_at", e.DueAt).Msg("old scheduled patrol marked missed without notification")
			return nil
		}
		site, err := repository.GetSite(ctx, tx, e.SiteID)
		if err != nil {
			return err
		}
		staged, err = s.notifier.Stage(ctx, tx, patrolNotStartedEvent(site, route, e))
		return err
	})
	if err != nil {
		return fmt.Errorf("expectation %s: %w", id, err)
	}
	s.notifier.Wake(ctx, staged)
	return nil
}

func patrolNotStartedEvent(site *model.Site, route *model.Route, e *model.PatrolExpectation) notification.Event {
	loc := site.Location()
	return notification.Event{
		Type: notification.EventPatrolNotStarted, Site: site, DedupeKey: "patrol_not_started:" + e.ID,
		Params: map[string]string{
			"route":       route.Name,
			"due":         e.DueAt.In(loc).Format("15:04"),
			"date":        e.DueAt.In(loc).Format("02 Jan"),
			"grace_until": e.GraceUntil.In(loc).Format("15:04"),
			"grace":       strconv.Itoa(int(e.Grace() / time.Minute)),
		},
	}
}

// recapCompliance renders the site's patrol schedule compliance for the
// monthly recap ("" when the site has no scheduled patrols).
func recapCompliance(c repository.ComplianceCounts, lang string, html bool) string {
	if c.Expected == 0 {
		return ""
	}
	pct := "-"
	if c.CompliancePct != nil {
		pct = strconv.FormatFloat(*c.CompliancePct, 'f', 1, 64) + "%"
	}
	var line string
	if lang == "en" {
		line = fmt.Sprintf("Patrol schedule compliance: %s (scheduled %d, on time %d, late %d, missed %d)",
			pct, c.Expected, c.StartedOnTime, c.Late, c.Missed)
	} else {
		line = fmt.Sprintf("Kepatuhan jadwal patroli: %s (terjadwal %d, tepat waktu %d, terlambat %d, terlewat %d)",
			pct, c.Expected, c.StartedOnTime, c.Late, c.Missed)
	}
	if html {
		return "<p>" + line + "</p>"
	}
	return line
}
