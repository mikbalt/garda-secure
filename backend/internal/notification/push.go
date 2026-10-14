package notification

import (
	"context"
	"encoding/json"
	"slices"
	"strconv"
	"time"

	"github.com/securepatrol/backend/internal/model"
	"github.com/securepatrol/backend/internal/push"
	"github.com/securepatrol/backend/internal/repository"
)

// Push-only events: they have no rule recipients (e-mail/WhatsApp), only
// browser push to the users whose roles receive them.
const (
	// EventAttendanceReview: a clock-in landed in the review queue (D-007).
	EventAttendanceReview = "attendance_review"
	// EventDevicePending: a tablet registered and waits for approval.
	EventDevicePending = "device_pending"
	// EventDeviceUnhealthy: a tablet's sync, clock or beacon looks wrong.
	EventDeviceUnhealthy = "device_unhealthy"
	// EventPushTest is the test message of POST /push/test.
	EventPushTest = "test"
)

// Web roles (mirrors middleware; the notification package must not import it).
const (
	roleAdmin      = "admin"
	roleSupervisor = "supervisor"
	roleHR         = "hr"
	roleClient     = "client-readonly"
)

// pushEvent is one event users can receive as a browser push: which roles
// may receive it and whether it is on by default for that role.
type pushEvent struct {
	name     string
	defaults map[string]bool // role -> on by default
}

var staffOn = map[string]bool{roleAdmin: true, roleSupervisor: true}

// pushEvents is the push catalogue, in display order (SOS & incidents,
// patrol, attendance, operations).
var pushEvents = []pushEvent{
	{EventSOS, staffOn},
	{EventSOSUnacknowledged, staffOn},
	{EventIncidentHigh, map[string]bool{roleAdmin: true, roleSupervisor: true, roleClient: false}},
	{EventIncidentFiled, map[string]bool{roleAdmin: false, roleSupervisor: false}},
	{EventPatrolNotStarted, map[string]bool{roleAdmin: true, roleSupervisor: true, roleClient: false}},
	{EventSLAMissed, staffOn},
	{EventAbsent, map[string]bool{roleAdmin: true, roleSupervisor: true, roleHR: true}},
	{EventLateClockIn, map[string]bool{roleAdmin: false, roleSupervisor: false, roleHR: false}},
	{EventAttendanceReview, map[string]bool{roleAdmin: true, roleSupervisor: true, roleHR: true}},
	{EventDefectOpened, staffOn},
	{EventDevicePending, map[string]bool{roleAdmin: true}},
	{EventDeviceUnhealthy, map[string]bool{roleAdmin: true}},
}

func pushEventSpec(event string) (pushEvent, bool) {
	for _, e := range pushEvents {
		if e.name == event {
			return e, true
		}
	}
	return pushEvent{}, false
}

func (e pushEvent) roles() []string {
	out := make([]string, 0, len(e.defaults))
	for r := range e.defaults {
		out = append(out, r)
	}
	slices.Sort(out)
	return out
}

// PushEvents lists every push event in display order.
func PushEvents() []string {
	out := make([]string, len(pushEvents))
	for i, e := range pushEvents {
		out[i] = e.name
	}
	return out
}

// IsPushOnly reports whether an event exists only as a push (no rules).
func IsPushOnly(event string) bool {
	return event == EventAttendanceReview || event == EventDevicePending || event == EventDeviceUnhealthy
}

// AvailablePushEvents lists the push events any of roles may receive.
func AvailablePushEvents(roles []string) []string {
	out := []string{}
	for _, e := range pushEvents {
		for _, r := range roles {
			if _, ok := e.defaults[r]; ok {
				out = append(out, e.name)
				break
			}
		}
	}
	return out
}

// PushDefault is the default of an event for a user: on when it is on by
// default for any of their roles that may receive it.
func PushDefault(roles []string, event string) bool {
	e, ok := pushEventSpec(event)
	if !ok {
		return false
	}
	for _, r := range roles {
		if e.defaults[r] {
			return true
		}
	}
	return false
}

// PushWanted reports whether a user with roles and preference overrides
// receives an event: it must be available to a role, then the user's
// override wins over the default.
func PushWanted(roles []string, prefs map[string]bool, event string) bool {
	if !slices.Contains(AvailablePushEvents(roles), event) {
		return false
	}
	if v, ok := prefs[event]; ok {
		return v
	}
	return PushDefault(roles, event)
}

// EffectivePushPrefs is GET /push/preferences "events": the effective value
// of every available event.
func EffectivePushPrefs(roles []string, prefs map[string]bool) map[string]bool {
	out := map[string]bool{}
	for _, e := range AvailablePushEvents(roles) {
		out[e] = PushWanted(roles, prefs, e)
	}
	return out
}

// ── Payloads ─────────────────────────────────────────────────────────────────

// pushURL is the web app deep link of an event.
func pushURL(event, ref string) string {
	switch event {
	case EventSOS, EventSOSUnacknowledged:
		return "/incidents?tab=sos"
	case EventIncidentHigh, EventIncidentFiled:
		if ref != "" {
			return "/incidents?id=" + ref
		}
		return "/incidents"
	case EventDefectOpened:
		return "/incidents?tab=defects"
	case EventPatrolNotStarted, EventSLAMissed:
		return "/patrol"
	case EventAbsent, EventLateClockIn:
		return "/attendance"
	case EventAttendanceReview:
		return "/attendance?tab=review"
	case EventDevicePending, EventDeviceUnhealthy:
		return "/settings?tab=devices"
	case EventPushTest:
		return "/notifications"
	}
	return "/"
}

// pushSeverity: SOS is critical (stays on screen, urgent delivery); missed
// work and serious incidents are high; the rest normal.
func pushSeverity(event string) string {
	switch event {
	case EventSOS, EventSOSUnacknowledged:
		return push.SeverityCritical
	case EventIncidentHigh, EventPatrolNotStarted, EventSLAMissed, EventAbsent, EventDeviceUnhealthy:
		return push.SeverityHigh
	}
	return push.SeverityNormal
}

// pushTag groups notifications of one record in the browser: an SOS and its
// reminders share "sos-<id>", so a reminder replaces the original.
func pushTag(event, ref string) string {
	base := event
	if event == EventSOSUnacknowledged {
		base = EventSOS
	}
	if ref == "" {
		return base
	}
	return base + "-" + ref
}

// WebPayloadFor builds the browser payload of an event.
func WebPayloadFor(event, ref, siteID, title, body string) push.WebPayload {
	return push.WebPayload{
		Title: title, Body: body, URL: pushURL(event, ref), Tag: pushTag(event, ref),
		Event: event, Severity: pushSeverity(event), SiteID: siteID,
	}
}

// tabletTypeFor maps events that also go to the site's tablets.
func tabletTypeFor(event string) string {
	switch event {
	case EventSOS:
		return push.TabletSOS
	case EventPatrolNotStarted:
		return push.TabletPatrolNotStarted
	}
	return ""
}

// ── Staging ──────────────────────────────────────────────────────────────────

// defaultAudienceMaxAge applies when UsePush got no maximum age.
const defaultAudienceMaxAge = 30 * 24 * time.Hour

// UsePush enables push staging. Users whose snapshot is older than maxAge
// receive no pushes.
func (n *Notifier) UsePush(s *push.Service, maxAge time.Duration) {
	if maxAge <= 0 {
		maxAge = defaultAudienceMaxAge
	}
	n.push, n.audienceMaxAge = s, maxAge
}

// Push returns the configured transports (nil when push is off).
func (n *Notifier) Push() *push.Service {
	if n == nil {
		return nil
	}
	return n.push
}

// stagePush stages browser pushes to the event's audience and, for SOS and
// patrol-not-started, data messages to the site's tablets.
func (n *Notifier) stagePush(ctx context.Context, db repository.DBTX, ev Event, siteID *string, realm, title, body string) (Staged, error) {
	var out Staged
	if n.push.WebEnabled() && realm != "" {
		if spec, ok := pushEventSpec(ev.Type); ok {
			targets, err := repository.ListPushTargets(ctx, db, realm, spec.roles(), time.Now().Add(-n.audienceMaxAge))
			if err != nil {
				return out, err
			}
			sid := ""
			if siteID != nil {
				sid = *siteID
			}
			payload := WebPayloadFor(ev.Type, ev.RefID, sid, title, body)
			raw, err := payload.Marshal()
			if err != nil {
				return out, err
			}
			for _, t := range targets {
				if siteID != nil && t.Sites != nil && !slices.Contains(t.Sites, *siteID) {
					continue // site-scoped user without this site (D-042)
				}
				if !PushWanted(t.Roles, t.Prefs, ev.Type) {
					continue
				}
				id, err := n.insert(ctx, db, outboxRow{
					siteID: siteID, event: ev.Type, channel: repository.ChannelWebPush, recipient: t.SubscriptionID,
					subject: payload.Title, body: string(raw), dedupe: ev.DedupeKey, requestID: ev.RequestID,
				})
				if err != nil {
					return out, err
				}
				out.add(id)
			}
		}
	}
	if typ := tabletTypeFor(ev.Type); typ != "" && siteID != nil {
		s, err := n.StageTablets(ctx, db, TabletEvent{
			Type: typ, SiteID: *siteID, Exclude: ev.SourceDeviceID, RefID: ev.RefID,
			Title: title, Body: body, DedupeKey: ev.DedupeKey, RequestID: ev.RequestID,
		})
		if err != nil {
			return out, err
		}
		out.IDs = append(out.IDs, s.IDs...)
	}
	return out, nil
}

// TabletEvent is a data message to tablets over FCM.
type TabletEvent struct {
	Type string // push.Tablet*
	// SiteID: every active tablet of the site with a token receives it,
	// unless DeviceID names one tablet (device_status).
	SiteID   string
	DeviceID string
	Exclude  string // tablet that raised the event (SOS)
	RefID    string
	Title    string
	Body     string
	// DedupeKey makes staging idempotent per tablet.
	DedupeKey string
	// NotBefore delays sending (debounced sync).
	NotBefore *time.Time
	RequestID string
}

// StageTablets stages one FCM delivery per target tablet (no-op while FCM
// is not configured). Use the transaction that writes the domain change.
func (n *Notifier) StageTablets(ctx context.Context, db repository.DBTX, te TabletEvent) (Staged, error) {
	var out Staged
	if !n.push.FCMEnabled() {
		return out, nil
	}
	var targets []repository.TabletTarget
	if te.DeviceID != "" {
		t, err := repository.GetTabletTarget(ctx, db, te.DeviceID)
		if err != nil {
			if err == repository.ErrNotFound {
				return out, nil
			}
			return out, err
		}
		targets = []repository.TabletTarget{*t}
	} else {
		var err error
		if targets, err = repository.ListSiteTabletTargets(ctx, db, te.SiteID, te.Exclude); err != nil {
			return out, err
		}
	}
	if len(targets) == 0 {
		return out, nil
	}
	raw, err := json.Marshal(push.TabletMessage{Type: te.Type, SiteID: te.SiteID, RefID: te.RefID, Title: te.Title, Body: te.Body})
	if err != nil {
		return out, err
	}
	var siteID *string
	if te.SiteID != "" {
		siteID = &te.SiteID
	}
	for _, t := range targets {
		dedupe := ""
		if te.DedupeKey != "" {
			dedupe = te.DedupeKey + "|" + t.DeviceID // per tablet, stable across token refreshes
		}
		id, err := n.insert(ctx, db, outboxRow{
			siteID: siteID, event: te.Type, channel: repository.ChannelFCM, recipient: t.Token,
			subject: te.Title, body: string(raw), dedupe: dedupe, requestID: te.RequestID, notBefore: te.NotBefore,
		})
		if err != nil {
			return out, err
		}
		out.add(id)
	}
	return out, nil
}

// SyncDebounce is the minimum spacing of "sync" messages to a site.
const SyncDebounce = 30 * time.Second

// StageSync tells a site's tablets to re-sync roster, routes and schedules.
// Changes are collected per 30-second window and sent at its end, so a
// burst of edits sends one message and every change is followed by one.
func (n *Notifier) StageSync(ctx context.Context, db repository.DBTX, siteID string, now time.Time) (Staged, error) {
	if !n.push.FCMEnabled() || siteID == "" {
		return Staged{}, nil
	}
	at := now.Truncate(SyncDebounce).Add(SyncDebounce)
	return n.StageTablets(ctx, db, TabletEvent{
		Type: push.TabletSync, SiteID: siteID, NotBefore: &at,
		DedupeKey: "sync:" + siteID + ":" + strconv.FormatInt(at.Unix(), 10),
	})
}

// outboxRow is one notification_log row to insert.
type outboxRow struct {
	siteID                           *string
	event, channel, recipient        string
	subject, body, dedupe, requestID string
	notBefore                        *time.Time
}

// insert queues a delivery; dedupe is suffixed with channel and recipient
// for rule and web push rows (callers pass the event key).
func (n *Notifier) insert(ctx context.Context, db repository.DBTX, d outboxRow) (string, error) {
	var dedupe, reqID *string
	if d.dedupe != "" {
		k := d.dedupe
		if d.channel != repository.ChannelFCM {
			k += "|" + d.channel + "|" + d.recipient
		} else {
			k += "|" + d.channel
		}
		dedupe = &k
	}
	if d.requestID != "" {
		reqID = &d.requestID
	}
	return repository.InsertDelivery(ctx, db, model.NotificationLog{
		SiteID: d.siteID, EventType: d.event, Channel: d.channel, Recipient: d.recipient,
		Subject: d.subject, Body: d.body,
	}, LaneFor(d.event), reqID, dedupe, d.notBefore)
}

func (s *Staged) add(id string) {
	if id != "" {
		s.IDs = append(s.IDs, id)
	}
}
