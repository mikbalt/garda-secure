// Package notification turns domain events into per-recipient deliveries
// (notification_log rows) and dispatches them from the worker.
package notification

import (
	"context"
	"fmt"
	"strings"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"

	"github.com/securepatrol/backend/internal/model"
	"github.com/securepatrol/backend/internal/repository"
)

// Event types (PRD FR-NOT-03 plus the monthly recap).
const (
	EventSOS          = "sos"
	EventSLAMissed    = "sla_missed"
	EventAbsent       = "absent"
	EventLateClockIn  = "late_clock_in"
	EventIncidentHigh = "incident_high"
	EventDefectOpened = "defect_opened"
	EventMonthlyRecap = "monthly_recap"
)

// EventTypes lists every event a rule can subscribe to.
var EventTypes = []string{
	EventSOS, EventSLAMissed, EventAbsent, EventLateClockIn,
	EventIncidentHigh, EventDefectOpened, EventMonthlyRecap,
}

// Redis keys.
const (
	queueKey   = "notifications:queue"   // list of notification_log IDs ready to send
	delayedKey = "notifications:delayed" // zset of IDs scored by retry time
)

// Event is a domain event to fan out to the recipients configured for a site.
type Event struct {
	Type   string
	Site   *model.Site
	Params map[string]string // placeholders used by the message templates
	// DedupeKey makes publishing idempotent (e.g. "absent:<record id>").
	// Each recipient gets "<DedupeKey>|<channel>|<recipient>".
	DedupeKey string
	// Body overrides the rendered body (used for the monthly recap e-mail).
	Subject, Body string
}

// Notifier publishes events.
type Notifier struct {
	db  repository.DBTX
	rdb *redis.Client
	log zerolog.Logger
}

// NewNotifier creates a notifier.
func NewNotifier(db repository.DBTX, rdb *redis.Client, log zerolog.Logger) *Notifier {
	return &Notifier{db: db, rdb: rdb, log: log.With().Str("component", "notifier").Logger()}
}

// Publish records one delivery per enabled recipient and enqueues it. It
// returns the number of new deliveries. Failures to reach Redis are not
// fatal: the worker's sweeper re-enqueues rows left in "queued".
func (n *Notifier) Publish(ctx context.Context, ev Event) (int, error) {
	if ev.Site == nil {
		return 0, fmt.Errorf("publish %s: site is required", ev.Type)
	}
	rules, err := repository.ListRecipients(ctx, n.db, ev.Site.ID, ev.Type)
	if err != nil {
		return 0, err
	}
	if len(rules) == 0 {
		return 0, nil
	}

	lang := ev.Site.Config().Language
	params := map[string]string{"site": ev.Site.Name}
	for k, v := range ev.Params {
		params[k] = v
	}
	subject, body := ev.Subject, ev.Body
	if subject == "" || body == "" {
		s, b := Render(ev.Type, lang, params)
		if subject == "" {
			subject = s
		}
		if body == "" {
			body = b
		}
	}

	added := 0
	for _, r := range rules {
		var dedupe *string
		if ev.DedupeKey != "" {
			k := ev.DedupeKey + "|" + r.Channel + "|" + r.Recipient
			dedupe = &k
		}
		siteID := ev.Site.ID
		id, err := repository.InsertNotificationLog(ctx, n.db, model.NotificationLog{
			SiteID: &siteID, EventType: ev.Type, Channel: r.Channel, Recipient: r.Recipient,
			Subject: subject, Body: body,
		}, dedupe)
		if err != nil {
			return added, err
		}
		if id == "" {
			continue // already published
		}
		added++
		if err := n.rdb.LPush(ctx, queueKey, id).Err(); err != nil {
			n.log.Warn().Err(err).Str("id", id).Msg("enqueue failed; sweeper will retry")
		}
	}
	return added, nil
}

// Enqueue pushes an existing delivery (e.g. an admin retry) onto the queue.
func Enqueue(ctx context.Context, rdb *redis.Client, id string) error {
	return rdb.LPush(ctx, queueKey, id).Err()
}

// ── Message templates ────────────────────────────────────────────────────────

type template struct{ subject, body string }

var templates = map[string]map[string]template{
	"id": {
		EventSOS: {"[SOS] {guard} di {site}",
			"SOS! {guard} menekan tombol darurat di {site} pada {time}. Lokasi terakhir: {location}. Segera tindak lanjuti dan konfirmasi di dashboard."},
		EventSLAMissed: {"[SLA] Checkpoint terlewat di {site}",
			"Checkpoint \"{checkpoint}\" pada rute {route} di {site} belum dipindai oleh {guard}. Batas waktu: {due}."},
		EventAbsent: {"[Absen] {guard} tidak hadir di {site}",
			"{guard} belum clock-in untuk shift {shift} di {site} dan ditandai ABSEN."},
		EventLateClockIn: {"[Terlambat] {guard} di {site}",
			"{guard} clock-in terlambat {minutes} menit untuk shift {shift} di {site}."},
		EventIncidentHigh: {"[Insiden {severity}] {type} di {site}",
			"Insiden {severity} dilaporkan oleh {guard} di {site} pada {time}.\nKategori: {type}\nLokasi: {location}\n{description}"},
		EventDefectOpened: {"[Kerusakan] {defect} di {site}",
			"Laporan kerusakan baru dari {guard} di {site}.\nJenis: {defect}\nLokasi: {location}\n{description}"},
		EventMonthlyRecap: {"Rekap absensi bulanan {month} — {site}", "Rekap absensi {month} untuk {site}."},
	},
	"en": {
		EventSOS: {"[SOS] {guard} at {site}",
			"SOS! {guard} pressed the panic button at {site} at {time}. Last known location: {location}. Respond and acknowledge in the dashboard."},
		EventSLAMissed: {"[SLA] Missed checkpoint at {site}",
			"Checkpoint \"{checkpoint}\" on route {route} at {site} was not scanned by {guard}. Due: {due}."},
		EventAbsent: {"[Absent] {guard} at {site}",
			"{guard} did not clock in for the {shift} shift at {site} and was marked ABSENT."},
		EventLateClockIn: {"[Late] {guard} at {site}",
			"{guard} clocked in {minutes} minutes late for the {shift} shift at {site}."},
		EventIncidentHigh: {"[{severity} incident] {type} at {site}",
			"{severity} incident reported by {guard} at {site} at {time}.\nCategory: {type}\nLocation: {location}\n{description}"},
		EventDefectOpened: {"[Defect] {defect} at {site}",
			"New defect report from {guard} at {site}.\nType: {defect}\nLocation: {location}\n{description}"},
		EventMonthlyRecap: {"Monthly attendance recap {month} — {site}", "Attendance recap {month} for {site}."},
	},
}

// Render produces subject and body for an event in a language ("id" or "en").
// Unknown placeholders render as "-".
func Render(eventType, lang string, params map[string]string) (string, string) {
	set, ok := templates[lang]
	if !ok {
		set = templates["id"]
	}
	t, ok := set[eventType]
	if !ok {
		return eventType, eventType
	}
	return fill(t.subject, params), strings.TrimSpace(fill(t.body, params))
}

func fill(s string, params map[string]string) string {
	var b strings.Builder
	for {
		start := strings.IndexByte(s, '{')
		if start < 0 {
			b.WriteString(s)
			return b.String()
		}
		end := strings.IndexByte(s[start:], '}')
		if end < 0 {
			b.WriteString(s)
			return b.String()
		}
		b.WriteString(s[:start])
		key := s[start+1 : start+end]
		if v := params[key]; v != "" {
			b.WriteString(v)
		} else {
			b.WriteString("-")
		}
		s = s[start+end+1:]
	}
}
