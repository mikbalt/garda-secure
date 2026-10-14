package notification

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/securepatrol/backend/internal/push"
)

func TestAvailablePushEventsByRole(t *testing.T) {
	all := PushEvents()
	if len(all) != 12 {
		t.Fatalf("push events = %v", all)
	}
	cases := map[string][]string{
		roleAdmin: all,
		roleSupervisor: {EventSOS, EventSOSUnacknowledged, EventIncidentHigh, EventIncidentFiled, EventPatrolNotStarted,
			EventSLAMissed, EventAbsent, EventLateClockIn, EventAttendanceReview, EventDefectOpened},
		roleHR:           {EventAbsent, EventLateClockIn, EventAttendanceReview},
		roleClient:       {EventIncidentHigh, EventPatrolNotStarted},
		"platform-admin": {},
	}
	for role, want := range cases {
		if got := AvailablePushEvents([]string{role}); !slices.Equal(got, want) {
			t.Errorf("%s: %v, want %v", role, got, want)
		}
	}
	// Roles add up, in catalogue order.
	if got := AvailablePushEvents([]string{roleHR, roleClient}); !slices.Equal(got,
		[]string{EventIncidentHigh, EventPatrolNotStarted, EventAbsent, EventLateClockIn, EventAttendanceReview}) {
		t.Errorf("hr+client = %v", got)
	}
}

func TestPushDefaultsAndPreferences(t *testing.T) {
	sup := []string{roleSupervisor}
	for ev, want := range map[string]bool{
		EventSOS: true, EventSOSUnacknowledged: true, EventIncidentHigh: true, EventIncidentFiled: false,
		EventLateClockIn: false, EventAbsent: true, EventDevicePending: false,
	} {
		if got := PushWanted(sup, nil, ev); got != want {
			t.Errorf("supervisor default %s = %v, want %v", ev, got, want)
		}
	}
	client := []string{roleClient}
	if PushWanted(client, nil, EventIncidentHigh) || PushWanted(client, nil, EventPatrolNotStarted) {
		t.Error("client-readonly events must default to off")
	}
	if !PushWanted(client, map[string]bool{EventIncidentHigh: true}, EventIncidentHigh) {
		t.Error("client-readonly opt-in ignored")
	}
	if PushWanted(client, map[string]bool{EventSOS: true}, EventSOS) {
		t.Error("an override must not grant an event the role cannot receive")
	}
	if PushWanted(sup, map[string]bool{EventSOS: false}, EventSOS) {
		t.Error("opt-out of SOS ignored")
	}
	// A client who is also admin gets the admin default.
	if !PushWanted([]string{roleClient, roleAdmin}, nil, EventIncidentHigh) {
		t.Error("admin default lost with an extra client role")
	}
	eff := EffectivePushPrefs([]string{roleHR}, map[string]bool{EventLateClockIn: true, EventSOS: true})
	if len(eff) != 3 || !eff[EventLateClockIn] || !eff[EventAbsent] || !eff[EventAttendanceReview] {
		t.Errorf("effective hr prefs = %v", eff)
	}
}

func TestWebPayloadFor(t *testing.T) {
	cases := []struct{ event, ref, url, tag, severity string }{
		{EventSOS, "a1", "/incidents?tab=sos", "sos-a1", "critical"},
		{EventSOSUnacknowledged, "a1", "/incidents?tab=sos", "sos-a1", "critical"},
		{EventIncidentHigh, "i1", "/incidents?id=i1", "incident_high-i1", "high"},
		{EventIncidentFiled, "", "/incidents", "incident_filed", "normal"},
		{EventPatrolNotStarted, "e1", "/patrol", "patrol_not_started-e1", "high"},
		{EventAttendanceReview, "r1", "/attendance?tab=review", "attendance_review-r1", "normal"},
		{EventAbsent, "r2", "/attendance", "absent-r2", "high"},
		{EventDefectOpened, "d1", "/incidents?tab=defects", "defect_opened-d1", "normal"},
		{EventDevicePending, "dev", "/settings?tab=devices", "device_pending-dev", "normal"},
		{EventDeviceUnhealthy, "dev", "/settings?tab=devices", "device_unhealthy-dev", "high"},
		{EventPushTest, "", "/notifications", "test", "normal"},
	}
	for _, c := range cases {
		p := WebPayloadFor(c.event, c.ref, "s1", "T", "B")
		if p.URL != c.url || p.Tag != c.tag || p.Severity != c.severity || p.Event != c.event || p.SiteID != "s1" || p.Title != "T" {
			t.Errorf("%s: %+v", c.event, p)
		}
	}
}

func TestPushLanesAndTabletTypes(t *testing.T) {
	for _, ev := range []string{EventSOSUnacknowledged, push.TabletSOSAck} {
		if LaneFor(ev) != LaneSOS {
			t.Errorf("%s must use the sos lane", ev)
		}
	}
	for _, ev := range []string{EventDeviceUnhealthy, push.TabletSync, push.TabletDeviceStatus, EventAttendanceReview} {
		if LaneFor(ev) != LaneNormal {
			t.Errorf("%s must use the normal lane", ev)
		}
	}
	if tabletTypeFor(EventSOS) != push.TabletSOS || tabletTypeFor(EventPatrolNotStarted) != push.TabletPatrolNotStarted ||
		tabletTypeFor(EventIncidentHigh) != "" {
		t.Error("tablet message mapping")
	}
	if !IsPushOnly(EventDevicePending) || IsPushOnly(EventSOSUnacknowledged) || slices.Contains(EventTypes, EventAttendanceReview) {
		t.Error("push-only events must not be rule events")
	}
	if !slices.Contains(EventTypes, EventSOSUnacknowledged) {
		t.Error("sos_unacknowledged must be a rule event")
	}
}

func TestPushTemplatesRenderInBothLanguages(t *testing.T) {
	params := map[string]string{"guard": "Budi", "site": "Gudang A", "device": "Pos 1", "reason": "x", "minutes": "3",
		"escalation": "1", "time": "10:00", "actor": "sari", "status": "aktif"}
	for _, ev := range []string{EventSOSUnacknowledged, EventAttendanceReview, EventDevicePending, EventDeviceUnhealthy,
		push.TabletSOSAck, push.TabletDeviceStatus, EventPushTest} {
		for _, lang := range []string{"id", "en"} {
			s, b := Render(ev, lang, params)
			if s == ev || b == ev || strings.Contains(s+b, "{") {
				t.Errorf("%s/%s renders %q / %q", ev, lang, s, b)
			}
		}
	}
	s, _ := Render(EventPushTest, "en", nil)
	if !strings.Contains(s, "GARDA") {
		t.Errorf("test push title %q lacks the product name", s)
	}
}

// Push deliveries whose target is gone are dropped at once, without the
// rule-channel alarms (no ops alert, no sp_notifications_failed_total).
func TestDeliverDropsGonePushTargets(t *testing.T) {
	gone := &fakeChannel{fail: fmt.Errorf("%w: status 410", push.ErrGone)}
	flaky := &fakeChannel{fail: errors.New("push service status 503")}
	store := newFakeStore(
		delivery("gone", LaneSOS, "webpush", EventSOS, 0),
		delivery("flaky", LaneSOS, "fcm", EventSOS, 0),
		delivery("off", LaneNormal, "fcm", "sync", 0),
	)
	w := testWorker(store, map[string]Channel{"webpush": gone, "fcm": flaky}, 2)
	for w.claimAndSend(context.Background(), []string{LaneSOS, LaneNormal}) {
	}
	if _, ok := store.failed["gone"]; !ok {
		t.Errorf("gone target not dropped: retried=%v", store.retried)
	}
	if _, ok := store.retried["gone"]; ok {
		t.Error("gone target rescheduled")
	}
	if _, ok := store.retried["flaky"]; !ok {
		t.Error("transient push failure must follow the retry policy")
	}
	delete(w.channels, "fcm")
	store.add(delivery("off2", LaneNormal, "fcm", "sync", 0))
	for w.claimAndSend(context.Background(), []string{LaneNormal}) {
	}
	if msg := store.failed["off2"]; !strings.Contains(msg, "not configured") {
		t.Errorf("push without transport: failed=%q retried=%v", msg, store.retried)
	}
}
