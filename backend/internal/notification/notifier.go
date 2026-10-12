// Package notification turns domain events into per-recipient deliveries
// (notification_log rows) and dispatches them from the worker.
//
// Delivery is a transactional outbox (audit BE-R1): Stage writes the
// notification_log rows in the same transaction as the domain row (SOS,
// incident, ...). Postgres is the source of truth; workers claim due rows from
// it (BE-R5). Redis only carries a wake-up signal that shortens latency.
package notification

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
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
	// EventIncidentFiled fires for every new incident, whatever its severity
	// (F8, D-035); incident_high is staged next to it for high/critical.
	EventIncidentFiled = "incident_filed"
	EventDefectOpened = "defect_opened"
	EventMonthlyRecap = "monthly_recap"
)

// EventTypes lists every site event a rule can subscribe to.
var EventTypes = []string{
	EventSOS, EventSLAMissed, EventAbsent, EventLateClockIn,
	EventIncidentHigh, EventIncidentFiled, EventDefectOpened, EventMonthlyRecap,
}

// Install-wide operations events. Their recipients are ops_alert_rules, not
// the per-site notification_rules.
const (
	EventOpsBackupFailed   = "ops_backup_failed"   // a backup run failed
	EventOpsBackupStale    = "ops_backup_stale"    // no successful backup for > 26 h
	EventOpsSOSUndelivered = "ops_sos_undelivered" // an SOS delivery gave up, or a site has no SOS recipients
	EventOpsJobFailing     = "ops_job_failing"     // a scheduled job keeps failing
)

// OpsEventTypes lists every ops event an ops rule can subscribe to.
var OpsEventTypes = []string{EventOpsBackupFailed, EventOpsBackupStale, EventOpsSOSUndelivered, EventOpsJobFailing}

// IsOpsEvent reports whether t is an install-wide operations event.
func IsOpsEvent(t string) bool { return strings.HasPrefix(t, "ops_") }

// Lanes. SOS deliveries have their own lane so they never queue behind e-mail.
const (
	LaneSOS    = "sos"
	LaneNormal = "normal"
)

// LaneFor returns the dispatch lane of an event type.
func LaneFor(eventType string) string {
	if eventType == EventSOS || eventType == EventOpsSOSUndelivered {
		return LaneSOS
	}
	return LaneNormal
}

// wakeKey is the Redis list workers block on; entries carry no meaning beyond
// "there is new work".
const wakeKey = "notifications:wake"

// wakeCap bounds the wake list while no worker is consuming it.
const wakeCap = 1000

// Event is a domain event to fan out to the recipients configured for a site
// (or, for ops events, to the install-wide ops recipients).
type Event struct {
	Type   string
	Site   *model.Site       // nil for ops events
	Params map[string]string // placeholders used by the message templates
	// DedupeKey makes publishing idempotent (e.g. "absent:<record id>").
	// Each recipient gets "<DedupeKey>|<channel>|<recipient>".
	DedupeKey string
	// Subject/Body override the rendered text (monthly recap e-mail).
	Subject, Body string
	// TextBody replaces an HTML Body for channels that cannot render HTML
	// (WhatsApp).
	TextBody string
	// RequestID links the delivery to the API request that caused it.
	RequestID string
}

// Staged is the result of staging an event: the new delivery IDs.
type Staged struct {
	IDs []string
}

// Notifier stages events and wakes the workers.
type Notifier struct {
	pool *pgxpool.Pool
	rdb  *redis.Client
	log  zerolog.Logger
}

// NewNotifier creates a notifier.
func NewNotifier(pool *pgxpool.Pool, rdb *redis.Client, log zerolog.Logger) *Notifier {
	return &Notifier{pool: pool, rdb: rdb, log: log.With().Str("component", "notifier").Logger()}
}

// Stage records one delivery per enabled recipient using db, which should be
// the transaction that writes the domain row. Already-staged deliveries
// (same dedupe key) are skipped, so staging again on an idempotent replay
// only fills in what is missing. After commit call Wake.
func (n *Notifier) Stage(ctx context.Context, db repository.DBTX, ev Event) (Staged, error) {
	return stage(ctx, db, n.log, ev)
}

func stage(ctx context.Context, db repository.DBTX, log zerolog.Logger, ev Event) (Staged, error) {
	var out Staged
	type recipient struct{ channel, address string }
	var recipients []recipient
	var siteID *string
	lang := "id"
	params := map[string]string{}

	if IsOpsEvent(ev.Type) {
		rules, err := repository.ListOpsRecipients(ctx, db, ev.Type)
		if err != nil {
			return out, err
		}
		for _, r := range rules {
			recipients = append(recipients, recipient{r.Channel, r.Recipient})
		}
		if len(recipients) == 0 {
			log.Warn().Str("event", ev.Type).Str("dedupe", ev.DedupeKey).Str("request_id", ev.RequestID).
				Msg("ops alert has no recipients (configure ops alert rules)")
		}
	} else {
		if ev.Site == nil {
			return out, fmt.Errorf("stage %s: site is required", ev.Type)
		}
		rules, err := repository.ListRecipients(ctx, db, ev.Site.ID, ev.Type)
		if err != nil {
			return out, err
		}
		for _, r := range rules {
			recipients = append(recipients, recipient{r.Channel, r.Recipient})
		}
		id := ev.Site.ID
		siteID = &id
		lang = ev.Site.Config().Language
		params["site"] = ev.Site.Name
		if len(recipients) == 0 && ev.Type == EventSOS {
			// An SOS nobody receives is an operations problem, not a no-op.
			log.Error().Str("site_id", id).Str("dedupe", ev.DedupeKey).Str("request_id", ev.RequestID).
				Msg("SOS has no recipients at this site; alerting ops")
			ops := Event{
				Type: EventOpsSOSUndelivered, DedupeKey: "ops:sos_no_recipients:" + ev.DedupeKey, RequestID: ev.RequestID,
				Params: map[string]string{
					"site": ev.Site.Name, "guard": ev.Params["guard"], "time": ev.Params["time"],
					"recipient": "(none)", "reason": "no SOS notification rule is configured for this site",
				},
			}
			return stage(ctx, db, log, ops)
		}
	}
	if len(recipients) == 0 {
		return out, nil
	}

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
	var reqID *string
	if ev.RequestID != "" {
		reqID = &ev.RequestID
	}
	lane := LaneFor(ev.Type)

	for _, r := range recipients {
		var dedupe *string
		if ev.DedupeKey != "" {
			k := ev.DedupeKey + "|" + r.channel + "|" + r.address
			dedupe = &k
		}
		b := body
		if r.channel != "email" && ev.TextBody != "" {
			b = ev.TextBody
		}
		id, err := repository.InsertNotificationLog(ctx, db, model.NotificationLog{
			SiteID: siteID, EventType: ev.Type, Channel: r.channel, Recipient: r.address,
			Subject: subject, Body: b,
		}, lane, reqID, dedupe)
		if err != nil {
			return out, err
		}
		if id != "" {
			out.IDs = append(out.IDs, id)
		}
	}
	return out, nil
}

// Wake signals the workers that staged deliveries are ready. It never fails
// the caller: if Redis is down the workers still find the rows by polling
// Postgres within a couple of seconds.
func (n *Notifier) Wake(ctx context.Context, staged ...Staged) {
	total := 0
	for _, s := range staged {
		total += len(s.IDs)
	}
	if total == 0 || n.rdb == nil {
		return
	}
	if err := wake(ctx, n.rdb); err != nil {
		n.log.Warn().Err(err).Int("deliveries", total).Msg("wake-up via redis failed; workers will poll postgres")
	}
}

func wake(ctx context.Context, rdb *redis.Client) error {
	pipe := rdb.Pipeline()
	pipe.LPush(ctx, wakeKey, "1")
	pipe.LTrim(ctx, wakeKey, 0, wakeCap-1)
	_, err := pipe.Exec(ctx)
	return err
}

// Publish stages an event in its own transaction and wakes the workers. Use
// it when there is no domain row to write atomically (jobs, ops alerts).
// It returns the number of new deliveries.
func (n *Notifier) Publish(ctx context.Context, ev Event) (int, error) {
	var s Staged
	err := pgx.BeginFunc(ctx, n.pool, func(tx pgx.Tx) error {
		var err error
		s, err = n.Stage(ctx, tx, ev)
		return err
	})
	if err != nil {
		return 0, fmt.Errorf("publish %s: %w", ev.Type, err)
	}
	n.Wake(ctx, s)
	return len(s.IDs), nil
}

// Enqueue wakes the workers for an existing delivery (e.g. an admin retry
// after RequeueNotification). The row itself is already due in Postgres.
func Enqueue(ctx context.Context, rdb *redis.Client, _ string) error {
	return wake(ctx, rdb)
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
		EventIncidentFiled: {"[Insiden] {type} di {site}",
			"Insiden baru ({severity}) dilaporkan oleh {guard} di {site} pada {time}.\nKategori: {type}\nLokasi: {location}\n{description}"},
		EventDefectOpened: {"[Kerusakan] {defect} di {site}",
			"Laporan kerusakan baru dari {guard} di {site}.\nJenis: {defect}\nLokasi: {location}\n{description}"},
		EventMonthlyRecap: {"Rekap absensi bulanan {month} — {site}", "Rekap absensi {month} untuk {site}."},
		EventOpsBackupFailed: {"[OPS] Backup database gagal",
			"Backup {backup} ({trigger}) gagal pada {time}: {error}. Periksa log worker dan ruang penyimpanan MinIO."},
		EventOpsBackupStale: {"[OPS] Tidak ada backup berhasil sejak {last}",
			"Backup database terakhir yang berhasil: {last}. Backup harian (jam {hour}) tidak berjalan atau gagal. Periksa worker."},
		EventOpsSOSUndelivered: {"[OPS] SOS tidak terkirim — {site}",
			"SOS dari {guard} di {site} ({time}) tidak terkirim ke {recipient}: {reason}. Hubungi lokasi secara langsung dan periksa pengaturan notifikasi."},
		EventOpsJobFailing: {"[OPS] Job {job} terus gagal",
			"Job terjadwal {job} gagal {failures} kali berturut-turut. Error terakhir: {error}."},
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
		EventIncidentFiled: {"[Incident] {type} at {site}",
			"New incident ({severity}) reported by {guard} at {site} at {time}.\nCategory: {type}\nLocation: {location}\n{description}"},
		EventDefectOpened: {"[Defect] {defect} at {site}",
			"New defect report from {guard} at {site}.\nType: {defect}\nLocation: {location}\n{description}"},
		EventMonthlyRecap: {"Monthly attendance recap {month} — {site}", "Attendance recap {month} for {site}."},
		EventOpsBackupFailed: {"[OPS] Database backup failed",
			"Backup {backup} ({trigger}) failed at {time}: {error}. Check the worker logs and MinIO storage."},
		EventOpsBackupStale: {"[OPS] No successful backup since {last}",
			"The newest successful database backup is from {last}. The daily backup ({hour}:00) is not running or failing. Check the worker."},
		EventOpsSOSUndelivered: {"[OPS] SOS not delivered — {site}",
			"SOS from {guard} at {site} ({time}) could not be delivered to {recipient}: {reason}. Contact the site directly and check the notification settings."},
		EventOpsJobFailing: {"[OPS] Job {job} keeps failing",
			"Scheduled job {job} failed {failures} times in a row. Last error: {error}."},
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
