// Package jobs runs the periodic background work inside the worker process:
// shift generation, absent auto-flag, SLA checks, monthly recap and backups.
package jobs

import (
	"context"
	"fmt"
	"html"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"

	"github.com/securepatrol/backend/internal/config"
	"github.com/securepatrol/backend/internal/model"
	"github.com/securepatrol/backend/internal/notification"
	"github.com/securepatrol/backend/internal/repository"
	"github.com/securepatrol/backend/internal/service"
	"github.com/securepatrol/backend/internal/storage"
)

// Scheduler owns the periodic jobs.
type Scheduler struct {
	pool     *pgxpool.Pool
	rdb      *redis.Client
	notifier *notification.Notifier
	stg      *storage.Client
	cfg      *config.Config
	log      zerolog.Logger
}

// NewScheduler creates a scheduler.
func NewScheduler(pool *pgxpool.Pool, rdb *redis.Client, n *notification.Notifier, stg *storage.Client, cfg *config.Config, log zerolog.Logger) *Scheduler {
	return &Scheduler{pool: pool, rdb: rdb, notifier: n, stg: stg, cfg: cfg,
		log: log.With().Str("component", "scheduler").Logger()}
}

// Run starts every job loop and blocks until ctx is cancelled.
func (s *Scheduler) Run(ctx context.Context) {
	go s.loop(ctx, "generate_shifts", time.Hour, s.GenerateShifts)
	go s.loop(ctx, "abandon_sessions", 15*time.Minute, s.AbandonStaleSessions)
	go s.loop(ctx, "flag_absent", time.Minute, s.FlagAbsent)
	go s.loop(ctx, "sla_check", 30*time.Second, s.CheckSLA)
	go s.loop(ctx, "monthly_recap", 10*time.Minute, s.MonthlyRecap)
	go s.loop(ctx, "backups", time.Minute, s.Backups)
	<-ctx.Done()
}

// loop runs fn immediately and then every interval. A Redis lock ensures only
// one worker replica runs a given job at a time.
func (s *Scheduler) loop(ctx context.Context, name string, every time.Duration, fn func(context.Context) error) {
	run := func() {
		lockTTL := every - time.Second
		if lockTTL < 10*time.Second {
			lockTTL = every
		}
		ok, err := s.rdb.SetNX(ctx, "jobs:lock:"+name, "1", lockTTL).Result()
		if err != nil || !ok {
			return
		}
		jctx, cancel := context.WithTimeout(ctx, every*4)
		defer cancel()
		start := time.Now()
		if err := fn(jctx); err != nil {
			s.log.Error().Err(err).Str("job", name).Msg("job failed")
			return
		}
		s.log.Debug().Str("job", name).Dur("took", time.Since(start)).Msg("job done")
	}
	run()
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			run()
		}
	}
}

// GenerateShifts pre-generates concrete shifts from templates (D-003).
func (s *Scheduler) GenerateShifts(ctx context.Context) error {
	n, err := service.GenerateAllShifts(ctx, s.pool)
	if n > 0 {
		s.log.Info().Int("shifts", n).Msg("generated shifts")
	}
	return err
}

// AbandonStaleSessions closes patrol sessions left open too long.
func (s *Scheduler) AbandonStaleSessions(ctx context.Context) error {
	_, err := repository.AbandonStaleSessions(ctx, s.pool, service.StaleSessionAge)
	return err
}

// FlagAbsent marks no-shows absent and notifies (FR-ATT-04).
func (s *Scheduler) FlagAbsent(ctx context.Context) error {
	flags, err := repository.FlagAbsentees(ctx, s.pool, model.DefaultSiteConfig().AbsentAfterMinutes)
	if err != nil {
		return err
	}
	sites := siteCache{db: s.pool}
	for _, f := range flags {
		site, err := sites.get(ctx, f.SiteID)
		if err != nil {
			return err
		}
		_, err = s.notifier.Publish(ctx, notification.Event{
			Type: notification.EventAbsent, Site: site, DedupeKey: "absent:" + f.RecordID,
			Params: map[string]string{
				"guard": f.GuardName,
				"shift": f.StartTime.In(site.Location()).Format("02 Jan 15:04"),
			},
		})
		if err != nil {
			s.log.Error().Err(err).Str("record", f.RecordID).Msg("publish absent failed")
		}
	}
	return nil
}

// CheckSLA records checkpoint SLA breaches of running sessions and notifies
// once per breach (FR-PAT-04, target < 2 minutes).
func (s *Scheduler) CheckSLA(ctx context.Context) error {
	sessions, err := repository.ListActiveSessions(ctx, s.pool, "")
	if err != nil {
		return err
	}
	sites := siteCache{db: s.pool}
	for _, sess := range sessions {
		p, err := service.LoadProgress(ctx, s.pool, sess)
		if err != nil {
			return err
		}
		for _, cp := range p.Checkpoints {
			if cp.DueAt == nil || (cp.Status != "overdue" && cp.Status != "late") {
				continue
			}
			isNew, err := repository.InsertSLABreach(ctx, s.pool, sess.ID, cp.ID, *cp.DueAt)
			if err != nil {
				return err
			}
			if !isNew {
				continue
			}
			site, err := sites.get(ctx, sess.SiteID)
			if err != nil {
				return err
			}
			guard, err := repository.GetGuard(ctx, s.pool, sess.GuardID)
			if err != nil {
				return err
			}
			_, err = s.notifier.Publish(ctx, notification.Event{
				Type: notification.EventSLAMissed, Site: site,
				DedupeKey: "sla:" + sess.ID + ":" + cp.ID,
				Params: map[string]string{
					"guard": guard.Name, "route": p.RouteName, "checkpoint": cp.Name,
					"due": cp.DueAt.In(site.Location()).Format("15:04"),
				},
			})
			if err != nil {
				s.log.Error().Err(err).Str("session", sess.ID).Msg("publish SLA breach failed")
			}
		}
	}
	return nil
}

// MonthlyRecap e-mails last month's recap on the 1st of each month from 07:00
// site time (FR-NOT-05). The dedupe key makes it run once per site and month.
func (s *Scheduler) MonthlyRecap(ctx context.Context) error {
	sites, err := repository.ListAllSites(ctx, s.pool)
	if err != nil {
		return err
	}
	for i := range sites {
		site := &sites[i]
		loc := site.Location()
		now := service.Now().In(loc)
		if now.Day() != 1 || now.Hour() < 7 {
			continue
		}
		prev := now.AddDate(0, -1, 0)
		month := prev.Format("2006-01")
		from, to, err := service.MonthRange(month, loc)
		if err != nil {
			return err
		}
		rows, err := repository.MonthlyRecap(ctx, s.pool, site.ID, from, to)
		if err != nil {
			return err
		}
		subject, _ := notification.Render(notification.EventMonthlyRecap, site.Config().Language,
			map[string]string{"site": site.Name, "month": month})
		_, err = s.notifier.Publish(ctx, notification.Event{
			Type: notification.EventMonthlyRecap, Site: site, DedupeKey: "recap:" + site.ID + ":" + month,
			Subject: subject, Body: recapHTML(site, month, rows, s.cfg.AppPublicURL),
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func recapHTML(site *model.Site, month string, rows []repository.MonthlyRecapRow, appURL string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "<h2>%s — %s</h2>", html.EscapeString(site.Name), month)
	b.WriteString(`<table border="1" cellpadding="4" cellspacing="0" style="border-collapse:collapse;font-family:sans-serif;font-size:13px">`)
	b.WriteString("<tr><th>Guard</th><th>Employee ID</th><th>Shifts</th><th>Present</th><th>Late</th><th>Absent</th><th>Early-out</th><th>Hours</th><th>Patrol %</th></tr>")
	for _, r := range rows {
		emp := ""
		if r.EmployeeID != nil {
			emp = *r.EmployeeID
		}
		fmt.Fprintf(&b, "<tr><td>%s</td><td>%s</td><td>%d</td><td>%d</td><td>%d</td><td>%d</td><td>%d</td><td>%.1f</td><td>%.1f</td></tr>",
			html.EscapeString(r.GuardName), html.EscapeString(emp), r.ScheduledShifts, r.Present, r.Late,
			r.Absent, r.EarlyOut, r.TotalHours, r.PatrolCompletePct)
	}
	b.WriteString("</table>")
	if appURL != "" {
		fmt.Fprintf(&b, `<p><a href="%s/reports?site=%s&amp;month=%s">Open in SecurePatrol</a></p>`,
			html.EscapeString(appURL), site.ID, month)
	}
	return b.String()
}

// siteCache memoises site lookups within one job run.
type siteCache struct {
	db    repository.DBTX
	sites map[string]*model.Site
}

func (c *siteCache) get(ctx context.Context, id string) (*model.Site, error) {
	if c.sites == nil {
		c.sites = map[string]*model.Site{}
	}
	if s, ok := c.sites[id]; ok {
		return s, nil
	}
	s, err := repository.GetSite(ctx, c.db, id)
	if err != nil {
		return nil, err
	}
	c.sites[id] = s
	return s, nil
}
