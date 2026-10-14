// Package jobs runs the periodic background work inside the worker process:
// shift generation, absent auto-flag, SLA checks, scheduled patrols, monthly
// recap and backups.
package jobs

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"

	"github.com/securepatrol/backend/internal/config"
	"github.com/securepatrol/backend/internal/metrics"
	"github.com/securepatrol/backend/internal/model"
	"github.com/securepatrol/backend/internal/notification"
	"github.com/securepatrol/backend/internal/repository"
	"github.com/securepatrol/backend/internal/service"
	"github.com/securepatrol/backend/internal/sla"
	"github.com/securepatrol/backend/internal/storage"
)

const (
	// absentLookback is how far back FlagAbsent looks for unflagged shifts,
	// so a worker outage of up to 3 days loses no absent flag (audit F14).
	absentLookback = 3 * 24 * time.Hour
	// absentNotifyWithin limits absent notifications to recent shifts; older
	// ones found after an outage are flagged silently (they are in reports).
	absentNotifyWithin = 24 * time.Hour
	// recapCatchUpDays: the monthly recap is sent from day 1, 07:00 site time,
	// until the end of this day of the month if the worker was down (F14).
	recapCatchUpDays = 5
	// jobFailingAfter consecutive failures raise an ops alert (hourly at most).
	jobFailingAfter = 5
	// lockKeyClass namespaces the Postgres advisory locks of this package.
	lockKeyClass = 7411
)

// Scheduler owns the periodic jobs.
type Scheduler struct {
	pool     *pgxpool.Pool
	rdb      *redis.Client
	notifier *notification.Notifier
	sla      *sla.Checker
	stg      *storage.Client
	cfg      *config.Config
	log      zerolog.Logger

	heartbeat metrics.Heartbeat
	mu        sync.Mutex
	failures  map[string]int // consecutive failures per job
	redisDown bool
}

// NewScheduler creates a scheduler.
func NewScheduler(pool *pgxpool.Pool, rdb *redis.Client, n *notification.Notifier, stg *storage.Client, cfg *config.Config, log zerolog.Logger) *Scheduler {
	l := log.With().Str("component", "scheduler").Logger()
	return &Scheduler{pool: pool, rdb: rdb, notifier: n, sla: sla.NewChecker(pool, n, log), stg: stg, cfg: cfg,
		log: l, failures: map[string]int{}}
}

// Heartbeat reports when a job loop last ticked.
func (s *Scheduler) Heartbeat() time.Time { return s.heartbeat.Last() }

// job is one periodic job.
type job struct {
	name    string
	every   time.Duration
	timeout time.Duration
	fn      func(context.Context) error
}

func (s *Scheduler) jobs() []job {
	backupTimeout := s.backupTimeout()*2 + 5*time.Minute // app + Keycloak dump
	return []job{
		{"generate_shifts", time.Hour, 4 * time.Hour, s.GenerateShifts},
		{"abandon_sessions", 15 * time.Minute, time.Hour, s.AbandonStaleSessions},
		{"flag_absent", time.Minute, 4 * time.Minute, s.FlagAbsent},
		{"sla_check", 30 * time.Second, 2 * time.Minute, s.CheckSLA},
		{"patrol_expectations", time.Hour, 30 * time.Minute, s.MaterialiseExpectations},
		{"patrol_not_started", time.Minute, 4 * time.Minute, s.CheckPatrolExpectations},
		{"monthly_recap", 10 * time.Minute, 40 * time.Minute, s.MonthlyRecap},
		{"backups", time.Minute, backupTimeout, s.Backups},
		{"retention", time.Hour, 50 * time.Minute, s.Retention},
		{"sos_escalation", 30 * time.Second, 2 * time.Minute, s.EscalateSOS},
		{"device_health", 5 * time.Minute, 4 * time.Minute, s.CheckDeviceHealth},
	}
}

// Run starts every job loop and blocks until ctx is cancelled and every
// in-flight run has finished. In-flight runs are not cancelled by ctx (a
// graceful shutdown lets them complete); they are cancelled when abort is.
func (s *Scheduler) Run(ctx, abort context.Context) {
	s.heartbeat.Beat()
	if err := s.failInterruptedBackups(ctx); err != nil {
		s.log.Warn().Err(err).Msg("could not check for interrupted backups")
	}
	var wg sync.WaitGroup
	for _, j := range s.jobs() {
		wg.Add(1)
		go func(j job) {
			defer wg.Done()
			s.loop(ctx, abort, j)
		}(j)
	}
	wg.Wait()
	s.log.Info().Msg("scheduler stopped")
}

// loop runs a job immediately and then every interval.
func (s *Scheduler) loop(ctx, abort context.Context, j job) {
	t := time.NewTicker(j.every)
	defer t.Stop()
	for {
		s.heartbeat.Beat()
		s.runOnce(ctx, abort, j)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// runOnce runs a job if this replica gets its lock: Redis SET NX (held for
// the interval, so replicas take turns), or a Postgres advisory lock when
// Redis is unavailable.
func (s *Scheduler) runOnce(ctx, abort context.Context, j job) {
	if ctx.Err() != nil {
		return
	}
	release, ok := s.lock(ctx, j)
	if !ok {
		metrics.JobRuns.WithLabelValues(j.name, "skipped").Inc()
		return
	}
	defer release()

	jctx, cancel := context.WithTimeout(abort, j.timeout)
	defer cancel()
	start := time.Now()
	err := j.fn(jctx)
	if err != nil {
		metrics.JobRuns.WithLabelValues(j.name, "error").Inc()
		s.log.Error().Err(err).Str("job", j.name).Dur("took", time.Since(start)).Msg("job failed")
		s.jobFailed(j.name, err)
		return
	}
	metrics.JobRuns.WithLabelValues(j.name, "success").Inc()
	metrics.JobLastSuccess.WithLabelValues(j.name).SetToCurrentTime()
	s.mu.Lock()
	s.failures[j.name] = 0
	s.mu.Unlock()
	s.log.Debug().Str("job", j.name).Dur("took", time.Since(start)).Msg("job done")
}

func (s *Scheduler) lock(ctx context.Context, j job) (release func(), ok bool) {
	ttl := j.every - time.Second
	if ttl < 10*time.Second {
		ttl = j.every
	}
	if s.rdb != nil {
		got, err := s.rdb.SetNX(ctx, "jobs:lock:"+j.name, "1", ttl).Result()
		if err == nil {
			s.setRedisDown(false)
			return func() {}, got
		}
		s.setRedisDown(true, err)
	}
	return s.advisoryLock(ctx, "jobs:"+j.name)
}

func (s *Scheduler) setRedisDown(down bool, err ...error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if down && !s.redisDown {
		var e error
		if len(err) > 0 {
			e = err[0]
		}
		s.log.Warn().Err(e).Msg("redis job lock unavailable; using postgres advisory locks")
	} else if !down && s.redisDown {
		s.log.Info().Msg("redis job locks restored")
	}
	s.redisDown = down
}

// advisoryLock takes a session-level Postgres advisory lock on a dedicated
// connection; release unlocks it and returns the connection.
func (s *Scheduler) advisoryLock(ctx context.Context, name string) (func(), bool) {
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		s.log.Error().Err(err).Str("lock", name).Msg("advisory lock: no connection")
		return nil, false
	}
	var got bool
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock($1, hashtext($2))`, lockKeyClass, name).Scan(&got); err != nil {
		conn.Release()
		s.log.Error().Err(err).Str("lock", name).Msg("advisory lock failed")
		return nil, false
	}
	if !got {
		conn.Release()
		return nil, false
	}
	return func() {
		uctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := conn.Exec(uctx, `SELECT pg_advisory_unlock($1, hashtext($2))`, lockKeyClass, name); err != nil {
			// Never return a connection that may still hold the lock.
			_ = conn.Conn().Close(uctx)
		}
		conn.Release()
	}, true
}

// jobFailed counts consecutive failures and raises an ops alert (at most
// hourly per job) once a job keeps failing.
func (s *Scheduler) jobFailed(name string, err error) {
	s.mu.Lock()
	s.failures[name]++
	n := s.failures[name]
	s.mu.Unlock()
	if n < jobFailingAfter || s.notifier == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, perr := s.notifier.Publish(ctx, notification.Event{
		Type:      notification.EventOpsJobFailing,
		DedupeKey: "ops:job_failing:" + name + ":" + time.Now().UTC().Format("2006010215"),
		Params:    map[string]string{"job": name, "failures": strconv.Itoa(n), "error": err.Error()},
	})
	if perr != nil {
		s.log.Error().Err(perr).Str("job", name).Msg("could not raise ops alert for failing job")
	}
}

// ── Jobs ─────────────────────────────────────────────────────────────────────

// GenerateShifts pre-generates concrete shifts from templates (D-003).
func (s *Scheduler) GenerateShifts(ctx context.Context) error {
	n, err := service.GenerateAllShifts(ctx, s.pool)
	if n > 0 {
		s.log.Info().Int("shifts", n).Msg("generated shifts")
	}
	return err
}

// AbandonStaleSessions closes patrol sessions left open too long (D-008).
// Their unscanned checkpoints are then evaluated by CheckSLA.
func (s *Scheduler) AbandonStaleSessions(ctx context.Context) error {
	ids, err := repository.ListStaleSessionIDs(ctx, s.pool, service.StaleSessionAge)
	if err != nil {
		return err
	}
	return eachIsolated(s.log, "abandon session", len(ids), func(i int) error {
		_, err := repository.EndSession(ctx, s.pool, ids[i], service.Now(), "abandoned")
		if errors.Is(err, repository.ErrNotFound) {
			return nil // ended concurrently
		}
		return err
	})
}

// FlagAbsent marks no-shows absent and notifies (FR-ATT-04). Each site is
// flagged and notified in its own transaction.
func (s *Scheduler) FlagAbsent(ctx context.Context) error {
	sites, err := repository.ListAllSites(ctx, s.pool)
	if err != nil {
		return err
	}
	return eachIsolated(s.log, "flag absent", len(sites), func(i int) error {
		return s.flagAbsentSite(ctx, &sites[i])
	})
}

func (s *Scheduler) flagAbsentSite(ctx context.Context, site *model.Site) error {
	var staged []notification.Staged
	now := service.Now()
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		staged = staged[:0]
		flags, err := repository.FlagAbsentees(ctx, tx, site.ID, model.DefaultSiteConfig().AbsentAfterMinutes, absentLookback)
		if err != nil {
			return err
		}
		for _, f := range flags {
			if now.Sub(f.StartTime) > absentNotifyWithin {
				s.log.Info().Str("record", f.RecordID).Time("shift_start", f.StartTime).Msg("flagged old absence without notification")
				continue
			}
			st, err := s.notifier.Stage(ctx, tx, notification.Event{
				Type: notification.EventAbsent, Site: site, DedupeKey: "absent:" + f.RecordID, RefID: f.RecordID,
				Params: map[string]string{
					"guard": f.GuardName,
					"shift": f.StartTime.In(site.Location()).Format("02 Jan 15:04"),
				},
			})
			if err != nil {
				return err
			}
			staged = append(staged, st)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("site %s: %w", site.ID, err)
	}
	s.notifier.Wake(ctx, staged...)
	return nil
}

// CheckSLA records and notifies checkpoint SLA breaches of running sessions
// and, once, of sessions that ended (FR-PAT-04, F2). See package sla.
func (s *Scheduler) CheckSLA(ctx context.Context) error {
	sessions, err := repository.ListActiveSessions(ctx, s.pool, "")
	if err != nil {
		return err
	}
	activeErr := eachIsolated(s.log, "SLA check", len(sessions), func(i int) error {
		_, err := s.sla.Evaluate(ctx, sessions[i])
		return err
	})
	ended, err := repository.ListUncheckedEndedSessions(ctx, s.pool, 200)
	if err != nil {
		return errors.Join(activeErr, err)
	}
	endedErr := eachIsolated(s.log, "SLA check of ended session", len(ended), func(i int) error {
		if _, err := s.sla.Evaluate(ctx, ended[i]); err != nil {
			return err
		}
		return repository.MarkSessionSLAChecked(ctx, s.pool, ended[i].ID)
	})
	return errors.Join(activeErr, endedErr)
}

// MonthlyRecap sends last month's recap from 07:00 site time on the 1st; if
// the worker was down it catches up until day recapCatchUpDays (F14). The
// dedupe key sends it once per site, month and recipient.
func (s *Scheduler) MonthlyRecap(ctx context.Context) error {
	sites, err := repository.ListAllSites(ctx, s.pool)
	if err != nil {
		return err
	}
	return eachIsolated(s.log, "monthly recap", len(sites), func(i int) error {
		return s.recapSite(ctx, &sites[i])
	})
}

// recapDue reports whether the recap of the previous month should be sent at
// the site-local time now, and which month that is ("YYYY-MM").
func recapDue(now time.Time) (string, bool) {
	if now.Day() > recapCatchUpDays || (now.Day() == 1 && now.Hour() < 7) {
		return "", false
	}
	return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).AddDate(0, -1, 0).Format("2006-01"), true
}

func (s *Scheduler) recapSite(ctx context.Context, site *model.Site) error {
	loc := site.Location()
	month, due := recapDue(service.Now().In(loc))
	if !due {
		return nil
	}
	rules, err := repository.ListRecipients(ctx, s.pool, site.ID, notification.EventMonthlyRecap)
	if err != nil || len(rules) == 0 {
		return err
	}
	from, to, err := service.MonthRange(month, loc)
	if err != nil {
		return err
	}
	rows, err := repository.MonthlyRecap(ctx, s.pool, site.ID, loc, from, to)
	if err != nil {
		return err
	}
	compliance, err := repository.PatrolComplianceTotals(ctx, s.pool, site.ID, from, to)
	if err != nil {
		return err
	}
	lang := site.Config().Language
	subject, _ := notification.Render(notification.EventMonthlyRecap, lang,
		map[string]string{"site": site.Name, "month": month})
	totals := repository.RecapTotals(rows)
	text := recapText(site, month, rows, totals, s.cfg.AppPublicURL)
	if line := recapCompliance(compliance, lang, false); line != "" {
		text = line + "\n" + text
	}
	_, err = s.notifier.Publish(ctx, notification.Event{
		Type: notification.EventMonthlyRecap, Site: site, DedupeKey: "recap:" + site.ID + ":" + month,
		Subject:  subject,
		Body:     recapHTML(site, month, rows, totals, s.cfg.AppPublicURL) + recapCompliance(compliance, lang, true),
		TextBody: text,
	})
	return err
}

// eachIsolated runs fn for items 0..n-1. A failing item is logged and
// counted but does not stop the others (audit BE-R4); the result reports
// how many failed ("flag absent: 1/3 failed: ...") or nil.
func eachIsolated(log zerolog.Logger, what string, n int, fn func(i int) error) error {
	var errs []error
	for i := 0; i < n; i++ {
		if err := fn(i); err != nil {
			log.Error().Err(err).Str("step", what).Int("item", i).Msg("item failed; continuing")
			errs = append(errs, err)
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("%s: %d/%d failed: %w", what, len(errs), n, errors.Join(errs...))
}
