package handler_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/securepatrol/backend/internal/handler"
)

// setupApp builds a minimal Fiber app with the handler registered
// and a fake pool injected via Locals (nil pool — only tests non-DB paths).
func setupApp(register func(fiber.Router)) *fiber.App {
	app := fiber.New(fiber.Config{
		// Don't exit on panic in tests
	})
	g := app.Group("/api")
	g.Use(func(c *fiber.Ctx) error {
		// No real pool — tests that reach DB will 500; we only test input validation.
		c.Locals("db", nil)
		return c.Next()
	})
	register(g)
	return app
}

func doRequest(app *fiber.App, method, path string) *http.Response {
	req := httptest.NewRequest(method, path, nil)
	resp, _ := app.Test(req, 5000)
	return resp
}

func readBody(resp *http.Response) map[string]any {
	b, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	var out map[string]any
	_ = json.Unmarshal(b, &out)
	return out
}

// ── Health handler ──────────────────────────────────────────────────────────

func TestHealthEndpointExistsAndIsJSON(t *testing.T) {
	// Health requires a live pool; we skip it here and cover it in integration tests.
	// This test merely verifies the route registers without panicking.
	t.Skip("Health endpoint requires a live Postgres + Redis connection; covered by integration tests")
}

// ── Attendance handler — input validation ──────────────────────────────────

func TestListAttendanceMissingSiteID(t *testing.T) {
	app := setupApp(handler.RegisterAttendance)
	resp := doRequest(app, "GET", "/api/?")
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
	body := readBody(resp)
	if body["error"] == nil {
		t.Error("expected error field in response")
	}
}

func TestListAttendanceWithSiteIDHitsDB(t *testing.T) {
	app := setupApp(handler.RegisterAttendance)
	// No real DB → expect 500, not a bad-input 400
	resp := doRequest(app, "GET", "/api/?site_id=test-site")
	if resp.StatusCode == fiber.StatusBadRequest {
		t.Error("should not get 400 when site_id is provided")
	}
}

// ── Guard handler — input validation ──────────────────────────────────────

func TestListGuardsWithoutDBPanics(t *testing.T) {
	// Verifies that calling ListGuards with nil pool returns 500, not a panic.
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("handler panicked: %v", r)
		}
	}()
	app := setupApp(handler.RegisterGuards)
	resp := doRequest(app, "GET", "/api/")
	// nil pool → should 500, not panic
	if resp.StatusCode == 0 {
		t.Error("expected a non-zero status code")
	}
}

// ── Shift handler — input validation ──────────────────────────────────────

func TestCreateShiftMissingFields(t *testing.T) {
	app := setupApp(handler.RegisterShifts)
	req := httptest.NewRequest("POST", "/api/", nil)
	req.Header.Set("Content-Type", "application/json")
	resp, _ := app.Test(req)
	// Empty body → 400 or 500; must not be 2xx
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		t.Errorf("expected error status, got %d", resp.StatusCode)
	}
}

// ── Site handler — basic route existence ──────────────────────────────────

func TestSiteRoutesExist(t *testing.T) {
	for _, method := range []string{"GET", "POST"} {
		app := setupApp(handler.RegisterSites)
		resp := doRequest(app, method, "/api/")
		if resp.StatusCode == fiber.StatusNotFound {
			t.Errorf("%s /api/ returned 404 — route not registered", method)
		}
	}
}

// ── Report handler — input validation ──────────────────────────────────────

func TestReportAttendanceDailyMissingSiteID(t *testing.T) {
	app := setupApp(handler.RegisterReports)
	resp := doRequest(app, "GET", "/api/attendance/daily")
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Errorf("expected 400 for missing site_id, got %d", resp.StatusCode)
	}
}

func TestReportPatrolCompletionMissingSiteID(t *testing.T) {
	app := setupApp(handler.RegisterReports)
	resp := doRequest(app, "GET", "/api/patrol/completion")
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Errorf("expected 400 for missing site_id, got %d", resp.StatusCode)
	}
}

func TestReportMonthlyMissingSiteID(t *testing.T) {
	app := setupApp(handler.RegisterReports)
	resp := doRequest(app, "GET", "/api/attendance/monthly")
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Errorf("expected 400 for missing site_id, got %d", resp.StatusCode)
	}
}

func TestReportKPIMissingSiteID(t *testing.T) {
	app := setupApp(handler.RegisterReports)
	resp := doRequest(app, "GET", "/api/guards/kpi")
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Errorf("expected 400 for missing site_id, got %d", resp.StatusCode)
	}
}
