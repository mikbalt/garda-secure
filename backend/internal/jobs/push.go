package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/securepatrol/backend/internal/config"
	"github.com/securepatrol/backend/internal/notification"
	"github.com/securepatrol/backend/internal/repository"
	"github.com/securepatrol/backend/internal/service"
)

// ── SOS escalation (sos_unacknowledged) ─────────────────────────────────────

// escalation returns SOS_ESCALATE_AFTER and SOS_ESCALATE_EVERY (defaults
// when the configuration was built without Load, e.g. in tests).
func (s *Scheduler) escalation() (after, every time.Duration) {
	after, every = 3*time.Minute, 5*time.Minute
	if s.cfg != nil {
		if s.cfg.Push.SOSEscalateAfter > 0 {
			after = s.cfg.Push.SOSEscalateAfter
		}
		if s.cfg.Push.SOSEscalateEvery > 0 {
			every = s.cfg.Push.SOSEscalateEvery
		}
	}
	return after, every
}

// EscalationDue returns the reminder number (1..SOSEscalations) due at now
// for an SOS received at received of which done reminders were staged, or 0.
// Reminder k is due at received + after + (k-1)*every and stays due for one
// interval. After an outage only the latest due reminder is sent, never a
// burst of them.
func EscalationDue(received, now time.Time, done int, after, every time.Duration) int {
	first := received.Add(after)
	if now.Before(first) || every <= 0 {
		return 0
	}
	k := 1 + int(now.Sub(first)/every)
	if k > config.SOSEscalations || k <= done {
		// Past the last reminder's interval (stale after an outage), or
		// already reminded.
		return 0
	}
	return k
}

// EscalateSOS reminds the SOS audience about alerts nobody acknowledged
// (sos_unacknowledged: rules and push), SOS_ESCALATE_AFTER after the alert
// arrived and then every SOS_ESCALATE_EVERY, 3 times at most.
func (s *Scheduler) EscalateSOS(ctx context.Context) error {
	after, every := s.escalation()
	now := service.Now()
	horizon := after + time.Duration(config.SOSEscalations)*every
	alerts, err := repository.ListSosForEscalation(ctx, s.pool, now.Add(-horizon), config.SOSEscalations)
	if err != nil {
		return err
	}
	return eachIsolated(s.log, "escalate SOS", len(alerts), func(i int) error {
		a := alerts[i]
		k := EscalationDue(a.ReceivedAt, now, a.Escalations, after, every)
		if k == 0 {
			return nil
		}
		return s.escalateSOS(ctx, a, k, now)
	})
}

func (s *Scheduler) escalateSOS(ctx context.Context, a repository.SosEscalation, k int, now time.Time) error {
	var staged notification.Staged
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		ok, err := repository.SetSosEscalations(ctx, tx, a.ID, k)
		if err != nil || !ok {
			return err // acknowledged or reminded meanwhile
		}
		site, err := repository.GetSite(ctx, tx, a.SiteID)
		if err != nil {
			return err
		}
		staged, err = s.notifier.Stage(ctx, tx, notification.Event{
			Type: notification.EventSOSUnacknowledged, Site: site, RefID: a.ID,
			DedupeKey: "sos_unack:" + a.ID + ":" + strconv.Itoa(k),
			Params: map[string]string{
				"guard": a.GuardName, "time": a.FiredAt.In(site.Location()).Format("02 Jan 15:04:05"),
				"minutes":    strconv.Itoa(int(now.Sub(a.ReceivedAt) / time.Minute)),
				"escalation": strconv.Itoa(k), "location": gpsText(a.GPSLat, a.GPSLng),
			},
		})
		return err
	})
	if err != nil {
		return fmt.Errorf("SOS %s: %w", a.ID, err)
	}
	s.notifier.Wake(ctx, staged)
	return nil
}

func gpsText(lat, lng *float64) string {
	if lat == nil || lng == nil {
		return ""
	}
	return fmt.Sprintf("%.6f,%.6f (https://maps.google.com/?q=%.6f,%.6f)", *lat, *lng, *lat, *lng)
}

// ── Tablet health (device_unhealthy) ─────────────────────────────────────────

// Tablet health thresholds.
const (
	outboxStuckAfter   = time.Hour
	maxClockSkew       = 2 * time.Minute
	beaconMissingAfter = time.Hour
	// unhealthyEvery debounces device_unhealthy per tablet.
	unhealthyEvery = 6 * time.Hour
)

// beacon is the part of the tablet health beacon the check reads.
type beacon struct {
	OutboxPending int        `json:"outbox_pending"`
	LastSync      *time.Time `json:"last_sync"`
	ClockOffsetMs int64      `json:"clock_offset_ms"`
}

// UnhealthyReasons evaluates an active tablet: outbox items unsynced for
// more than an hour, a clock off by more than 2 minutes, or no beacon for an
// hour although the tablet was active today (site time). lang is id or en.
func UnhealthyReasons(raw json.RawMessage, healthAt, lastSeen *time.Time, now time.Time, loc *time.Location, lang string) []string {
	en := lang == "en"
	hm := func(t time.Time) string { return t.In(loc).Format("15:04") }
	var out []string
	if healthAt == nil {
		return out // never sent a beacon (older app): nothing to judge
	}
	if now.Sub(*healthAt) > beaconMissingAfter {
		active := lastSeen != nil && sameDay(*lastSeen, now, loc) || sameDay(*healthAt, now, loc)
		if active {
			if en {
				out = append(out, "no status beacon since "+hm(*healthAt))
			} else {
				out = append(out, "tidak ada laporan status sejak "+hm(*healthAt))
			}
		}
		return out // an old beacon says nothing about the current state
	}
	var b beacon
	if json.Unmarshal(raw, &b) != nil {
		return out
	}
	if b.OutboxPending > 0 && (b.LastSync == nil || now.Sub(*b.LastSync) > outboxStuckAfter) {
		since := "-"
		if b.LastSync != nil {
			since = hm(*b.LastSync)
		}
		if en {
			out = append(out, fmt.Sprintf("%d records not synced (last sync %s)", b.OutboxPending, since))
		} else {
			out = append(out, fmt.Sprintf("%d data belum terkirim (sinkron terakhir %s)", b.OutboxPending, since))
		}
	}
	if skew := time.Duration(math.Abs(float64(b.ClockOffsetMs))) * time.Millisecond; skew > maxClockSkew {
		m := int(math.Round(skew.Minutes()))
		if en {
			out = append(out, fmt.Sprintf("clock is off by %d min", m))
		} else {
			out = append(out, fmt.Sprintf("jam tablet selisih %d menit", m))
		}
	}
	return out
}

func sameDay(a, b time.Time, loc *time.Location) bool {
	ay, am, ad := a.In(loc).Date()
	by, bm, bd := b.In(loc).Date()
	return ay == by && am == bm && ad == bd
}

// CheckDeviceHealth raises device_unhealthy (push to admins) for tablets
// whose beacon shows a problem, at most once per tablet every 6 hours.
func (s *Scheduler) CheckDeviceHealth(ctx context.Context) error {
	if !s.notifier.Push().WebEnabled() {
		return nil // push-only event
	}
	devices, err := repository.ListActiveDeviceHealth(ctx, s.pool)
	if err != nil {
		return err
	}
	now := service.Now()
	return eachIsolated(s.log, "device health", len(devices), func(i int) error {
		d := devices[i]
		if d.UnhealthyAlertedAt != nil && now.Sub(*d.UnhealthyAlertedAt) < unhealthyEvery {
			return nil
		}
		site, err := repository.GetSite(ctx, s.pool, d.SiteID)
		if err != nil {
			return err
		}
		reasons := UnhealthyReasons(d.Health, d.HealthAt, d.LastSeenAt, now, site.Location(), site.Config().Language)
		if len(reasons) == 0 {
			return nil
		}
		var staged notification.Staged
		err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
			ok, err := repository.MarkDeviceUnhealthyAlerted(ctx, tx, d.ID, unhealthyEvery)
			if err != nil || !ok {
				return err
			}
			staged, err = s.notifier.Stage(ctx, tx, notification.Event{
				Type: notification.EventDeviceUnhealthy, Site: site, RefID: d.ID,
				DedupeKey: "device_unhealthy:" + d.ID + ":" + strconv.FormatInt(now.Unix(), 10),
				Params:    map[string]string{"device": d.Name, "reason": strings.Join(reasons, "; ")},
			})
			return err
		})
		if err != nil {
			return fmt.Errorf("device %s: %w", d.ID, err)
		}
		s.notifier.Wake(ctx, staged)
		return nil
	})
}
