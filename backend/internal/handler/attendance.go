package handler

import "github.com/gofiber/fiber/v2"

// AttendanceRouter returns a sub-router with attendance endpoints (stub).
func AttendanceRouter() fiber.Router {
	r := fiber.New()
	r.Get("/", notImplemented)
	r.Post("/clock-in", notImplemented)
	r.Post("/clock-out", notImplemented)
	r.Get("/daily-summary", notImplemented)
	r.Get("/monthly-recap", notImplemented)
	return r
}
