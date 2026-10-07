package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/securepatrol/backend/internal/model"
	"github.com/securepatrol/backend/internal/repository"
)

// RegisterSites registers site management routes.
func RegisterSites(r fiber.Router) {
	r.Get("/", listSites)
	r.Get("/:id", getSite)
	r.Post("/", createSite)
}

func listSites(c *fiber.Ctx) error {
	pool := poolRequired(c)
	if pool == nil {
		return nil
	}
	sites, err := repository.ListSites(c.Context(), pool)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	if sites == nil {
		sites = []model.Site{}
	}
	return c.JSON(sites)
}

func getSite(c *fiber.Ctx) error {
	pool := poolRequired(c)
	if pool == nil {
		return nil
	}
	site, err := repository.GetSiteByID(c.Context(), pool, c.Params("id"))
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "site not found"})
	}
	return c.JSON(site)
}

func createSite(c *fiber.Ctx) error {
	pool := poolRequired(c)
	if pool == nil {
		return nil
	}

	var body model.Site
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}
	if body.Name == "" {
		return c.Status(400).JSON(fiber.Map{"error": "name is required"})
	}
	if body.Timezone == "" {
		body.Timezone = "Asia/Jakarta"
	}

	site, err := repository.CreateSite(c.Context(), pool, body)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(201).JSON(site)
}
