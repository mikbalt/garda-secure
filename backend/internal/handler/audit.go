package handler

import (
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/securepatrol/backend/internal/middleware"
	"github.com/securepatrol/backend/internal/repository"
	"github.com/securepatrol/backend/internal/service"
)

// GET /audit?from=&to=&actor=&resource_type=&resource_id=&limit=&offset=
// Audit entries of the caller's realm, newest first (NFR-05). from/to are
// RFC 3339 timestamps or UTC dates (YYYY-MM-DD, to inclusive).
func (h *Handler) listAudit(c *fiber.Ctx) error {
	f := repository.AuditFilter{
		Realm: middleware.GetPrincipal(c).Realm, Actor: c.Query("actor"),
		ResourceType: c.Query("resource_type"), ResourceID: c.Query("resource_id"),
	}
	var err error
	if f.From, err = auditTime(c.Query("from"), false); err != nil {
		return h.fail(c, err)
	}
	if f.To, err = auditTime(c.Query("to"), true); err != nil {
		return h.fail(c, err)
	}
	f.Limit, _ = strconv.Atoi(c.Query("limit", "100"))
	if f.Limit <= 0 || f.Limit > 500 {
		f.Limit = 100
	}
	f.Offset, _ = strconv.Atoi(c.Query("offset", "0"))
	if f.Offset < 0 || f.Offset > 100_000 {
		return h.fail(c, service.BadRequest("invalid_offset", "offset must be between 0 and 100000"))
	}
	want := f.Limit
	f.Limit++ // one extra row tells whether there is a next page
	rows, err := repository.ListAudit(c.Context(), h.DB, f)
	if err != nil {
		return h.fail(c, err)
	}
	more := len(rows) > want
	if more {
		rows = rows[:want]
	}
	return c.JSON(fiber.Map{"rows": rows, "limit": want, "offset": f.Offset, "has_more": more})
}

// auditTime parses an RFC 3339 time or a UTC date; an end date is exclusive
// of the following day.
func auditTime(v string, end bool) (*time.Time, error) {
	if v == "" {
		return nil, nil
	}
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return &t, nil
	}
	d, err := time.Parse("2006-01-02", v)
	if err != nil {
		return nil, service.BadRequest("invalid_time", "use RFC 3339 or YYYY-MM-DD")
	}
	if end {
		d = d.AddDate(0, 0, 1)
	}
	return &d, nil
}
