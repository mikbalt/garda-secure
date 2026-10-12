// Package export renders report tables as CSV, XLSX and PDF (audit F5).
//
// A Table has typed columns; cells hold string, int, float64, time.Time
// (already converted to the site's time zone; zero = empty) or nil. Text
// cells are protected against formula injection in every format.
package export

import (
	"bytes"
	"encoding/csv"
	"strconv"
	"strings"
	"time"
)

// Kind is the type of a column.
type Kind int

// Column kinds.
const (
	Text  Kind = iota
	Int        // whole numbers
	Float      // one decimal
	Time       // date and time (site local)
	Date       // date only (site local)
)

// Column describes one column.
type Column struct {
	Name  string
	Kind  Kind
	Width float64 // relative width in the PDF (0 = 1)
}

// Table is a report to export.
type Table struct {
	Title    string
	Subtitle string      // e.g. site and period
	Filters  [][2]string // label, value pairs shown above the table (PDF) / in a sheet header
	Columns  []Column
	Rows     [][]any
	// Generated is printed in the PDF footer (site-local time).
	Generated time.Time
}

// Formats.
const (
	FormatCSV  = "csv"
	FormatXLSX = "xlsx"
	FormatPDF  = "pdf"
)

// ContentType returns the MIME type of a format.
func ContentType(format string) string {
	switch format {
	case FormatXLSX:
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case FormatPDF:
		return "application/pdf"
	}
	return "text/csv; charset=utf-8"
}

// Render produces the table in a format (csv, xlsx or pdf).
func Render(t *Table, format string) ([]byte, error) {
	switch format {
	case FormatXLSX:
		return XLSX(t)
	case FormatPDF:
		return PDF(t)
	}
	return CSV(t), nil
}

// CSV writes UTF-8 CSV with a BOM so Excel opens it correctly.
func CSV(t *Table) []byte {
	var buf bytes.Buffer
	buf.Write([]byte{0xEF, 0xBB, 0xBF})
	w := csv.NewWriter(&buf)
	header := make([]string, len(t.Columns))
	for i, c := range t.Columns {
		header[i] = c.Name
	}
	_ = w.Write(header)
	for _, r := range t.Rows {
		rec := make([]string, len(t.Columns))
		for i := range t.Columns {
			if i < len(r) {
				rec[i] = FormatCell(r[i], t.Columns[i].Kind)
			}
		}
		_ = w.Write(rec)
	}
	w.Flush()
	return buf.Bytes()
}

// FormatCell renders a cell as spreadsheet text (CSV): text is made
// formula-safe.
func FormatCell(v any, k Kind) string { return formatCell(v, k, true) }

func formatCell(v any, k Kind, safe bool) string {
	text := func(s string) string {
		if safe {
			return SafeText(s)
		}
		return s
	}
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return text(x)
	case *string:
		if x == nil {
			return ""
		}
		return text(*x)
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case float64:
		return strconv.FormatFloat(x, 'f', 1, 64)
	case bool:
		if x {
			return "yes"
		}
		return "no"
	case time.Time:
		if x.IsZero() {
			return ""
		}
		if k == Date {
			return x.Format("2006-01-02")
		}
		return x.Format("2006-01-02 15:04")
	case *time.Time:
		if x == nil {
			return ""
		}
		return formatCell(*x, k, safe)
	}
	return ""
}

// SafeText stops spreadsheet apps from evaluating user-entered text as a
// formula (CSV/XLSX injection): a leading = + - @ tab or CR gets a quote.
func SafeText(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}
