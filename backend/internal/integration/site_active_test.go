//go:build integration

package integration

import (
	"context"
	"testing"
	"time"
)

// Site soft delete (issue #34). Uses realm2 and its own sites only.
func TestSiteDeactivation(t *testing.T) {
	e := newEnv(t)
	admin := e.web(realm2, "sd-admin", "admin")
	sup := e.web(realm2, "sd-sup", "supervisor")
	ctx := context.Background()

	mk := func(name string) string {
		return e.expect(e.do("POST", "/api/v1/sites", admin, "", map[string]any{"name": name}), 201, "create "+name).str("id")
	}
	has := func(tok, path, id string) bool {
		for _, s := range e.do("GET", path, tok, "", nil).list() {
			if m, _ := s.(map[string]any); m["id"] == id {
				return true
			}
		}
		return false
	}

	site := mk("SD empty")
	if e.do("GET", "/api/v1/sites/"+site, admin, "", nil).obj()["active"] != true {
		t.Fatal("new site must be active")
	}
	e.expect(e.do("PATCH", "/api/v1/sites/"+site+"/deactivate", sup, "", nil), 403, "supervisor deactivates")
	e.expect(e.do("PATCH", "/api/v1/sites/"+site+"/deactivate", admin, "", nil), 200, "deactivate")

	if has(admin, "/api/v1/sites", site) {
		t.Fatal("inactive site still in default list")
	}
	if !has(admin, "/api/v1/sites?include_inactive=true", site) {
		t.Fatal("admin cannot see inactive site with include_inactive")
	}
	if has(sup, "/api/v1/sites?include_inactive=true", site) {
		t.Fatal("non-admin must not see inactive sites")
	}
	e.expect(e.do("PUT", "/api/v1/sites/"+site, admin, "", map[string]any{"name": "renamed"}), 409, "edit inactive site")
	e.expect(e.do("PATCH", "/api/v1/sites/"+site+"/activate", admin, "", nil), 200, "activate")
	if !has(admin, "/api/v1/sites", site) {
		t.Fatal("reactivated site missing from list")
	}
	e.expect(e.do("PUT", "/api/v1/sites/"+site, admin, "", map[string]any{"name": "SD renamed"}), 200, "edit active site")

	// Blocked by an active tablet.
	if _, err := e.pool.Exec(ctx, `INSERT INTO devices (keycloak_realm, keycloak_user_id, name, site_id, status)
		VALUES ($1, 'sd-dev', 'SD tablet', $2, 'active')`, realm2, site); err != nil {
		t.Fatal(err)
	}
	r := e.expect(e.do("PATCH", "/api/v1/sites/"+site+"/deactivate", admin, "", nil), 409, "active device blocks")
	if r.obj()["code"] != "site_has_active_devices" {
		t.Fatalf("code = %v", r.obj()["code"])
	}
	if _, err := e.pool.Exec(ctx, `UPDATE devices SET status = 'revoked' WHERE keycloak_user_id = 'sd-dev'`); err != nil {
		t.Fatal(err)
	}

	// Blocked by a future shift.
	guard := e.expect(e.do("POST", "/api/v1/guards", admin, "", map[string]any{"name": "SD guard", "site_id": site, "role": "guard"}), 201, "guard").str("id")
	start := time.Now().UTC().Add(48 * time.Hour)
	e.expect(e.do("POST", "/api/v1/shifts", admin, "", map[string]any{
		"site_id": site, "guard_id": guard, "start_time": rfc(start), "end_time": rfc(start.Add(8 * time.Hour)),
	}), 201, "future shift")
	r = e.expect(e.do("PATCH", "/api/v1/sites/"+site+"/deactivate", admin, "", nil), 409, "future shift blocks")
	if r.obj()["code"] != "site_has_future_shifts" {
		t.Fatalf("code = %v", r.obj()["code"])
	}

	e.expect(e.do("PATCH", "/api/v1/sites/"+mk("SD other")+"/deactivate", e.web(realm1, "sd-admin1", "admin"), "", nil), 404, "other realm")
}
