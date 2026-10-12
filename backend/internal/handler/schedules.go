package handler

import (
	"context"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"

	"github.com/securepatrol/backend/internal/export"
	"github.com/securepatrol/backend/internal/middleware"
	"github.com/securepatrol/backend/internal/model"
	"github.com/securepatrol/backend/internal/repository"
	"github.com/securepatrol/backend/internal/service"
)

// ── Route schedules (D-035) ──────────────────────────────────────────────────

// GET /patrol/routes/:id/schedules
func (h *Handler) getRouteSchedules(c *fiber.Ctx) error {
	r, err := h.routeFor(c)
	if err != nil {
		return h.fail(c, err)
	}
	list, err := repository.ListRouteSchedules(c.Context(), h.DB, r.ID)
	if err != nil {
		return h.fail(c, err)
	}
	return c.JSON(fiber.Map{"route_id": r.ID, "schedules": list})
}

// PUT /patrol/routes/:id/schedules {schedules: [...]} — replaces the route's
// windows and regenerates its future scheduled patrols.
func (h *Handler) putRouteSchedules(c *fiber.Ctx) error {
	r, err := h.routeFor(c)
	if err != nil {
		return h.fail(c, err)
	}
	var b struct {
		Schedules []service.ScheduleInput `json:"schedules"`
	}
	if err := parse(c, &b); err != nil {
		return h.fail(c, err)
	}
	schedules, err := service.ValidateSchedules(b.Schedules)
	if err != nil {
		return h.fail(c, err)
	}
	ctx := c.Context()
	before, err := repository.ListRouteSchedules(ctx, h.DB, r.ID)
	if err != nil {
		return h.fail(c, err)
	}
	middleware.SetAuditBefore(c, fiber.Map{"route_id": r.ID, "schedules": before})
	site, err := repository.GetSite(ctx, h.DB, r.SiteID)
	if err != nil {
		return h.fail(c, err)
	}
	var out []model.RouteSchedule
	generated := 0
	err = pgx.BeginFunc(ctx, h.DB, func(tx pgx.Tx) error {
		var err error
		if out, err = repository.ReplaceRouteSchedules(ctx, tx, r.ID, schedules); err != nil {
			return err
		}
		generated, err = service.SyncRouteExpectations(ctx, tx, r, site)
		return err
	})
	if err != nil {
		return h.fail(c, err)
	}
	return c.JSON(fiber.Map{"route_id": r.ID, "schedules": out, "generated_expectations": generated})
}

// patrolDue is a scheduled patrol shown on the tablet.
type patrolDue struct {
	RouteID    string    `json:"route_id"`
	RouteName  string    `json:"route_name"`
	DueAt      time.Time `json:"due_at"`
	GraceUntil time.Time `json:"grace_until"`
	Status     string    `json:"status"` // pending | missed
}

// patrolsDueLookback is how long a missed patrol stays on the tablet (it can
// still be started late, see service.LateStartMax).
const patrolsDueLookback = service.LateStartMax

// patrolsDue lists the site's next and overdue scheduled patrols (max 5).
func (h *Handler) patrolsDue(ctx context.Context, siteID string, now time.Time) ([]patrolDue, error) {
	rows, err := repository.ListPatrolsDue(ctx, h.DB, siteID, now.Add(-patrolsDueLookback), now.Add(24*time.Hour), 5)
	if err != nil {
		return nil, err
	}
	out := make([]patrolDue, 0, len(rows))
	for _, e := range rows {
		out = append(out, patrolDue{RouteID: e.RouteID, RouteName: e.RouteName, DueAt: e.DueAt, GraceUntil: e.GraceUntil, Status: e.Status})
	}
	return out, nil
}

// GET /reports/patrol-compliance?site_id=&from=&to=&format=csv|xlsx — per
// route and site-local day: expected, started on time, late, missed, compliance %.
func (h *Handler) reportPatrolCompliance(c *fiber.Ctx) error {
	site, err := h.siteFor(c, c.Query("site_id"))
	if err != nil {
		return h.fail(c, err)
	}
	format, err := exportFormat(c, false)
	if err != nil {
		return h.fail(c, err)
	}
	loc := site.Location()
	from, to, err := service.DateRange(c.Query("from"), c.Query("to"), loc, 92)
	if err != nil {
		return h.fail(c, err)
	}
	rows, err := repository.PatrolCompliance(c.Context(), h.DB, site.ID, from, to, loc.String())
	if err != nil {
		return h.fail(c, err)
	}
	totals, err := repository.PatrolComplianceTotals(c.Context(), h.DB, site.ID, from, to)
	if err != nil {
		return h.fail(c, err)
	}
	if format == "" {
		return c.JSON(fiber.Map{
			"site_id": site.ID, "from": from.In(loc).Format("2006-01-02"),
			"to": to.In(loc).AddDate(0, 0, -1).Format("2006-01-02"), "rows": rows, "totals": totals,
		})
	}
	t := &export.Table{
		Title: "Patrol schedule compliance", Subtitle: site.Name + " · " + period(from, to, loc), Generated: siteNow(site),
		Filters: [][2]string{{"Site", site.Name}, {"Period", period(from, to, loc)}},
		Columns: []export.Column{
			{Name: "Date", Kind: export.Text, Width: 0.8}, {Name: "Route", Kind: export.Text, Width: 1.5},
			{Name: "Expected", Kind: export.Int}, {Name: "Started on time", Kind: export.Int}, {Name: "Late", Kind: export.Int},
			{Name: "Missed", Kind: export.Int}, {Name: "Pending", Kind: export.Int}, {Name: "Compliance %", Kind: export.Float},
		},
	}
	for _, r := range rows {
		var pct any
		if r.CompliancePct != nil {
			pct = *r.CompliancePct
		}
		t.Rows = append(t.Rows, []any{r.Date, r.RouteName, r.Expected, r.StartedOnTime, r.Late, r.Missed, r.Pending, pct})
	}
	return h.sendExport(c, format, fmt.Sprintf("patrol-compliance-%s", from.In(loc).Format("2006-01-02")), t)
}
