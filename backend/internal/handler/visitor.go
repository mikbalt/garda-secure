package handler

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/securepatrol/backend/internal/model"
	"github.com/securepatrol/backend/internal/repository"
)

// RegisterVisitors registers visitor log routes.
func RegisterVisitors(r fiber.Router) {
	r.Get("/", listVisitors)
	r.Post("/", createVisitor)
	r.Post("/:id/exit", visitorExit)
}

func listVisitors(c *fiber.Ctx) error {
	pool := poolRequired(c)
	if pool == nil {
		return nil
	}
	logs, err := repository.ListVisitorLogs(c.Context(), pool, c.Query("site_id"), c.Query("date"))
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	if logs == nil {
		logs = []model.VisitorLog{}
	}
	return c.JSON(logs)
}

func createVisitor(c *fiber.Ctx) error {
	pool := poolRequired(c)
	if pool == nil {
		return nil
	}

	var v model.VisitorLog
	if err := c.BodyParser(&v); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid body"})
	}
	if v.VisitorName == "" || v.SiteID == "" || v.GuardID == "" {
		return c.Status(400).JSON(fiber.Map{"error": "visitor_name, site_id, guard_id are required"})
	}

	if v.EntryAt == nil {
		now := time.Now().UTC()
		v.EntryAt = &now
	}

	out, err := repository.CreateVisitorLog(c.Context(), pool, &v)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(201).JSON(out)
}

func visitorExit(c *fiber.Ctx) error {
	pool := poolRequired(c)
	if pool == nil {
		return nil
	}
	out, err := repository.UpdateVisitorExit(c.Context(), pool, c.Params("id"), time.Now().UTC())
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(out)
}
