package handler

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/securepatrol/backend/internal/model"
	"github.com/securepatrol/backend/internal/repository"
)

// RegisterShifts registers shift management routes.
func RegisterShifts(r fiber.Router) {
	r.Get("/", listShifts)
	r.Post("/", createShift)
	r.Get("/:id", getShift)
	r.Put("/:id", updateShift)
	r.Delete("/:id", deleteShift)
}

func listShifts(c *fiber.Ctx) error {
	pool := poolRequired(c)
	if pool == nil {
		return nil
	}
	shifts, err := repository.ListShifts(c.Context(), pool,
		c.Query("site_id"), c.Query("guard_id"), c.Query("date"))
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	if shifts == nil {
		shifts = []model.Shift{}
	}
	return c.JSON(shifts)
}

func getShift(c *fiber.Ctx) error {
	pool := poolRequired(c)
	if pool == nil {
		return nil
	}
	shift, err := repository.GetShift(c.Context(), pool, c.Params("id"))
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "shift not found"})
	}
	return c.JSON(shift)
}

func createShift(c *fiber.Ctx) error {
	pool := poolRequired(c)
	if pool == nil {
		return nil
	}

	var body struct {
		GuardID    string  `json:"guard_id"`
		SiteID     string  `json:"site_id"`
		StartTime  string  `json:"start_time"`
		EndTime    string  `json:"end_time"`
		RepeatRule *string `json:"repeat_rule"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid body"})
	}
	if body.GuardID == "" || body.SiteID == "" || body.StartTime == "" || body.EndTime == "" {
		return c.Status(400).JSON(fiber.Map{"error": "guard_id, site_id, start_time, end_time are required"})
	}

	start, err := time.Parse(time.RFC3339, body.StartTime)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "start_time must be RFC3339"})
	}
	end, err := time.Parse(time.RFC3339, body.EndTime)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "end_time must be RFC3339"})
	}

	shift, err := repository.CreateShift(c.Context(), pool, body.GuardID, body.SiteID, start, end, body.RepeatRule)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(201).JSON(shift)
}

func updateShift(c *fiber.Ctx) error {
	pool := poolRequired(c)
	if pool == nil {
		return nil
	}

	var fields map[string]any
	if err := c.BodyParser(&fields); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid body"})
	}

	shift, err := repository.UpdateShift(c.Context(), pool, c.Params("id"), fields)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(shift)
}

func deleteShift(c *fiber.Ctx) error {
	pool := poolRequired(c)
	if pool == nil {
		return nil
	}
	if err := repository.DeleteShift(c.Context(), pool, c.Params("id")); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.SendStatus(204)
}
