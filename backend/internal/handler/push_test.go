package handler

import (
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/securepatrol/backend/internal/middleware"
	"github.com/securepatrol/backend/internal/model"
)

func TestPushUserOfKeepsSiteScope(t *testing.T) {
	all := pushUserOf(&middleware.Principal{Realm: "r", Subject: "u", Roles: []string{"admin"}})
	if all.Sites != nil || all.Realm != "r" || all.UserID != "u" {
		t.Errorf("unrestricted user = %+v", all)
	}
	none := pushUserOf(&middleware.Principal{Realm: "r", Subject: "u", SitesRestricted: true})
	if none.Sites == nil || len(none.Sites) != 0 {
		t.Errorf("a claim with only invalid sites must restrict to no site, got %#v", none.Sites)
	}
	some := pushUserOf(&middleware.Principal{Realm: "r", Subject: "u", SitesRestricted: true, Sites: []string{"s1"}})
	if !slices.Equal(some.Sites, []string{"s1"}) {
		t.Errorf("scoped user = %+v", some)
	}
}

func TestRefreshDueThrottles(t *testing.T) {
	now := time.Now()
	key := "realm|refresh-test-" + now.String()
	if !refreshDue(key, "a", now) {
		t.Fatal("first request must refresh")
	}
	if refreshDue(key, "a", now.Add(time.Minute)) {
		t.Error("unchanged audience refreshed within the throttle")
	}
	if !refreshDue(key, "b", now.Add(2*time.Minute)) {
		t.Error("changed roles/sites must refresh at once")
	}
	if !refreshDue(key, "b", now.Add(13*time.Minute)) {
		t.Error("refresh after the throttle window")
	}
}

func TestMarkSyncCollectsSites(t *testing.T) {
	app := fiber.New()
	var got []string
	app.Get("/", func(c *fiber.Ctx) error {
		a, b, empty := "a", "b", ""
		markSync(c, &a, nil, &empty)
		markSync(c, &b, &a)
		got, _ = c.Locals(pushSyncKey).([]string)
		return nil
	})
	if _, err := app.Test(httptest.NewRequest("GET", "/", nil)); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, []string{"a", "b"}) {
		t.Errorf("sync sites = %v", got)
	}
}

func TestReviewReasonAndStatusText(t *testing.T) {
	note := "late sync replaced absent"
	if r := reviewReason(&model.AttendanceRecord{Flags: []string{"late_sync", "gps_far"}}); r != "late_sync, gps_far" {
		t.Errorf("flags reason = %q", r)
	}
	if r := reviewReason(&model.AttendanceRecord{Flags: []string{}, ReviewNote: &note}); r != note {
		t.Errorf("note reason = %q", r)
	}
	if deviceStatusText("active", "id") != "aktif" || deviceStatusText("revoked", "en") != "revoked" || deviceStatusText("x", "id") != "x" {
		t.Error("device status text")
	}
}
