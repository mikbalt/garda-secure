package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/securepatrol/backend/internal/model"
	"github.com/securepatrol/backend/internal/repository"
)

// RegisterGuards registers guard management routes.
func RegisterGuards(r fiber.Router) {
	r.Get("/", listGuards)
	r.Get("/:id", getGuard)
	r.Post("/", createGuard)
	r.Put("/:id", updateGuard)
}

func listGuards(c *fiber.Ctx) error {
	pool := poolRequired(c)
	if pool == nil {
		return nil
	}
	guards, err := repository.ListGuards(c.Context(), pool, c.Query("site_id"))
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	if guards == nil {
		guards = []model.Guard{}
	}
	return c.JSON(guards)
}

func getGuard(c *fiber.Ctx) error {
	pool := poolRequired(c)
	if pool == nil {
		return nil
	}
	guard, err := repository.GetGuardByID(c.Context(), pool, c.Params("id"))
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "guard not found"})
	}
	return c.JSON(guard)
}

func createGuard(c *fiber.Ctx) error {
	pool := poolRequired(c)
	if pool == nil {
		return nil
	}

	var body model.Guard
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}
	if body.Name == "" || body.Role == "" {
		return c.Status(400).JSON(fiber.Map{"error": "name and role are required"})
	}
	if body.Status == "" {
		body.Status = "active"
	}

	guard, err := repository.CreateGuard(c.Context(), pool, body)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(201).JSON(guard)
}

func updateGuard(c *fiber.Ctx) error {
	pool := poolRequired(c)
	if pool == nil {
		return nil
	}

	var body struct {
		Name   string `json:"name"`
		Role   string `json:"role"`
		Status string `json:"status"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	fields := map[string]any{}
	if body.Name != "" {
		fields["name"] = body.Name
	}
	if body.Role != "" {
		fields["role"] = body.Role
	}
	if body.Status != "" {
		fields["status"] = body.Status
	}

	guard, err := repository.UpdateGuard(c.Context(), pool, c.Params("id"), fields)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "guard not found or update failed"})
	}
	return c.JSON(guard)
}
