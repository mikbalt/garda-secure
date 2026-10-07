package handler

import (
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"

	"github.com/securepatrol/backend/internal/model"
	"github.com/securepatrol/backend/internal/repository"
	"github.com/securepatrol/backend/internal/service"
)

// ── Templates ────────────────────────────────────────────────────────────────

type templateBody struct {
	SiteID     string  `json:"site_id"`
	GuardID    string  `json:"guard_id"`
	StartTime  string  `json:"start_time"`
	EndTime    string  `json:"end_time"`
	Weekdays   []int16 `json:"weekdays"`
	ValidFrom  string  `json:"valid_from"`
	ValidUntil *string `json:"valid_until"`
	Active     *bool   `json:"active"`
}

// checkGuardAtSite ensures a guard is assigned to the site of a shift.
func (h *Handler) checkGuardAtSite(c *fiber.Ctx, guardID, siteID string) error {
	g, err := repository.GetGuard(c.Context(), h.DB, guardID)
	if errors.Is(err, repository.ErrNotFound) || repository.IsInvalidInput(err) {
		return service.BadRequest("guard_not_found", "guard not found")
	}
	if err != nil {
		return err
	}
	if g.SiteID == nil || *g.SiteID != siteID {
		return service.BadRequest("guard_other_site", "guard is not assigned to this site")
	}
	return nil
}

func (h *Handler) listTemplates(c *fiber.Ctx) error {
	site, err := h.siteFor(c, c.Query("site_id"))
	if err != nil {
		return h.fail(c, err)
	}
	out, err := repository.ListShiftTemplates(c.Context(), h.DB, site.ID)
	if err != nil {
		return h.fail(c, err)
	}
	return c.JSON(out)
}

func (h *Handler) createTemplate(c *fiber.Ctx) error {
	var b templateBody
	if err := parse(c, &b); err != nil {
		return h.fail(c, err)
	}
	site, err := h.siteFor(c, b.SiteID)
	if err != nil {
		return h.fail(c, err)
	}
	t := model.ShiftTemplate{
		SiteID: site.ID, GuardID: b.GuardID, StartTime: b.StartTime, EndTime: b.EndTime,
		Weekdays: b.Weekdays, ValidFrom: b.ValidFrom, ValidUntil: b.ValidUntil, Active: b.Active == nil || *b.Active,
	}
	if err := service.ValidateTemplate(&t); err != nil {
		return h.fail(c, err)
	}
	if err := h.checkGuardAtSite(c, t.GuardID, site.ID); err != nil {
		return h.fail(c, err)
	}

	var out *model.ShiftTemplate
	var generated int
	err = pgx.BeginFunc(c.Context(), h.DB, func(tx pgx.Tx) error {
		var err error
		if out, err = repository.CreateShiftTemplate(c.Context(), tx, t); err != nil {
			return err
		}
		generated, err = service.GenerateTemplateShifts(c.Context(), tx, *out, site.Location())
		return err
	})
	if err != nil {
		return h.fail(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"template": out, "generated_shifts": generated})
}

func (h *Handler) templateFor(c *fiber.Ctx) (*model.ShiftTemplate, *model.Site, error) {
	t, err := repository.GetShiftTemplate(c.Context(), h.DB, c.Params("id"))
	if err != nil {
		return nil, nil, err
	}
	site, err := h.siteFor(c, t.SiteID)
	if err != nil {
		return nil, nil, err
	}
	return t, site, nil
}

func (h *Handler) updateTemplate(c *fiber.Ctx) error {
	t, site, err := h.templateFor(c)
	if err != nil {
		return h.fail(c, err)
	}
	var b templateBody
	if err := parse(c, &b); err != nil {
		return h.fail(c, err)
	}
	t.GuardID, t.StartTime, t.EndTime, t.Weekdays, t.ValidFrom, t.ValidUntil =
		b.GuardID, b.StartTime, b.EndTime, b.Weekdays, b.ValidFrom, b.ValidUntil
	if b.Active != nil {
		t.Active = *b.Active
	}
	if err := service.ValidateTemplate(t); err != nil {
		return h.fail(c, err)
	}
	if err := h.checkGuardAtSite(c, t.GuardID, site.ID); err != nil {
		return h.fail(c, err)
	}

	var out *model.ShiftTemplate
	var generated int
	err = pgx.BeginFunc(c.Context(), h.DB, func(tx pgx.Tx) error {
		var err error
		if out, err = repository.UpdateShiftTemplate(c.Context(), tx, *t); err != nil {
			return err
		}
		generated, err = service.RegenerateTemplateShifts(c.Context(), tx, *out, site.Location())
		return err
	})
	if err != nil {
		return h.fail(c, err)
	}
	return c.JSON(fiber.Map{"template": out, "generated_shifts": generated})
}

func (h *Handler) deleteTemplate(c *fiber.Ctx) error {
	t, _, err := h.templateFor(c)
	if err != nil {
		return h.fail(c, err)
	}
	err = pgx.BeginFunc(c.Context(), h.DB, func(tx pgx.Tx) error {
		if _, err := repository.DeleteFutureTemplateShifts(c.Context(), tx, t.ID, service.Now()); err != nil {
			return err
		}
		return repository.DeleteShiftTemplate(c.Context(), tx, t.ID)
	})
	if err != nil {
		return h.fail(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// ── Shifts ───────────────────────────────────────────────────────────────────

type shiftBody struct {
	SiteID    string    `json:"site_id"`
	GuardID   string    `json:"guard_id"`
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
}

func validateShiftTimes(start, end time.Time) error {
	if start.IsZero() || end.IsZero() {
		return service.BadRequest("times_required", "start_time and end_time are required (RFC 3339)")
	}
	if !end.After(start) {
		return service.BadRequest("invalid_times", "end_time must be after start_time")
	}
	if end.Sub(start) > 24*time.Hour {
		return service.BadRequest("invalid_times", "a shift cannot be longer than 24 hours")
	}
	return nil
}

// GET /shifts?site_id=&from=YYYY-MM-DD&to=YYYY-MM-DD&guard_id=
func (h *Handler) listShifts(c *fiber.Ctx) error {
	site, err := h.siteFor(c, c.Query("site_id"))
	if err != nil {
		return h.fail(c, err)
	}
	from, to, err := service.DateRange(c.Query("from"), c.Query("to"), site.Location(), 62)
	if err != nil {
		return h.fail(c, err)
	}
	shifts, err := repository.ListShifts(c.Context(), h.DB, site.ID, c.Query("guard_id"), from, to)
	if err != nil {
		return h.fail(c, err)
	}
	return c.JSON(shifts)
}

func (h *Handler) createShift(c *fiber.Ctx) error {
	var b shiftBody
	if err := parse(c, &b); err != nil {
		return h.fail(c, err)
	}
	site, err := h.siteFor(c, b.SiteID)
	if err != nil {
		return h.fail(c, err)
	}
	if err := validateShiftTimes(b.StartTime, b.EndTime); err != nil {
		return h.fail(c, err)
	}
	if err := h.checkGuardAtSite(c, b.GuardID, site.ID); err != nil {
		return h.fail(c, err)
	}
	s, err := repository.CreateShift(c.Context(), h.DB, b.GuardID, site.ID, b.StartTime.UTC(), b.EndTime.UTC())
	if err != nil {
		return h.fail(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(s)
}

func (h *Handler) shiftFor(c *fiber.Ctx, id string) (*model.Shift, error) {
	s, err := repository.GetShift(c.Context(), h.DB, id)
	if err != nil {
		return nil, err
	}
	if _, err := h.siteFor(c, s.SiteID); err != nil {
		return nil, err
	}
	return s, nil
}

func (h *Handler) updateShift(c *fiber.Ctx) error {
	s, err := h.shiftFor(c, c.Params("id"))
	if err != nil {
		return h.fail(c, err)
	}
	var b shiftBody
	if err := parse(c, &b); err != nil {
		return h.fail(c, err)
	}
	if b.GuardID == "" {
		b.GuardID = s.GuardID
	}
	if err := validateShiftTimes(b.StartTime, b.EndTime); err != nil {
		return h.fail(c, err)
	}
	if err := h.checkGuardAtSite(c, b.GuardID, s.SiteID); err != nil {
		return h.fail(c, err)
	}
	if _, err := repository.GetAttendanceByShift(c.Context(), h.DB, s.ID); err == nil {
		return h.fail(c, service.Conflict("shift_has_attendance", "shift already has attendance; use an attendance override instead"))
	}
	out, err := repository.UpdateShift(c.Context(), h.DB, s.ID, b.GuardID, b.StartTime.UTC(), b.EndTime.UTC())
	if err != nil {
		return h.fail(c, err)
	}
	return c.JSON(out)
}

func (h *Handler) deleteShift(c *fiber.Ctx) error {
	s, err := h.shiftFor(c, c.Params("id"))
	if err != nil {
		return h.fail(c, err)
	}
	if err := repository.DeleteShift(c.Context(), h.DB, s.ID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return h.fail(c, service.Conflict("shift_has_attendance", "shift already has attendance and cannot be deleted"))
		}
		return h.fail(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}
