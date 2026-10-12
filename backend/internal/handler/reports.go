package handler

import (
	"fmt"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/securepatrol/backend/internal/repository"
	"github.com/securepatrol/backend/internal/service"
)

func fmtTime(t *time.Time, loc *time.Location) string {
	if t == nil {
		return ""
	}
	return t.In(loc).Format("2006-01-02 15:04")
}

func itoa(n int) string { return strconv.Itoa(n) }

func ftoa(f float64) string { return strconv.FormatFloat(f, 'f', 1, 64) }

func optPct(p *float64) string {
	if p == nil {
		return ""
	}
	return ftoa(*p)
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return ""
}

// GET /reports/attendance/daily?site_id=&date=&format=csv (FR-REP-01)
func (h *Handler) reportDailyAttendance(c *fiber.Ctx) error {
	site, err := h.siteFor(c, c.Query("site_id"))
	if err != nil {
		return h.fail(c, err)
	}
	loc := site.Location()
	from, to, err := service.DayRange(c.Query("date"), loc)
	if err != nil {
		return h.fail(c, err)
	}
	rows, err := repository.DailyAttendance(c.Context(), h.DB, site.ID, from, to)
	if err != nil {
		return h.fail(c, err)
	}
	date := from.In(loc).Format("2006-01-02")
	if !wantsCSV(c) {
		return c.JSON(fiber.Map{"site_id": site.ID, "date": date, "rows": rows})
	}
	out := make([][]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, []string{
			r.GuardName, deref(r.EmployeeID), fmtTime(&r.ShiftStart, loc), fmtTime(&r.ShiftEnd, loc),
			r.Status, fmtTime(r.ClockInAt, loc), fmtTime(r.ClockOutAt, loc),
			itoa(r.LateMinutes), itoa(r.EarlyOutMinutes), ftoa(r.HoursWorked), deref(r.OverrideReason),
		})
	}
	return sendCSV(c, fmt.Sprintf("attendance-%s.csv", date),
		[]string{"Guard", "Employee ID", "Shift start", "Shift end", "Status", "Clock in", "Clock out",
			"Late (min)", "Early out (min)", "Hours", "Override reason"}, out)
}

// GET /reports/attendance/monthly?site_id=&month=YYYY-MM&format=csv (FR-REP-05)
func (h *Handler) reportMonthlyRecap(c *fiber.Ctx) error {
	site, err := h.siteFor(c, c.Query("site_id"))
	if err != nil {
		return h.fail(c, err)
	}
	loc := site.Location()
	from, to, err := service.MonthRange(c.Query("month"), loc)
	if err != nil {
		return h.fail(c, err)
	}
	rows, err := repository.MonthlyRecap(c.Context(), h.DB, site.ID, from, to)
	if err != nil {
		return h.fail(c, err)
	}
	month := from.In(loc).Format("2006-01")
	if !wantsCSV(c) {
		compliance, err := repository.PatrolComplianceTotals(c.Context(), h.DB, site.ID, from, to)
		if err != nil {
			return h.fail(c, err)
		}
		return c.JSON(fiber.Map{"site_id": site.ID, "month": month, "rows": rows, "patrol_compliance": compliance})
	}
	out := make([][]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, []string{
			r.GuardName, deref(r.EmployeeID), itoa(r.ScheduledShifts), itoa(r.Present), itoa(r.Late),
			itoa(r.Absent), itoa(r.EarlyOut), itoa(r.Pending), ftoa(r.TotalHours),
			itoa(r.PatrolSessions), ftoa(r.PatrolCompletePct),
		})
	}
	return sendCSV(c, fmt.Sprintf("monthly-recap-%s.csv", month),
		[]string{"Guard", "Employee ID", "Shifts", "Present", "Late", "Absent", "Early out", "Pending",
			"Hours", "Patrols", "Patrol completion %"}, out)
}

// GET /reports/patrol/sessions?site_id=&from=&to=&format=csv (FR-REP-02)
func (h *Handler) reportPatrolSessions(c *fiber.Ctx) error {
	site, err := h.siteFor(c, c.Query("site_id"))
	if err != nil {
		return h.fail(c, err)
	}
	loc := site.Location()
	from, to, err := service.DateRange(c.Query("from"), c.Query("to"), loc, 92)
	if err != nil {
		return h.fail(c, err)
	}
	rows, err := repository.SessionSummaries(c.Context(), h.DB, site.ID, from, to)
	if err != nil {
		return h.fail(c, err)
	}
	if !wantsCSV(c) {
		return c.JSON(fiber.Map{"site_id": site.ID, "rows": rows})
	}
	out := make([][]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, []string{
			r.RouteName, r.GuardName, fmtTime(&r.StartedAt, loc), fmtTime(r.EndedAt, loc), r.Status,
			itoa(r.TotalCheckpoints), itoa(r.ScannedCount), itoa(r.TotalCheckpoints - r.ScannedCount),
			itoa(r.SLABreaches), ftoa(r.CompletionPct), fmtTime(r.DueAt, loc), yesNo(r.DueAt != nil && r.LateStart),
		})
	}
	return sendCSV(c, "patrol-sessions.csv",
		[]string{"Route", "Guard", "Started", "Ended", "Status", "Checkpoints", "Scanned", "Missed",
			"SLA breaches", "Completion %", "Scheduled for", "Late start"}, out)
}

// GET /reports/incidents?site_id=&from=&to=&type=&severity=&status=&format=csv (FR-REP-03)
func (h *Handler) reportIncidents(c *fiber.Ctx) error {
	site, f, err := h.incidentFilter(c)
	if err != nil {
		return h.fail(c, err)
	}
	rows, err := repository.ListIncidents(c.Context(), h.DB, f)
	if err != nil {
		return h.fail(c, err)
	}
	if !wantsCSV(c) {
		return c.JSON(rows)
	}
	loc := site.Location()
	names := map[string]string{}
	out := make([][]string, 0, len(rows))
	for _, r := range rows {
		name, ok := names[r.GuardID]
		if !ok {
			if g, err := repository.GetGuard(c.Context(), h.DB, r.GuardID); err == nil {
				name = g.Name
			}
			names[r.GuardID] = name
		}
		out = append(out, []string{
			fmtTime(&r.OccurredAt, loc), r.Type, r.Severity, r.Status, name, deref(r.Location),
			deref(r.Description), itoa(len(r.PhotoObjectKeys)), r.ID,
		})
	}
	return sendCSV(c, "incidents.csv",
		[]string{"Occurred", "Category", "Severity", "Status", "Guard", "Location", "Description", "Photos", "ID"}, out)
}

// GET /reports/guards/kpi?site_id=&from=&to=&format=csv (FR-REP-04)
func (h *Handler) reportGuardKPI(c *fiber.Ctx) error {
	site, err := h.siteFor(c, c.Query("site_id"))
	if err != nil {
		return h.fail(c, err)
	}
	from, to, err := service.DateRange(c.Query("from"), c.Query("to"), site.Location(), 366)
	if err != nil {
		return h.fail(c, err)
	}
	rows, err := repository.GuardKPIs(c.Context(), h.DB, site.ID, from, to)
	if err != nil {
		return h.fail(c, err)
	}
	if !wantsCSV(c) {
		return c.JSON(fiber.Map{"site_id": site.ID, "rows": rows})
	}
	out := make([][]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, []string{
			r.GuardName, ftoa(r.AttendanceRatePct), ftoa(r.OnTimeRatePct), ftoa(r.PatrolCompletePct),
			itoa(r.IncidentsFiled), itoa(r.SLABreaches), itoa(r.ScheduledPatrols), optPct(r.ScheduleCompliancePct),
		})
	}
	return sendCSV(c, "guard-kpi.csv",
		[]string{"Guard", "Attendance %", "On-time %", "Patrol completion %", "Incidents", "SLA breaches",
			"Scheduled patrols", "Schedule compliance %"}, out)
}

// GET /dashboard/summary?site_id=
func (h *Handler) dashboardSummary(c *fiber.Ctx) error {
	site, err := h.siteFor(c, c.Query("site_id"))
	if err != nil {
		return h.fail(c, err)
	}
	loc := site.Location()
	dayFrom, dayTo, err := service.DayRange("", loc)
	if err != nil {
		return h.fail(c, err)
	}
	weekFrom := dayFrom.In(loc).AddDate(0, 0, -6).UTC()
	sum, err := repository.GetDashboardSummary(c.Context(), h.DB, site.ID, dayFrom, dayTo, weekFrom, loc.String())
	if err != nil {
		return h.fail(c, err)
	}
	return c.JSON(sum)
}
