package handler

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/securepatrol/backend/internal/export"
	"github.com/securepatrol/backend/internal/repository"
)

// pdfPhotoMaxPx bounds embedded photos (longer side) to keep PDFs small.
const pdfPhotoMaxPx = 1200

// GET /incidents/:id/pdf — printable incident detail: effective values (with
// the filed originals of amended fields), amendment history and the photos,
// scaled (FR-REP-03, F5).
func (h *Handler) incidentPDF(c *fiber.Ctx) error {
	ctx := c.Context()
	inc, err := repository.GetIncident(ctx, h.DB, c.Params("id"))
	if err != nil {
		return h.fail(c, err)
	}
	site, err := h.siteFor(c, inc.SiteID)
	if err != nil {
		return h.fail(c, err)
	}
	amendments, err := repository.ListAmendments(ctx, h.DB, inc.ID)
	if err != nil {
		return h.fail(c, err)
	}
	loc := site.Location()
	withOriginal := func(v string, orig *string) string {
		if orig == nil {
			return v
		}
		return fmt.Sprintf("%s  (amended; filed as: %s)", v, *orig)
	}
	d := export.NewDoc("Incident report", false, siteNow(site))
	d.Heading("Incident report — "+site.Name, 14)
	kv := [][2]string{
		{"Incident ID", inc.ID},
		{"Occurred", inc.OccurredAt.In(loc).Format("2006-01-02 15:04 MST")},
		{"Received", inc.ReceivedAt.In(loc).Format("2006-01-02 15:04 MST")},
		{"Category", withOriginal(inc.Type, inc.OriginalType)},
		{"Severity", withOriginal(inc.Severity, inc.OriginalSeverity)},
		{"Status", inc.Status},
		{"Guard", inc.GuardName},
		{"Location", withOriginal(deref(inc.Location), inc.OriginalLocation)},
	}
	if len(inc.Flags) > 0 {
		kv = append(kv, [2]string{"Flags", strings.Join(inc.Flags, ", ")})
	}
	d.KeyValues(kv)
	d.Heading("Description", 11)
	d.Paragraph(deref(inc.Description))
	if inc.OriginalDescription != nil {
		d.Heading("Description as filed", 10)
		d.Paragraph(*inc.OriginalDescription)
	}

	if len(amendments) > 0 {
		d.Heading("Amendments", 11)
		rows := make([][]any, 0, len(amendments))
		for _, a := range amendments {
			rows = append(rows, []any{a.CreatedAt.In(loc), a.AmendedBy, changesText(a.Changes), a.Reason})
		}
		d.Table([]export.Column{{Name: "When", Kind: export.Time}, {Name: "By", Kind: export.Text},
			{Name: "Changes", Kind: export.Text, Width: 2.5}, {Name: "Reason", Kind: export.Text, Width: 2}}, rows)
	}

	if len(inc.PhotoObjectKeys) > 0 {
		d.Heading(fmt.Sprintf("Photos (%d)", len(inc.PhotoObjectKeys)), 11)
		for i, key := range inc.PhotoObjectKeys {
			data, ok, err := h.Storage.Get(ctx, h.Cfg.MinioBucketPhotos, key, MaxPhotoBytes)
			if err != nil || !ok {
				if err != nil {
					h.Log.Warn().Err(err).Str("key", key).Msg("incident pdf: photo not readable")
				}
				d.Paragraph(fmt.Sprintf("Photo %d: not available (not uploaded yet)", i+1))
				continue
			}
			img, w, hh, err := export.PrepareImage(data, pdfPhotoMaxPx)
			if err != nil {
				d.Paragraph(fmt.Sprintf("Photo %d: unreadable image", i+1))
				continue
			}
			d.Paragraph(fmt.Sprintf("Photo %d", i+1))
			d.Image(fmt.Sprintf("photo%d", i), img, w, hh, 170, 115)
		}
	}
	out, err := d.Bytes()
	if err != nil {
		return h.fail(c, err)
	}
	c.Set(fiber.HeaderContentType, "application/pdf")
	c.Set(fiber.HeaderContentDisposition, `attachment; filename="incident-`+inc.ID+`.pdf"`)
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.Send(out)
}

// changesText renders an amendment's changes as "field: value; ...".
func changesText(raw json.RawMessage) string {
	var m map[string]string
	if json.Unmarshal(raw, &m) != nil {
		return string(raw)
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+": "+m[k])
	}
	return strings.Join(parts, "; ")
}
