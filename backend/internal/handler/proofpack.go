package handler

import (
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/securepatrol/backend/internal/proofpack"
	"github.com/securepatrol/backend/internal/service"
)

// packLang is ?lang= (en or id); the site's notification language when absent.
func packLang(c *fiber.Ctx, def string) (string, error) {
	switch l := c.Query("lang"); l {
	case "":
		if def == "en" {
			return "en", nil
		}
		return "id", nil
	case "en", "id":
		return l, nil
	}
	return "", service.BadRequest("invalid_lang", "lang must be en or id")
}

// GET /sites/:id/proof-pack?month=YYYY-MM&lang=en|id[&stored=true]
// The signed monthly proof pack (TAQ-20, D-074): a PDF with the month's
// summaries, the chain head, the newest signed head and the signer's key
// fingerprint, and on its last page the steps to check it with garda-verify.
// Same roles as the chain export (admin, supervisor). It is made now from the
// database; stored=true returns the copy the scheduled job kept instead
// (404 proof_pack_not_kept when there is none). X-Proofpack-Status says
// signed, unsigned_tail, failed, not_proven or no_activity.
func (h *Handler) proofPack(c *fiber.Ctx) error {
	site, err := h.siteFor(c, c.Params("id"))
	if err != nil {
		return h.fail(c, err)
	}
	month := c.Query("month")
	if month == "" {
		return h.fail(c, service.BadRequest("invalid_month", "month is required (YYYY-MM)"))
	}
	from, to, err := service.MonthRange(month, site.Location())
	if err != nil {
		return h.fail(c, err)
	}
	if !from.Before(service.Now()) {
		return h.fail(c, service.BadRequest("month_in_future", "that month has not started yet"))
	}
	lang, err := packLang(c, site.Config().Language)
	if err != nil {
		return h.fail(c, err)
	}
	name := fmt.Sprintf(`attachment; filename="proof-pack-%s-%s-%s.pdf"`, site.ID[:8], month, lang)

	if c.Query("stored") == "true" {
		for _, signed := range []bool{true, false} {
			data, ok, err := h.Storage.Get(c.Context(), h.Cfg.MinioBucketBackups, proofpack.Key(site.ID, month, signed), 32<<20)
			if err != nil {
				return h.fail(c, err)
			}
			if ok {
				status := "unsigned_tail"
				if signed {
					status = "signed"
				}
				return sendPack(c, data, status, name)
			}
		}
		return h.fail(c, &service.Error{Status: fiber.StatusNotFound, Code: "proof_pack_not_kept", Msg: "no proof pack was kept for that month"})
	}

	b, err := h.exportChain(c, "export", exportSlots, site, from, to, ChainLimits.ExportMaxEntries, ChainLimits.ExportTimeout)
	if err != nil {
		return h.fail(c, err)
	}
	now := time.Now()
	data, err := proofpack.Collect(c.Context(), h.DB, site, month, b, now)
	if err != nil {
		return h.fail(c, err)
	}
	pdf, err := proofpack.Compose(data, lang).Render(now.In(site.Location()))
	if err != nil {
		return h.fail(c, err)
	}
	return sendPack(c, pdf, string(data.Chain.Status), name)
}

func sendPack(c *fiber.Ctx, pdf []byte, status, disposition string) error {
	c.Set(fiber.HeaderContentType, "application/pdf")
	c.Set(fiber.HeaderContentDisposition, disposition)
	c.Set(fiber.HeaderCacheControl, "no-store")
	c.Set("X-Proofpack-Status", status)
	return c.Send(pdf)
}
