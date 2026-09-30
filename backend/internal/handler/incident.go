package handler

import "github.com/gofiber/fiber/v2"

// RegisterIncidents registers incident routes on the given router group.
func RegisterIncidents(r fiber.Router) {
	r.Get("/", notImplemented)
	r.Post("/", notImplemented)
	r.Get("/:id", notImplemented)
	r.Post("/sos", notImplemented)
	r.Post("/defects", notImplemented)
	r.Post("/visitors", notImplemented)
}

// notImplemented is a shared handler returning HTTP 501.
func notImplemented(c *fiber.Ctx) error {
	return c.Status(fiber.StatusNotImplemented).JSON(fiber.Map{
		"error": "not implemented",
	})
}
