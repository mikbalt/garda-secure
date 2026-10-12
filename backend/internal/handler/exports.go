package handler

import (
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/securepatrol/backend/internal/export"
	"github.com/securepatrol/backend/internal/model"
	"github.com/securepatrol/backend/internal/service"
)

// Export size limits: exports never truncate silently (audit BE-R6); a
// larger export answers 400 export_too_large and the user narrows the range.
const (
	exportMaxRows    = 50_000 // CSV / XLSX
	exportMaxRowsPDF = 2_000
)

// exportFormat returns the requested ?format= (csv, xlsx, or pdf where
// allowed); "" means JSON.
func exportFormat(c *fiber.Ctx, allowPDF bool) (string, error) {
	switch f := c.Query("format"); f {
	case "", "json":
		return "", nil
	case export.FormatCSV, export.FormatXLSX:
		return f, nil
	case export.FormatPDF:
		if allowPDF {
			return f, nil
		}
	}
	allowed := "csv or xlsx"
	if allowPDF {
		allowed = "csv, xlsx or pdf"
	}
	return "", service.BadRequest("invalid_format", "format must be %s", allowed)
}

// checkExportSize rejects exports above the format's row limit.
func checkExportSize(format string, rows int) error {
	limit := exportMaxRows
	if format == export.FormatPDF {
		limit = exportMaxRowsPDF
	}
	if rows > limit {
		return service.BadRequest("export_too_large",
			"the export has %d rows; the limit for %s is %d — narrow the date range or filters", rows, format, limit)
	}
	return nil
}

// sendExport renders t and sends it as a download named <base>.<format>.
func (h *Handler) sendExport(c *fiber.Ctx, format, base string, t *export.Table) error {
	if err := checkExportSize(format, len(t.Rows)); err != nil {
		return h.fail(c, err)
	}
	data, err := export.Render(t, format)
	if err != nil {
		return h.fail(c, err)
	}
	c.Set(fiber.HeaderContentType, export.ContentType(format))
	c.Set(fiber.HeaderContentDisposition, `attachment; filename="`+base+"."+format+`"`)
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.Send(data)
}

// siteNow is the generation time shown on exports (site-local).
func siteNow(site *model.Site) time.Time { return service.Now().In(site.Location()) }

// localTime converts an optional timestamp to the site's zone for export
// cells (nil stays empty).
func localTime(t *time.Time, loc *time.Location) any {
	if t == nil || t.IsZero() {
		return nil
	}
	return t.In(loc)
}

// period renders [from, to) as inclusive site-local dates.
func period(from, to time.Time, loc *time.Location) string {
	a, b := from.In(loc).Format("2006-01-02"), to.In(loc).Add(-time.Second).Format("2006-01-02")
	if a == b {
		return a
	}
	return fmt.Sprintf("%s – %s", a, b)
}
