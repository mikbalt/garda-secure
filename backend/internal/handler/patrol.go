package handler

import (
	"encoding/json"

	"github.com/gofiber/fiber/v2"
	"github.com/securepatrol/backend/internal/model"
	"github.com/securepatrol/backend/internal/repository"
)

func marshalCheckpoints(cps []model.Checkpoint) (json.RawMessage, error) {
	if cps == nil {
		return json.RawMessage("[]"), nil
	}
	b, err := json.Marshal(cps)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(b), nil
}

// RegisterPatrol registers patrol route management endpoints.
func RegisterPatrol(r fiber.Router) {
	r.Get("/routes", listRoutes)
	r.Post("/routes", createRoute)
	r.Get("/routes/:id", getRoute)
	r.Post("/routes/:id/scan", recordScan)
	r.Get("/routes/:id/report", getRouteReport)
}

func listRoutes(c *fiber.Ctx) error {
	pool := poolRequired(c)
	if pool == nil {
		return nil
	}
	routes, err := repository.ListRoutes(c.Context(), pool, c.Query("site_id"))
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	if routes == nil {
		routes = []model.Route{}
	}
	return c.JSON(routes)
}

func getRoute(c *fiber.Ctx) error {
	pool := poolRequired(c)
	if pool == nil {
		return nil
	}
	route, err := repository.GetRoute(c.Context(), pool, c.Params("id"))
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "route not found"})
	}
	return c.JSON(route)
}

func createRoute(c *fiber.Ctx) error {
	pool := poolRequired(c)
	if pool == nil {
		return nil
	}

	var body struct {
		SiteID      string             `json:"site_id"`
		Name        string             `json:"name"`
		Checkpoints []model.Checkpoint `json:"checkpoints"`
		Active      bool               `json:"active"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}
	if body.SiteID == "" || body.Name == "" {
		return c.Status(400).JSON(fiber.Map{"error": "site_id and name are required"})
	}

	// Serialize checkpoints to JSONB.
	checkpointsJSON, err := marshalCheckpoints(body.Checkpoints)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid checkpoints"})
	}

	route, err := repository.CreateRoute(c.Context(), pool, model.Route{
		SiteID:      body.SiteID,
		Name:        body.Name,
		Checkpoints: checkpointsJSON,
		Active:      body.Active,
	})
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(201).JSON(route)
}

func recordScan(c *fiber.Ctx) error {
	pool := poolRequired(c)
	if pool == nil {
		return nil
	}

	var body struct {
		CheckpointID   string   `json:"checkpoint_id"`
		GuardID        string   `json:"guard_id"`
		Method         string   `json:"method"`
		GPSLat         *float64 `json:"gps_lat"`
		GPSLng         *float64 `json:"gps_lng"`
		PhotoObjectKey *string  `json:"photo_object_key"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}
	if body.CheckpointID == "" || body.GuardID == "" {
		return c.Status(400).JSON(fiber.Map{"error": "checkpoint_id and guard_id are required"})
	}
	if body.Method == "" {
		body.Method = "qr"
	}

	scan, err := repository.RecordScan(c.Context(), pool, model.CheckpointScan{
		RouteID:        c.Params("id"),
		CheckpointID:   body.CheckpointID,
		GuardID:        body.GuardID,
		Method:         body.Method,
		GPSLat:         body.GPSLat,
		GPSLng:         body.GPSLng,
		PhotoObjectKey: body.PhotoObjectKey,
	})
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(201).JSON(scan)
}

func getRouteReport(c *fiber.Ctx) error {
	pool := poolRequired(c)
	if pool == nil {
		return nil
	}
	date := c.Query("date")
	if date == "" {
		date = "today"
	}
	report, err := repository.GetRouteReport(c.Context(), pool, c.Params("id"), date)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(report)
}
