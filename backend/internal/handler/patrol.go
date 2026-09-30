package handler

import "github.com/gofiber/fiber/v2"

// RegisterPatrol registers patrol routes on the given router group.
func RegisterPatrol(r fiber.Router) {
	r.Get("/routes", notImplemented)
	r.Post("/routes", notImplemented)
	r.Post("/routes/:id/scan", notImplemented)
	r.Get("/routes/:id/report", notImplemented)
}
