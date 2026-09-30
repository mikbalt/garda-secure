package handler

import "github.com/gofiber/fiber/v2"

// IncidentRouter returns a sub-router with incident endpoints (stub).
func IncidentRouter() fiber.Router {
	r := fiber.New()
	r.Get("/", notImplemented)
	r.Post("/", notImplemented)
	r.Get("/:id", notImplemented)
	r.Post("/sos", notImplemented)
	r.Post("/defects", notImplemented)
	r.Post("/visitors", notImplemented)
	return r
}

// notImplemented is a shared handler returning HTTP 501.
func notImplemented(c *fiber.Ctx) error {
	return c.Status(fiber.StatusNotImplemented).JSON(fiber.Map{
		"error": "not implemented",
	})
}
