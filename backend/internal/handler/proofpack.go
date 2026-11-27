package handler

import (
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/securepatrol/backend/internal/chainproof"
	"github.com/securepatrol/backend/internal/model"
	"github.com/securepatrol/backend/internal/proofpack"
	"github.com/securepatrol/backend/internal/repository"
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
		return h.storedPack(c, site, from, to, month, name)
	}

	// The chain and the figures are read inside one slot and one snapshot, under the export's limits.
	release, err := h.chainGate(c, "export", exportSlots)
	if err != nil {
		return h.fail(c, err)
	}
	defer release()
	// A fresh signed head first, outside the snapshot (as the bundle export does).
	signed := h.signNow(c.Context(), site.ID)
	tx, err := h.chainSnapshot(c, ChainLimits.ExportTimeout)
	if err != nil {
		return h.fail(c, err)
	}
	defer func() { _ = tx.Rollback(c.Context()) }()
	now := time.Now()
	fail := func(err error) error {
		err = exportErr(err)
		h.auditFailure(c, site, repository.ExportProofPack, from, to, err)
		return h.fail(c, err)
	}
	b, err := chainproof.Export(c.Context(), tx, site.ID, site.Timezone, from, to, now, ChainLimits.ExportMaxEntries)
	if err != nil {
		return fail(err)
	}
	b.NoteSigning(signed)
	data, err := proofpack.Collect(c.Context(), tx, site, month, b, now)
	if err != nil {
		return fail(err)
	}
	pdf, err := proofpack.Compose(data, lang).Render(now.In(site.Location()))
	if err != nil {
		return fail(err)
	}
	if err := h.auditExport(c, site, repository.ExportProofPack, from, to, int64(len(pdf)), repository.ExportOK, signed.Status, "status "+string(data.Chain.Status)); err != nil {
		return h.fail(c, err)
	}
	return sendPack(c, pdf, string(data.Chain.Status), name)
}

// storedPack returns the copy the scheduler kept, with the status it was kept with: the final
// (signed) copy, else the open one. Reads take the user's turn and a slot like the other chain reads.
func (h *Handler) storedPack(c *fiber.Ctx, site *model.Site, from, to time.Time, month, name string) error {
	siteID := site.ID
	release, err := h.chainGate(c, "stored", storedSlots)
	if err != nil {
		return h.fail(c, err)
	}
	defer release()
	for _, key := range []string{proofpack.KeyFinal(siteID, month), proofpack.KeyOpen(siteID, month)} {
		data, status, ok, err := h.Storage.GetPack(c.Context(), h.Cfg.MinioBucketBackups, key, 32<<20)
		if err != nil {
			return h.fail(c, err)
		}
		if !ok {
			continue
		}
		if status == "" || (status == "signed" && key != proofpack.KeyFinal(siteID, month)) {
			status = "not_proven" // never claim more than was recorded
		}
		if err := h.auditExport(c, site, repository.ExportProofPack, from, to, int64(len(data)), repository.ExportOK, "stored", "status "+status); err != nil {
			return h.fail(c, err)
		}
		return sendPack(c, data, status, name)
	}
	return h.fail(c, &service.Error{Status: fiber.StatusNotFound, Code: "proof_pack_not_kept", Msg: "no proof pack was kept for that month"})
}

// storedSlots bounds the stored-pack reads (up to 32 MB each) at the same time.
var storedSlots = make(chan struct{}, 2)

func sendPack(c *fiber.Ctx, pdf []byte, status, disposition string) error {
	c.Set(fiber.HeaderContentType, "application/pdf")
	c.Set(fiber.HeaderContentDisposition, disposition)
	c.Set(fiber.HeaderCacheControl, "no-store")
	c.Set("X-Proofpack-Status", status)
	return c.Send(pdf)
}
