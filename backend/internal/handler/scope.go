package handler

import (
	"slices"

	"github.com/gofiber/fiber/v2"

	"github.com/securepatrol/backend/internal/middleware"
	"github.com/securepatrol/backend/internal/model"
	"github.com/securepatrol/backend/internal/repository"
)

// visibleSites returns the sites of the caller's realm, limited to the
// caller's allowed sites when the token restricts them (F4, D-035).
func (h *Handler) visibleSites(c *fiber.Ctx) ([]model.Site, error) {
	p := middleware.GetPrincipal(c)
	sites, err := repository.ListSitesByRealm(c.Context(), h.DB, p.Realm)
	if err != nil {
		return nil, err
	}
	if p.SitesRestricted {
		sites = slices.DeleteFunc(sites, func(s model.Site) bool { return !p.CanSeeSite(s.ID) })
	}
	return sites, nil
}

// visibleSiteIDs is visibleSites as IDs.
func (h *Handler) visibleSiteIDs(c *fiber.Ctx) ([]string, error) {
	sites, err := h.visibleSites(c)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(sites))
	for i, s := range sites {
		ids[i] = s.ID
	}
	return ids, nil
}
