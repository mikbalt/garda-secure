package handler

import "github.com/gofiber/fiber/v2"

// RegisterAttendance registers attendance routes on the given router group.
func RegisterAttendance(r fiber.Router) {
	r.Get("/", notImplemented)
	r.Post("/clock-in", notImplemented)
	r.Post("/clock-out", notImplemented)
	r.Get("/daily-summary", notImplemented)
	r.Get("/monthly-recap", notImplemented)
}
