package handler

import (
	"github.com/gofiber/fiber/v2"

	"github.com/securepatrol/backend/internal/model"
	"github.com/securepatrol/backend/internal/pii"
	"github.com/securepatrol/backend/internal/repository"
)

// Visitor ID card numbers are stored encrypted (BE-S9) and answered masked
// (3201********0001) everywhere except GET /visitors/:id for admin and
// supervisor (the only roles allowed on that route).

func (h *Handler) piiCipher() (*pii.Cipher, error) { return pii.For(h.Cfg.SettingsEncryptionKey) }

// sealIDNumber encrypts an ID number before it is stored.
func (h *Handler) sealIDNumber(v *string) (*string, error) {
	if v == nil || *v == "" {
		return v, nil
	}
	c, err := h.piiCipher()
	if err != nil {
		return nil, err
	}
	enc, err := c.Encrypt(*v)
	if err != nil {
		return nil, err
	}
	return &enc, nil
}

// visitorOut replaces the stored ID number with the plain (full) or masked
// value for a response.
func (h *Handler) visitorOut(v *model.VisitorLog, full bool) {
	if v == nil || v.IDNumber == nil || *v.IDNumber == "" {
		return
	}
	plain := "[unreadable]"
	if c, err := h.piiCipher(); err == nil {
		if p, err := c.Decrypt(*v.IDNumber); err == nil {
			plain = p
		} else {
			h.Log.Warn().Str("visitor_id", v.ID).Msg("visitor ID number cannot be decrypted (SETTINGS_ENCRYPTION_KEY changed?)")
		}
	}
	if !full && plain != "[unreadable]" {
		plain = pii.Mask(plain)
	}
	v.IDNumber = &plain
}

// GET /visitors/:id (admin, supervisor) — one visitor log with the full ID
// number.
func (h *Handler) getVisitor(c *fiber.Ctx) error {
	v, err := repository.GetVisitor(c.Context(), h.DB, c.Params("id"))
	if err != nil {
		return h.fail(c, err)
	}
	if _, err := h.siteFor(c, v.SiteID); err != nil {
		return h.fail(c, err)
	}
	h.visitorOut(v, true)
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.JSON(fiber.Map{"visitor": v, "photos": h.photos(c.Context(), deref(v.PhotoObjectKey))})
}
