package handler

import (
	"github.com/gofiber/fiber/v2"

	"github.com/securepatrol/backend/internal/repository"
)

// activeSosLimit bounds the banner payload; X-Total-Count tells the rest.
const activeSosLimit = 100

// GET /sos/active — unacknowledged SOS alerts across every site the caller
// may see (realm, narrowed to allowed sites), for the banner shown on every
// page of the web app (WEB-3). Small and indexed; poll it.
func (h *Handler) activeSos(c *fiber.Ctx) error {
	ids, err := h.visibleSiteIDs(c)
	if err != nil {
		return h.fail(c, err)
	}
	rows, total, err := repository.ListActiveSos(c.Context(), h.DB, ids, activeSosLimit)
	if err != nil {
		return h.fail(c, err)
	}
	setPageHeaders(c, total, 0, len(rows))
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.JSON(rows)
}
