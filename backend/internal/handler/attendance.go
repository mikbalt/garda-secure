package handler

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/securepatrol/backend/internal/repository"
	"github.com/securepatrol/backend/internal/service"
)

// RegisterAttendance registers attendance routes on the given router group.
func RegisterAttendance(r fiber.Router) {
	r.Get("/", listAttendance)
	r.Post("/clock-in", clockIn)
	r.Post("/clock-out", clockOut)
	r.Get("/daily-summary", dailySummary)
	r.Get("/monthly-recap", monthlyRecap)
}

func getPool(c *fiber.Ctx) *pgxpool.Pool {
	v := c.Locals("db")
	if v == nil {
		return nil
	}
	p, _ := v.(*pgxpool.Pool)
	return p
}

// poolRequired returns the pool or sends a 503 and returns nil.
// Callers must check: if pool == nil { return nil }
func poolRequired(c *fiber.Ctx) *pgxpool.Pool {
	p := getPool(c)
	if p == nil {
		_ = c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "database not available"})
	}
	return p
}

// GET /attendance?site_id=&date=&status=
func listAttendance(c *fiber.Ctx) error {
	siteID := c.Query("site_id")
	if siteID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "site_id is required"})
	}
	date := c.Query("date")
	if date == "" {
		date = time.Now().UTC().Format("2006-01-02")
	}
	status := c.Query("status")

	pool := poolRequired(c)
	if pool == nil {
		return nil
	}
	var records interface{}
	var err error
	if status != "" {
		records, err = repository.ListByDateStatus(c.Context(), pool, siteID, date, status)
	} else {
		records, err = repository.ListByDate(c.Context(), pool, siteID, date)
	}
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(records)
}

// POST /attendance/clock-in
func clockIn(c *fiber.Ctx) error {
	pool := poolRequired(c)
	if pool == nil {
		return nil
	}

	var body struct {
		GuardID         string   `json:"guard_id"`
		SiteID          string   `json:"site_id"`
		GPSLat          *float64 `json:"gps_lat"`
		GPSLng          *float64 `json:"gps_lng"`
		SelfieObjectKey *string  `json:"selfie_object_key"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON body"})
	}
	if body.GuardID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "guard_id is required"})
	}
	if body.SiteID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "site_id is required"})
	}

	rec, err := service.ClockIn(c.Context(), pool, service.ClockInRequest{
		GuardID:         body.GuardID,
		SiteID:          body.SiteID,
		GPSLat:          body.GPSLat,
		GPSLng:          body.GPSLng,
		SelfieObjectKey: body.SelfieObjectKey,
	})
	if err != nil {
		msg := err.Error()
		switch msg {
		case "guard not found: " + body.GuardID:
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": msg})
		case "guard already clocked in today":
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": msg})
		default:
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": msg})
		}
	}
	return c.Status(fiber.StatusCreated).JSON(rec)
}

// POST /attendance/clock-out
func clockOut(c *fiber.Ctx) error {
	pool := poolRequired(c)
	if pool == nil {
		return nil
	}

	var body struct {
		AttendanceID string   `json:"attendance_id"`
		GPSLat       *float64 `json:"gps_lat"`
		GPSLng       *float64 `json:"gps_lng"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON body"})
	}
	if body.AttendanceID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "attendance_id is required"})
	}

	rec, err := service.ClockOut(c.Context(), pool, service.ClockOutRequest{
		AttendanceID: body.AttendanceID,
		GPSLat:       body.GPSLat,
		GPSLng:       body.GPSLng,
	})
	if err != nil {
		msg := err.Error()
		switch msg {
		case "attendance record not found: " + body.AttendanceID:
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": msg})
		case "guard has not clocked in yet", "guard already clocked out":
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": msg})
		default:
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": msg})
		}
	}
	return c.JSON(rec)
}

// GET /attendance/daily-summary?site_id=&date=
func dailySummary(c *fiber.Ctx) error {
	pool := poolRequired(c)
	if pool == nil {
		return nil
	}

	siteID := c.Query("site_id")
	if siteID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "site_id is required"})
	}
	date := c.Query("date")
	if date == "" {
		date = time.Now().UTC().Format("2006-01-02")
	}

	summary, err := repository.DailySummary(c.Context(), pool, siteID, date)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(summary)
}

// GET /attendance/monthly-recap?site_id=&month=
func monthlyRecap(c *fiber.Ctx) error {
	pool := poolRequired(c)
	if pool == nil {
		return nil
	}

	siteID := c.Query("site_id")
	if siteID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "site_id is required"})
	}
	month := c.Query("month")
	if month == "" {
		month = time.Now().UTC().Format("2006-01")
	}

	recap, err := repository.MonthlyRecap(c.Context(), pool, siteID, month)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(recap)
}
