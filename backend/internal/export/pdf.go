package export

import (
	"bytes"
	_ "embed" // fonts
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/go-pdf/fpdf"
)

// DejaVu Sans Condensed (license: fonts/LICENSE-DejaVu.txt) covers Latin
// Extended, so Indonesian names and free text render correctly.
var (
	//go:embed fonts/DejaVuSansCondensed.ttf
	fontRegular []byte
	//go:embed fonts/DejaVuSansCondensed-Bold.ttf
	fontBold []byte
)

const (
	family     = "dejavu"
	lineH      = 4.6 // mm per text line at 8.5 pt
	maxCellLen = 2000
	maxLines   = 30 // per table cell; longer text is cut with an ellipsis
)

// Doc is a simple PDF builder: headings, key/value blocks, paragraphs,
// tables with a repeated header row, and images.
type Doc struct {
	pdf       *fpdf.Fpdf
	generated time.Time
	title     string
}

// NewDoc starts an A4 document; landscape for wide tables.
func NewDoc(title string, landscape bool, generated time.Time) *Doc {
	orientation := "P"
	if landscape {
		orientation = "L"
	}
	p := fpdf.New(orientation, "mm", "A4", "")
	p.AddUTF8FontFromBytes(family, "", fontRegular)
	p.AddUTF8FontFromBytes(family, "B", fontBold)
	p.SetTitle(clean(title), true)
	p.SetCreator("SecurePatrol", true)
	p.SetProducer("SecurePatrol", true)
	if !generated.IsZero() {
		p.SetCreationDate(generated)
	}
	p.SetMargins(12, 12, 12)
	p.SetAutoPageBreak(true, 14)
	p.AliasNbPages("{nb}")
	d := &Doc{pdf: p, generated: generated, title: title}
	p.SetFooterFunc(func() {
		p.SetY(-10)
		p.SetFont(family, "", 7)
		p.SetTextColor(100, 116, 139)
		left := clean(title)
		if !generated.IsZero() {
			left += " - " + generated.Format("2006-01-02 15:04 MST")
		}
		p.CellFormat(0, 4, left, "", 0, "L", false, 0, "")
		p.SetX(-40)
		p.CellFormat(28, 4, fmt.Sprintf("%d / {nb}", p.PageNo()), "", 0, "R", false, 0, "")
		p.SetTextColor(0, 0, 0)
	})
	p.AddPage()
	return d
}

// Heading writes a bold heading (size in pt).
func (d *Doc) Heading(text string, size float64) {
	d.pdf.SetFont(family, "B", size)
	d.pdf.MultiCell(0, size*0.5, clean(text), "", "L", false)
	d.pdf.Ln(1.5)
}

// Paragraph writes wrapped text.
func (d *Doc) Paragraph(text string) {
	d.pdf.SetFont(family, "", 9)
	d.pdf.MultiCell(0, lineH, clean(text), "", "L", false)
	d.pdf.Ln(1)
}

// KeyValues writes "label: value" rows (e.g. the filters of a report).
func (d *Doc) KeyValues(kv [][2]string) {
	labelW := 42.0
	for _, p := range kv {
		d.pdf.SetFont(family, "B", 8.5)
		x, y := d.pdf.GetXY()
		d.pdf.CellFormat(labelW, lineH, clean(p[0]), "", 0, "L", false, 0, "")
		d.pdf.SetFont(family, "", 8.5)
		d.pdf.SetXY(x+labelW, y)
		d.pdf.MultiCell(0, lineH, clean(p[1]), "", "L", false)
	}
	d.pdf.Ln(2)
}

// Table writes a table; the header row repeats on every page.
func (d *Doc) Table(cols []Column, rows [][]any) {
	p := d.pdf
	pageW, pageH := p.GetPageSize()
	left, _, right, bottom := p.GetMargins()
	avail := pageW - left - right
	total := 0.0
	for _, c := range cols {
		total += weight(c)
	}
	widths := make([]float64, len(cols))
	for i, c := range cols {
		widths[i] = avail * weight(c) / total
	}
	header := func() {
		p.SetFont(family, "B", 8)
		p.SetFillColor(226, 232, 240)
		h := 0.0
		lines := make([][]string, len(cols))
		for i, c := range cols {
			lines[i] = p.SplitText(clean(c.Name), widths[i])
			if lh := float64(len(lines[i])) * lineH; lh > h {
				h = lh
			}
		}
		d.row(lines, widths, h, true, cols)
	}
	header()
	p.SetFont(family, "", 8)
	for _, r := range rows {
		lines := make([][]string, len(cols))
		h := lineH
		for i, c := range cols {
			var v any
			if i < len(r) {
				v = r[i]
			}
			txt := clean(formatCell(v, c.Kind, false)) // a PDF evaluates no formulas
			ls := p.SplitText(txt, widths[i])
			if len(ls) > maxLines {
				ls = append(ls[:maxLines-1], ls[maxLines-1]+" ...")
			}
			if len(ls) == 0 {
				ls = []string{""}
			}
			lines[i] = ls
			if lh := float64(len(ls)) * lineH; lh > h {
				h = lh
			}
		}
		if p.GetY()+h > pageH-bottom {
			p.AddPage()
			header()
			p.SetFont(family, "", 8)
		}
		d.row(lines, widths, h, false, cols)
	}
	p.Ln(2)
}

func (d *Doc) row(lines [][]string, widths []float64, h float64, fill bool, cols []Column) {
	p := d.pdf
	x0, y := p.GetXY()
	x := x0
	for i, ls := range lines {
		p.Rect(x, y, widths[i], h, map[bool]string{true: "FD", false: "D"}[fill])
		align := "L"
		if !fill && (cols[i].Kind == Int || cols[i].Kind == Float) {
			align = "R"
		}
		for k, l := range ls {
			p.SetXY(x, y+float64(k)*lineH)
			p.CellFormat(widths[i], lineH, l, "", 0, align, false, 0, "")
		}
		x += widths[i]
	}
	p.SetXY(x0, y+h)
}

// Image embeds a JPEG (see PrepareImage) scaled to fit maxW x maxH mm; it
// starts a new page when it does not fit on the current one.
func (d *Doc) Image(name string, jpegData []byte, wPx, hPx int, maxW, maxH float64) {
	p := d.pdf
	if wPx <= 0 || hPx <= 0 {
		return
	}
	w := maxW
	h := w * float64(hPx) / float64(wPx)
	if h > maxH {
		h = maxH
		w = h * float64(wPx) / float64(hPx)
	}
	_, pageH := p.GetPageSize()
	_, _, _, bottom := p.GetMargins()
	if p.GetY()+h > pageH-bottom {
		p.AddPage()
	}
	opts := fpdf.ImageOptions{ImageType: "JPG"}
	p.RegisterImageOptionsReader(name, opts, bytes.NewReader(jpegData))
	x, y := p.GetXY()
	p.ImageOptions(name, x, y, w, h, false, opts, 0, "")
	p.SetY(y + h + 2)
}

// Ln adds vertical space (mm).
func (d *Doc) Ln(h float64) { d.pdf.Ln(h) }

// Bytes renders the document.
func (d *Doc) Bytes() ([]byte, error) {
	var buf bytes.Buffer
	if err := d.pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// PDF renders a report table: title, filters summary and the table.
func PDF(t *Table) ([]byte, error) {
	d := NewDoc(t.Title, true, t.Generated)
	d.Heading(t.Title, 14)
	if t.Subtitle != "" {
		d.Paragraph(t.Subtitle)
	}
	kv := append([][2]string{}, t.Filters...)
	kv = append(kv, [2]string{"Rows", fmt.Sprint(len(t.Rows))})
	d.KeyValues(kv)
	d.Table(t.Columns, t.Rows)
	return d.Bytes()
}

func weight(c Column) float64 {
	if c.Width > 0 {
		return c.Width
	}
	switch c.Kind {
	case Int, Float:
		return 0.6
	case Time:
		return 1.1
	case Date:
		return 0.8
	}
	return 1
}

// clean makes text safe for the embedded font: no control characters other
// than newlines, runes outside the Basic Multilingual Plane (e.g. emoji)
// replaced, and very long values cut.
func clean(s string) string {
	if len(s) > maxCellLen*4 {
		s = s[:maxCellLen*4]
	}
	var b strings.Builder
	n := 0
	for _, r := range s {
		if n >= maxCellLen {
			b.WriteString(" ...")
			break
		}
		switch {
		case r == '\n':
			b.WriteRune(r)
		case r == '\t' || r == '\r':
			b.WriteRune(' ')
		case unicode.IsControl(r) || r == unicode.ReplacementChar:
			continue
		case r > 0xFFFF:
			b.WriteRune('?')
		default:
			b.WriteRune(r)
		}
		n++
	}
	return b.String()
}
