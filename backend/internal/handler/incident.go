package handler

import (
	"encoding/json"

	"github.com/gofiber/fiber/v2"
	"github.com/securepatrol/backend/internal/model"
	"github.com/securepatrol/backend/internal/repository"
)

// RegisterIncidents registers incident and SOS routes.
func RegisterIncidents(r fiber.Router) {
	r.Get("/", listIncidents)
	r.Post("/", createIncident)
	r.Get("/sos", listSosAlerts)
	r.Post("/sos", createSosAlert)
	r.Put("/sos/:id/acknowledge", acknowledgeSos)
	r.Get("/:id", getIncident)
	r.Put("/:id/status", updateIncidentStatus)
}

func listIncidents(c *fiber.Ctx) error {
	pool := poolRequired(c)
	if pool == nil {
		return nil
	}
	incidents, err := repository.ListIncidents(c.Context(), pool,
		c.Query("site_id"),
		c.Query("status"),
		c.Query("type"),
	)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	if incidents == nil {
		incidents = []model.Incident{}
	}
	return c.JSON(incidents)
}

func createIncident(c *fiber.Ctx) error {
	pool := poolRequired(c)
	if pool == nil {
		return nil
	}

	var body struct {
		SiteID          string          `json:"site_id"`
		GuardID         string          `json:"guard_id"`
		Type            string          `json:"type"`
		Severity        string          `json:"severity"`
		Description     *string         `json:"description"`
		Location        *string         `json:"location"`
		PhotoObjectKeys json.RawMessage `json:"photo_object_keys"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}
	if body.SiteID == "" || body.GuardID == "" || body.Type == "" {
		return c.Status(400).JSON(fiber.Map{"error": "site_id, guard_id, and type are required"})
	}
	if body.Severity == "" {
		body.Severity = "low"
	}
	if body.PhotoObjectKeys == nil {
		body.PhotoObjectKeys = json.RawMessage("[]")
	}

	inc := model.Incident{
		SiteID:          body.SiteID,
		GuardID:         body.GuardID,
		Type:            body.Type,
		Severity:        body.Severity,
		Description:     body.Description,
		Location:        body.Location,
		PhotoObjectKeys: body.PhotoObjectKeys,
		Status:          "open",
	}
	result, err := repository.CreateIncident(c.Context(), pool, inc)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(201).JSON(result)
}

func getIncident(c *fiber.Ctx) error {
	pool := poolRequired(c)
	if pool == nil {
		return nil
	}
	inc, err := repository.GetIncidentByID(c.Context(), pool, c.Params("id"))
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "incident not found"})
	}
	return c.JSON(inc)
}

func updateIncidentStatus(c *fiber.Ctx) error {
	pool := poolRequired(c)
	if pool == nil {
		return nil
	}
	var body struct {
		Status string `json:"status"`
	}
	if err := c.BodyParser(&body); err != nil || body.Status == "" {
		return c.Status(400).JSON(fiber.Map{"error": "status is required"})
	}
	if err := repository.UpdateIncidentStatus(c.Context(), pool, c.Params("id"), body.Status); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"status": body.Status})
}

func createSosAlert(c *fiber.Ctx) error {
	pool := poolRequired(c)
	if pool == nil {
		return nil
	}
	var body struct {
		GuardID string   `json:"guard_id"`
		SiteID  string   `json:"site_id"`
		GPSLat  *float64 `json:"gps_lat"`
		GPSLng  *float64 `json:"gps_lng"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}
	if body.GuardID == "" || body.SiteID == "" {
		return c.Status(400).JSON(fiber.Map{"error": "guard_id and site_id are required"})
	}

	alert, err := repository.CreateSosAlert(c.Context(), pool, model.SosAlert{
		GuardID: body.GuardID,
		SiteID:  body.SiteID,
		GPSLat:  body.GPSLat,
		GPSLng:  body.GPSLng,
	})
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(201).JSON(alert)
}

func listSosAlerts(c *fiber.Ctx) error {
	pool := poolRequired(c)
	if pool == nil {
		return nil
	}
	siteID := c.Query("site_id")
	if siteID == "" {
		return c.Status(400).JSON(fiber.Map{"error": "site_id is required"})
	}
	unackOnly := c.Query("unack_only") == "true"
	alerts, err := repository.ListSosAlerts(c.Context(), pool, siteID, unackOnly)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	if alerts == nil {
		alerts = []model.SosAlert{}
	}
	return c.JSON(alerts)
}

func acknowledgeSos(c *fiber.Ctx) error {
	pool := poolRequired(c)
	if pool == nil {
		return nil
	}
	var body struct {
		AcknowledgedBy string `json:"acknowledged_by"`
	}
	if err := c.BodyParser(&body); err != nil || body.AcknowledgedBy == "" {
		return c.Status(400).JSON(fiber.Map{"error": "acknowledged_by is required"})
	}
	if err := repository.AcknowledgeSosAlert(c.Context(), pool, c.Params("id"), body.AcknowledgedBy); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"acknowledged": true})
}

// notImplemented is retained for future stubs.
func notImplemented(c *fiber.Ctx) error {
	return c.Status(fiber.StatusNotImplemented).JSON(fiber.Map{"error": "not implemented"})
}
