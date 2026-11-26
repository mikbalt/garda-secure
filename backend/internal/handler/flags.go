package handler

import (
	"slices"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/securepatrol/backend/internal/middleware"
	"github.com/securepatrol/backend/internal/repository"
	"github.com/securepatrol/backend/internal/service"
)

const (
	maxNote       = 500
	maxFlagDays   = 92   // longest range of one GET /flags
	maxFlagOffset = 5000 // deeper pages: narrow the filter
)

// GET /flags?site_id=&guard_id=&from=&to=&flag=&reviewed=&limit=&offset=
// Plausibility flags (D-035) of the caller's visible sites, newest first, one
// row per flag. from/to are RFC 3339 times or UTC dates (to inclusive).
func (h *Handler) listFlags(c *fiber.Ctx) error {
	f := repository.FlagFilter{Flag: strings.TrimSpace(c.Query("flag"))}
	var err error
	if f.SiteIDs, err = h.visibleSiteIDs(c); err != nil {
		return h.fail(c, err)
	}
	if id := c.Query("site_id"); id != "" {
		site, err := h.siteFor(c, id)
		if err != nil {
			return h.fail(c, err)
		}
		f.SiteID = &site.ID
	}
	if id := c.Query("guard_id"); id != "" {
		if _, err := uuid.Parse(id); err != nil {
			return h.fail(c, service.BadRequest("invalid_guard_id", "guard_id must be a UUID"))
		}
		f.GuardID = &id
	}
	if c.Query("from") == "" || c.Query("to") == "" {
		return h.fail(c, service.BadRequest("range_required", "from and to are required"))
	}
	if f.From, err = auditTime(c.Query("from"), false); err != nil {
		return h.fail(c, err)
	}
	if f.To, err = auditTime(c.Query("to"), true); err != nil {
		return h.fail(c, err)
	}
	if f.To.Sub(*f.From) > maxFlagDays*24*time.Hour {
		return h.fail(c, service.BadRequest("range_too_long", "date range is limited to %d days", maxFlagDays))
	}
	switch c.Query("reviewed") {
	case "":
	case "true", "false":
		v := c.Query("reviewed") == "true"
		f.Reviewed = &v
	default:
		return h.fail(c, service.BadRequest("invalid_reviewed", "reviewed must be true or false"))
	}
	page, err := pageParams(c, 50, 200)
	if err != nil {
		return h.fail(c, err)
	}
	if page.Offset > maxFlagOffset {
		return h.fail(c, service.BadRequest("invalid_offset", "offset must be between 0 and %d", maxFlagOffset))
	}
	f.Limit, f.Offset = page.Limit+1, page.Offset // one extra row tells whether there is a next page
	rows, err := repository.ListFlags(c.Context(), h.DB, f)
	if err != nil {
		return h.fail(c, err)
	}
	more := len(rows) > page.Limit
	if more {
		rows = rows[:page.Limit]
	}
	return c.JSON(fiber.Map{"rows": rows, "limit": page.Limit, "offset": page.Offset, "has_more": more})
}

// POST /flags/review {kind, record_id, flag, note?}
// Marks one flag of a record as reviewed. The mark is stored apart from the
// record, which is not modified. Repeating it returns the first review with
// already_reviewed: true.
func (h *Handler) reviewFlag(c *fiber.Ctx) error {
	var b struct {
		Kind     string `json:"kind"`
		RecordID string `json:"record_id"`
		Flag     string `json:"flag"`
		Note     string `json:"note"`
	}
	if err := c.BodyParser(&b); err != nil {
		return h.fail(c, service.BadRequest("invalid_body", "invalid JSON body"))
	}
	if !slices.Contains(repository.FlagKinds, b.Kind) {
		return h.fail(c, service.BadRequest("invalid_flag_kind", "unknown record kind"))
	}
	if _, err := uuid.Parse(b.RecordID); err != nil {
		return h.fail(c, service.BadRequest("invalid_record_id", "record_id must be a UUID"))
	}
	note := strings.TrimSpace(b.Note)
	if b.Flag == "" || len(note) > maxNote {
		return h.fail(c, service.BadRequest("invalid_body", "flag is required and note may have at most %d characters", maxNote))
	}
	row, err := repository.GetFlag(c.Context(), h.DB, b.Kind, b.RecordID, b.Flag)
	if err != nil {
		return h.fail(c, err)
	}
	if _, err := h.siteFor(c, row.SiteID); err != nil { // realm and site scope
		return h.fail(c, err)
	}
	p := middleware.GetPrincipal(c)
	by := p.Username
	if by == "" {
		by = p.Subject
	}
	created, err := repository.ReviewFlag(c.Context(), h.DB, *row, by, note)
	if err != nil {
		return h.fail(c, err)
	}
	row, err = repository.GetFlag(c.Context(), h.DB, b.Kind, b.RecordID, b.Flag)
	if err != nil {
		return h.fail(c, err)
	}
	return c.JSON(fiber.Map{
		"kind": row.Kind, "record_id": row.RecordID, "flag": row.Flag, "site_id": row.SiteID, "site_name": row.SiteName,
		"guard_id": row.GuardID, "guard_name": row.GuardName, "event_at": row.EventAt,
		"reviewed_by": row.ReviewedBy, "reviewed_at": row.ReviewedAt, "note": row.Note,
		"already_reviewed": !created,
	})
}
