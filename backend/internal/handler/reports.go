package handler

import (
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/securepatrol/backend/internal/export"
	"github.com/securepatrol/backend/internal/repository"
	"github.com/securepatrol/backend/internal/service"
)

// Reports answer JSON, or a download with ?format=csv|xlsx (every report)
// and ?format=pdf (incident log) — F5.

// GET /reports/attendance/daily?site_id=&date=&format= (FR-REP-01)
func (h *Handler) reportDailyAttendance(c *fiber.Ctx) error {
	site, err := h.siteFor(c, c.Query("site_id"))
	if err != nil {
		return h.fail(c, err)
	}
	format, err := exportFormat(c, false)
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
	if format == "" {
		return c.JSON(fiber.Map{"site_id": site.ID, "date": date, "rows": rows})
	}
	t := &export.Table{
		Title: "Daily attendance", Subtitle: site.Name + " · " + date, Generated: siteNow(site),
		Filters: [][2]string{{"Site", site.Name}, {"Date", date}, {"Time zone", loc.String()}},
		Columns: []export.Column{
			{Name: "Guard", Kind: export.Text, Width: 1.4}, {Name: "Employee ID", Kind: export.Text},
			{Name: "Shift start", Kind: export.Time}, {Name: "Shift end", Kind: export.Time},
			{Name: "Status", Kind: export.Text, Width: 0.7}, {Name: "Clock in", Kind: export.Time},
			{Name: "Clock out", Kind: export.Time}, {Name: "Late (min)", Kind: export.Int},
			{Name: "Early out (min)", Kind: export.Int}, {Name: "Hours", Kind: export.Float},
			{Name: "Override reason", Kind: export.Text, Width: 1.5},
		},
	}
	for _, r := range rows {
		t.Rows = append(t.Rows, []any{
			r.GuardName, r.EmployeeID, r.ShiftStart.In(loc), r.ShiftEnd.In(loc), r.Status,
			localTime(r.ClockInAt, loc), localTime(r.ClockOutAt, loc), r.LateMinutes, r.EarlyOutMinutes,
			r.HoursWorked, r.OverrideReason,
		})
	}
	return h.sendExport(c, format, "attendance-"+date, t)
}

// GET /reports/attendance/monthly?site_id=&month=YYYY-MM&format= (FR-REP-05)
func (h *Handler) reportMonthlyRecap(c *fiber.Ctx) error {
	site, err := h.siteFor(c, c.Query("site_id"))
	if err != nil {
		return h.fail(c, err)
	}
	format, err := exportFormat(c, false)
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
	if format == "" {
		compliance, err := repository.PatrolComplianceTotals(c.Context(), h.DB, site.ID, from, to)
		if err != nil {
			return h.fail(c, err)
		}
		return c.JSON(fiber.Map{"site_id": site.ID, "month": month, "rows": rows, "patrol_compliance": compliance})
	}
	t := &export.Table{
		Title: "Monthly attendance recap", Subtitle: site.Name + " · " + month, Generated: siteNow(site),
		Filters: [][2]string{{"Site", site.Name}, {"Month", month}},
		Columns: []export.Column{
			{Name: "Guard", Kind: export.Text, Width: 1.4}, {Name: "Employee ID", Kind: export.Text},
			{Name: "Shifts", Kind: export.Int}, {Name: "Present", Kind: export.Int}, {Name: "Late", Kind: export.Int},
			{Name: "Absent", Kind: export.Int}, {Name: "Early out", Kind: export.Int}, {Name: "Pending", Kind: export.Int},
			{Name: "Hours", Kind: export.Float}, {Name: "Patrols", Kind: export.Int},
			{Name: "Patrol completion %", Kind: export.Float},
		},
	}
	for _, r := range rows {
		t.Rows = append(t.Rows, []any{
			r.GuardName, r.EmployeeID, r.ScheduledShifts, r.Present, r.Late, r.Absent, r.EarlyOut, r.Pending,
			r.TotalHours, r.PatrolSessions, r.PatrolCompletePct,
		})
	}
	return h.sendExport(c, format, "monthly-recap-"+month, t)
}

// GET /reports/patrol/sessions?site_id=&from=&to=&format= (FR-REP-02)
func (h *Handler) reportPatrolSessions(c *fiber.Ctx) error {
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
	rows, err := repository.SessionSummaries(c.Context(), h.DB, site.ID, from, to)
	if err != nil {
		return h.fail(c, err)
	}
	if format == "" {
		return c.JSON(fiber.Map{"site_id": site.ID, "rows": rows})
	}
	t := &export.Table{
		Title: "Patrol sessions", Subtitle: site.Name + " · " + period(from, to, loc), Generated: siteNow(site),
		Filters: [][2]string{{"Site", site.Name}, {"Period", period(from, to, loc)}},
		Columns: []export.Column{
			{Name: "Route", Kind: export.Text, Width: 1.3}, {Name: "Guard", Kind: export.Text, Width: 1.3},
			{Name: "Started", Kind: export.Time}, {Name: "Ended", Kind: export.Time},
			{Name: "Status", Kind: export.Text, Width: 0.8}, {Name: "Checkpoints", Kind: export.Int},
			{Name: "Scanned", Kind: export.Int}, {Name: "Missed", Kind: export.Int},
			{Name: "SLA breaches", Kind: export.Int}, {Name: "Flagged scans", Kind: export.Int},
			{Name: "Completion %", Kind: export.Float}, {Name: "Scheduled for", Kind: export.Time},
			{Name: "Late start", Kind: export.Text, Width: 0.6},
		},
	}
	for _, r := range rows {
		late := ""
		if r.DueAt != nil && r.LateStart {
			late = "yes"
		}
		t.Rows = append(t.Rows, []any{
			r.RouteName, r.GuardName, r.StartedAt.In(loc), localTime(r.EndedAt, loc), r.Status,
			r.TotalCheckpoints, r.ScannedCount, r.TotalCheckpoints - r.ScannedCount, r.SLABreaches,
			r.FlaggedScans, r.CompletionPct, localTime(r.DueAt, loc), late,
		})
	}
	return h.sendExport(c, format, "patrol-sessions", t)
}

// GET /reports/incidents?site_id=&from=&to=&type=&severity=&status=&limit=&offset=&format=
// (FR-REP-03). Values and filters are the effective (amended) ones (F7).
func (h *Handler) reportIncidents(c *fiber.Ctx) error {
	site, f, err := h.incidentFilter(c)
	if err != nil {
		return h.fail(c, err)
	}
	format, err := exportFormat(c, true)
	if err != nil {
		return h.fail(c, err)
	}
	if format == "" {
		return h.listIncidentsPage(c, f)
	}
	total, err := repository.CountIncidents(c.Context(), h.DB, f)
	if err != nil {
		return h.fail(c, err)
	}
	if err := checkExportSize(format, total); err != nil {
		return h.fail(c, err)
	}
	rows, err := repository.ListIncidents(c.Context(), h.DB, f, repository.Page{})
	if err != nil {
		return h.fail(c, err)
	}
	loc := site.Location()
	filters := [][2]string{{"Site", site.Name}}
	if f.From != nil && f.To != nil {
		filters = append(filters, [2]string{"Period", period(*f.From, *f.To, loc)})
	}
	for _, kv := range [][2]string{{"Category", f.Type}, {"Severity", f.Severity}, {"Status", f.Status}} {
		if kv[1] != "" {
			filters = append(filters, kv)
		}
	}
	t := &export.Table{
		Title: "Incident log", Subtitle: site.Name, Generated: siteNow(site), Filters: filters,
		Columns: []export.Column{
			{Name: "Occurred", Kind: export.Time}, {Name: "Category", Kind: export.Text},
			{Name: "Severity", Kind: export.Text, Width: 0.7}, {Name: "Status", Kind: export.Text, Width: 0.8},
			{Name: "Guard", Kind: export.Text}, {Name: "Location", Kind: export.Text},
			{Name: "Description", Kind: export.Text, Width: 2.6}, {Name: "Photos", Kind: export.Int},
			{Name: "Amended", Kind: export.Text, Width: 0.6}, {Name: "Original values", Kind: export.Text, Width: 1.3},
			{Name: "GPS", Kind: export.Text, Width: 1.1},
			{Name: "ID", Kind: export.Text, Width: 1.6},
		},
	}
	if format == export.FormatPDF {
		t.Columns = t.Columns[:len(t.Columns)-1] // the ID column does not fit
	}
	for _, r := range rows {
		amended := "no"
		if r.Amended {
			amended = "yes"
		}
		row := []any{
			r.OccurredAt.In(loc), r.Type, r.Severity, r.Status, r.GuardName, r.Location, r.Description,
			len(r.PhotoObjectKeys), amended, originalValues(r.OriginalType, r.OriginalSeverity,
				r.OriginalDescription, r.OriginalLocation), gpsText(r.GPSLat, r.GPSLng),
		}
		if format != export.FormatPDF {
			row = append(row, r.ID)
		}
		t.Rows = append(t.Rows, row)
	}
	return h.sendExport(c, format, "incidents", t)
}

// gpsText renders an optional position as "lat,lng".
func gpsText(lat, lng *float64) string {
	if lat == nil || lng == nil {
		return ""
	}
	return strconv.FormatFloat(*lat, 'f', 6, 64) + "," + strconv.FormatFloat(*lng, 'f', 6, 64)
}

// originalValues summarises the filed values an amendment replaced.
func originalValues(typ, severity, description, location *string) string {
	var parts []string
	for _, kv := range []struct {
		k string
		v *string
	}{{"category", typ}, {"severity", severity}, {"location", location}, {"description", description}} {
		if kv.v != nil {
			parts = append(parts, kv.k+": "+*kv.v)
		}
	}
	return strings.Join(parts, "; ")
}

// GET /reports/guards/kpi?site_id=&from=&to=&format= (FR-REP-04)
func (h *Handler) reportGuardKPI(c *fiber.Ctx) error {
	site, err := h.siteFor(c, c.Query("site_id"))
	if err != nil {
		return h.fail(c, err)
	}
	format, err := exportFormat(c, false)
	if err != nil {
		return h.fail(c, err)
	}
	loc := site.Location()
	from, to, err := service.DateRange(c.Query("from"), c.Query("to"), loc, 366)
	if err != nil {
		return h.fail(c, err)
	}
	rows, err := repository.GuardKPIs(c.Context(), h.DB, site.ID, from, to)
	if err != nil {
		return h.fail(c, err)
	}
	if format == "" {
		return c.JSON(fiber.Map{"site_id": site.ID, "rows": rows})
	}
	t := &export.Table{
		Title: "Guard KPIs", Subtitle: site.Name + " · " + period(from, to, loc), Generated: siteNow(site),
		Filters: [][2]string{{"Site", site.Name}, {"Period", period(from, to, loc)}},
		Columns: []export.Column{
			{Name: "Guard", Kind: export.Text, Width: 1.5}, {Name: "Attendance %", Kind: export.Float},
			{Name: "On-time %", Kind: export.Float}, {Name: "Patrol completion %", Kind: export.Float},
			{Name: "Incidents", Kind: export.Int}, {Name: "SLA breaches", Kind: export.Int},
			{Name: "Scheduled patrols", Kind: export.Int}, {Name: "Schedule compliance %", Kind: export.Float},
		},
	}
	for _, r := range rows {
		var compliance any
		if r.ScheduleCompliancePct != nil {
			compliance = *r.ScheduleCompliancePct
		}
		t.Rows = append(t.Rows, []any{
			r.GuardName, r.AttendanceRatePct, r.OnTimeRatePct, r.PatrolCompletePct, r.IncidentsFiled, r.SLABreaches,
			r.ScheduledPatrols, compliance,
		})
	}
	return h.sendExport(c, format, "guard-kpi", t)
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
