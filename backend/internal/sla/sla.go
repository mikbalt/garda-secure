// Package sla evaluates checkpoint SLA breaches of patrol sessions and
// notifies them (PRD FR-PAT-04, D-008, audit F2).
//
// A breach is recorded once per (session, checkpoint) in sla_breaches and its
// notification is staged in the same transaction. Breaches are only raised
// when they are certain, so a tablet that was offline and syncs scans later
// (on time by their event time) does not cause false alarms:
//
//   - a scan after the due time (late) is always a breach;
//   - an unscanned checkpoint of an ended session (missed) is a breach;
//   - an unscanned checkpoint of a running session is a breach once the due
//     time plus Grace has passed AND either a later scan of the session has
//     already arrived (the tablet outbox is FIFO, so the earlier scan would
//     have too), or the tablet was online across the due time (its current
//     online period started at or before the due time and it was seen after
//     it). Sessions started offline are thus judged when they end or when a
//     later scan arrives.
package sla

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"github.com/securepatrol/backend/internal/model"
	"github.com/securepatrol/backend/internal/notification"
	"github.com/securepatrol/backend/internal/repository"
	"github.com/securepatrol/backend/internal/service"
)

// Grace is how long after the due time an unscanned checkpoint of a running
// session waits before it counts as missed (the scan may be in flight).
const Grace = time.Minute

// Breaches returns the checkpoints of p that breach their SLA at now. contact
// is the session tablet's connectivity, nil when unknown (then only time
// decides, the pre-F2 behaviour).
func Breaches(p service.SessionProgress, contact *repository.DeviceContact, now time.Time) []service.CheckpointProgress {
	ended := p.Session.Status != "in_progress"
	var latestScan time.Time
	for _, cp := range p.Checkpoints {
		if cp.ScannedAt != nil && cp.ScannedAt.After(latestScan) {
			latestScan = *cp.ScannedAt
		}
	}
	out := []service.CheckpointProgress{}
	for _, cp := range p.Checkpoints {
		if cp.DueAt == nil {
			continue
		}
		switch cp.Status {
		case "late":
			out = append(out, cp)
		case "missed":
			if ended {
				out = append(out, cp)
			}
		case "overdue":
			if !ended && confirmedMissing(*cp.DueAt, latestScan, contact, now) {
				out = append(out, cp)
			}
		}
	}
	return out
}

func confirmedMissing(due, latestScan time.Time, c *repository.DeviceContact, now time.Time) bool {
	if now.Before(due.Add(Grace)) {
		return false
	}
	if latestScan.After(due) {
		return true
	}
	if c == nil {
		return true
	}
	return c.OnlineSince != nil && !c.OnlineSince.After(due) &&
		c.LastSeen != nil && !c.LastSeen.Before(due)
}

// Checker evaluates sessions and stages breach notifications.
type Checker struct {
	pool     *pgxpool.Pool
	notifier *notification.Notifier
	log      zerolog.Logger
}

// NewChecker creates a checker.
func NewChecker(pool *pgxpool.Pool, n *notification.Notifier, log zerolog.Logger) *Checker {
	return &Checker{pool: pool, notifier: n, log: log.With().Str("component", "sla").Logger()}
}

// Evaluate records and notifies the new breaches of one session. It returns
// the number of new breaches. Each breach is its own transaction; an error
// on one does not stop the others (the first error is returned).
func (c *Checker) Evaluate(ctx context.Context, sess model.PatrolSession) (int, error) {
	p, err := service.LoadProgress(ctx, c.pool, sess)
	if err != nil {
		return 0, fmt.Errorf("session %s: %w", sess.ID, err)
	}
	var contact *repository.DeviceContact
	if sess.DeviceID != nil {
		contact, err = repository.GetDeviceContact(ctx, c.pool, *sess.DeviceID)
		if err != nil && !errors.Is(err, repository.ErrNotFound) {
			return 0, fmt.Errorf("session %s device: %w", sess.ID, err)
		}
	}
	breaches := Breaches(*p, contact, service.Now())
	if len(breaches) == 0 {
		return 0, nil
	}
	site, err := repository.GetSite(ctx, c.pool, sess.SiteID)
	if err != nil {
		return 0, fmt.Errorf("session %s site: %w", sess.ID, err)
	}
	guard, err := repository.GetGuard(ctx, c.pool, sess.GuardID)
	if err != nil {
		return 0, fmt.Errorf("session %s guard: %w", sess.ID, err)
	}

	added := 0
	var firstErr error
	for _, cp := range breaches {
		var staged notification.Staged
		isNew := false
		err := pgx.BeginFunc(ctx, c.pool, func(tx pgx.Tx) error {
			var err error
			isNew, err = repository.InsertSLABreach(ctx, tx, sess.ID, cp.ID, *cp.DueAt)
			if err != nil || !isNew {
				return err
			}
			staged, err = c.notifier.Stage(ctx, tx, notification.Event{
				Type: notification.EventSLAMissed, Site: site,
				DedupeKey: "sla:" + sess.ID + ":" + cp.ID,
				Params: map[string]string{
					"guard": guard.Name, "route": p.RouteName, "checkpoint": cp.Name,
					"due": cp.DueAt.In(site.Location()).Format("15:04"),
				},
			})
			return err
		})
		if err != nil {
			c.log.Error().Err(err).Str("session", sess.ID).Str("checkpoint", cp.ID).Msg("record SLA breach failed")
			if firstErr == nil {
				firstErr = fmt.Errorf("session %s checkpoint %s: %w", sess.ID, cp.ID, err)
			}
			continue
		}
		if isNew {
			added++
			c.log.Info().Str("session", sess.ID).Str("checkpoint", cp.ID).Str("status", cp.Status).Msg("SLA breach")
			c.notifier.Wake(ctx, staged)
		}
	}
	return added, firstErr
}
