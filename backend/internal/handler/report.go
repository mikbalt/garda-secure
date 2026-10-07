package handler

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/securepatrol/backend/internal/repository"
)

// RegisterReports registers reporting endpoints.
func RegisterReports(r fiber.Router) {
	r.Get("/attendance/daily", dailyAttendanceReport)
	r.Get("/patrol/completion", patrolCompletionReport)
	r.Get("/attendance/monthly", monthlyAttendanceReport)
	r.Get("/guards/kpi", guardKPIReport)
}

// GET /reports/attendance/daily?site_id=&date=
func dailyAttendanceReport(c *fiber.Ctx) error {
	siteID := c.Query("site_id")
	if siteID == "" {
		return c.Status(400).JSON(fiber.Map{"error": "site_id is required"})
	}
	date := c.Query("date")
	if date == "" {
		date = time.Now().UTC().Format("2006-01-02")
	}

	pool := poolRequired(c)
	if pool == nil {
		return nil
	}
	rows, err := repository.DailyAttendanceSummary(c.Context(), pool, siteID, date)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	if rows == nil {
		rows = []repository.AttendanceSummaryRow{}
	}
	return c.JSON(fiber.Map{"date": date, "site_id": siteID, "records": rows})
}

// GET /reports/patrol/completion?site_id=&date=
func patrolCompletionReport(c *fiber.Ctx) error {
	siteID := c.Query("site_id")
	if siteID == "" {
		return c.Status(400).JSON(fiber.Map{"error": "site_id is required"})
	}
	date := c.Query("date")
	if date == "" {
		date = time.Now().UTC().Format("2006-01-02")
	}

	pool := poolRequired(c)
	if pool == nil {
		return nil
	}
	rows, err := repository.PatrolCompletionSummary(c.Context(), pool, siteID, date)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	if rows == nil {
		rows = []repository.PatrolCompletionRow{}
	}
	return c.JSON(fiber.Map{"date": date, "site_id": siteID, "routes": rows})
}

// GET /reports/attendance/monthly?site_id=&month=YYYY-MM
func monthlyAttendanceReport(c *fiber.Ctx) error {
	siteID := c.Query("site_id")
	if siteID == "" {
		return c.Status(400).JSON(fiber.Map{"error": "site_id is required"})
	}
	month := c.Query("month")
	if month == "" {
		month = time.Now().UTC().Format("2006-01")
	}

	pool := poolRequired(c)
	if pool == nil {
		return nil
	}
	rows, err := repository.MonthlyRecap(c.Context(), pool, siteID, month)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	if rows == nil {
		rows = []repository.MonthlyRecapRow{}
	}
	return c.JSON(fiber.Map{"month": month, "site_id": siteID, "guards": rows})
}

// GET /reports/guards/kpi?site_id=&month=YYYY-MM
func guardKPIReport(c *fiber.Ctx) error {
	siteID := c.Query("site_id")
	if siteID == "" {
		return c.Status(400).JSON(fiber.Map{"error": "site_id is required"})
	}
	month := c.Query("month")
	if month == "" {
		month = time.Now().UTC().Format("2006-01")
	}

	pool := poolRequired(c)
	if pool == nil {
		return nil
	}
	rows, err := repository.GuardKPIReport(c.Context(), pool, siteID, month)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	if rows == nil {
		rows = []repository.GuardKPIRow{}
	}
	return c.JSON(fiber.Map{"month": month, "site_id": siteID, "guards": rows})
}
