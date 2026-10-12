package handler

import (
	"strconv"

	"github.com/gofiber/fiber/v2"

	"github.com/securepatrol/backend/internal/repository"
	"github.com/securepatrol/backend/internal/service"
)

// maxOffset bounds offset paging (deep pages: narrow the filter instead).
const maxOffset = 1_000_000

// pageParams reads ?limit=&offset= for lists that used to be silently
// capped (audit BE-R6). limit defaults to def and may not exceed max.
func pageParams(c *fiber.Ctx, def, max int) (repository.Page, error) {
	p := repository.Page{Limit: def}
	if v := c.Query("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > max {
			return p, service.BadRequest("invalid_limit", "limit must be between 1 and %d", max)
		}
		p.Limit = n
	}
	if v := c.Query("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 || n > maxOffset {
			return p, service.BadRequest("invalid_offset", "offset must be between 0 and %d", maxOffset)
		}
		p.Offset = n
	}
	return p, nil
}

// setPageHeaders exposes the total row count and whether the response holds
// fewer rows than match (X-Total-Count, X-Truncated: true), so UIs and
// exports can warn instead of silently showing a partial list.
func setPageHeaders(c *fiber.Ctx, total, offset, returned int) {
	c.Set("X-Total-Count", strconv.Itoa(total))
	if offset+returned < total {
		c.Set("X-Truncated", "true")
	}
}
