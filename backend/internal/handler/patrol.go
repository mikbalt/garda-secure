package handler

import "github.com/gofiber/fiber/v2"

// PatrolRouter returns a sub-router with patrol endpoints (stub).
func PatrolRouter() fiber.Router {
	r := fiber.New()
	r.Get("/routes", notImplemented)
	r.Post("/routes", notImplemented)
	r.Post("/routes/:id/scan", notImplemented)
	r.Get("/routes/:id/report", notImplemented)
	return r
}
