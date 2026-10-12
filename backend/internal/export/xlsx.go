package export

import (
	"fmt"
	"time"

	"github.com/xuri/excelize/v2"
)

// excelEpoch is day 0 of Excel's 1900 date system (with its leap-year bug).
var excelEpoch = time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC)

// excelSerial converts a site-local time to an Excel date serial of its wall
// clock, so the sheet shows site time whatever the reader's time zone.
func excelSerial(t time.Time) float64 {
	wall := time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), 0, time.UTC)
	return wall.Sub(excelEpoch).Hours() / 24
}

// XLSX writes the table as a workbook: sheet "Report" with a bold, frozen
// header row and typed cells (numbers, site-local date/times), and sheet
// "Info" with the title, filters and generation time.
func XLSX(t *Table) ([]byte, error) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	const sheet = "Report"
	if err := f.SetSheetName("Sheet1", sheet); err != nil {
		return nil, err
	}
	bold, err := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true},
		Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"E2E8F0"}}})
	if err != nil {
		return nil, err
	}
	dtFmt, dFmt, numFmt := "yyyy-mm-dd hh:mm", "yyyy-mm-dd", "0.0"
	dtStyle, err := f.NewStyle(&excelize.Style{CustomNumFmt: &dtFmt})
	if err != nil {
		return nil, err
	}
	dStyle, err := f.NewStyle(&excelize.Style{CustomNumFmt: &dFmt})
	if err != nil {
		return nil, err
	}
	fStyle, err := f.NewStyle(&excelize.Style{CustomNumFmt: &numFmt})
	if err != nil {
		return nil, err
	}

	sw, err := f.NewStreamWriter(sheet)
	if err != nil {
		return nil, err
	}
	for i, c := range t.Columns {
		w := 14.0
		switch c.Kind {
		case Text:
			w = 18 + 8*c.Width
		case Time:
			w = 17
		}
		if err := sw.SetColWidth(i+1, i+1, w); err != nil {
			return nil, err
		}
	}
	if err := sw.SetPanes(&excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"}); err != nil {
		return nil, err
	}
	header := make([]any, len(t.Columns))
	for i, c := range t.Columns {
		header[i] = excelize.Cell{StyleID: bold, Value: c.Name}
	}
	if err := sw.SetRow("A1", header); err != nil {
		return nil, err
	}
	for ri, r := range t.Rows {
		row := make([]any, len(t.Columns))
		for i, c := range t.Columns {
			var v any
			if i < len(r) {
				v = r[i]
			}
			row[i] = xlsxCell(v, c.Kind, dtStyle, dStyle, fStyle)
		}
		cell, err := excelize.CoordinatesToCellName(1, ri+2)
		if err != nil {
			return nil, err
		}
		if err := sw.SetRow(cell, row); err != nil {
			return nil, err
		}
	}
	if err := sw.Flush(); err != nil {
		return nil, err
	}

	if _, err := f.NewSheet("Info"); err != nil {
		return nil, err
	}
	info := [][2]string{{"Report", t.Title}}
	if t.Subtitle != "" {
		info = append(info, [2]string{"", t.Subtitle})
	}
	info = append(info, t.Filters...)
	if !t.Generated.IsZero() {
		info = append(info, [2]string{"Generated", t.Generated.Format("2006-01-02 15:04 MST")})
	}
	info = append(info, [2]string{"Rows", fmt.Sprint(len(t.Rows))})
	for i, kv := range info {
		if err := f.SetCellStr("Info", fmt.Sprintf("A%d", i+1), kv[0]); err != nil {
			return nil, err
		}
		if err := f.SetCellStr("Info", fmt.Sprintf("B%d", i+1), SafeText(kv[1])); err != nil {
			return nil, err
		}
	}
	_ = f.SetColWidth("Info", "A", "A", 18)
	_ = f.SetColWidth("Info", "B", "B", 60)
	f.SetActiveSheet(0)
	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func xlsxCell(v any, k Kind, dtStyle, dStyle, fStyle int) any {
	switch x := v.(type) {
	case nil:
		return nil
	case *string:
		if x == nil {
			return nil
		}
		return SafeText(*x)
	case string:
		return SafeText(x) // stored as a text cell; never a formula
	case int, int64:
		return x
	case float64:
		return excelize.Cell{StyleID: fStyle, Value: x}
	case bool:
		return FormatCell(x, k)
	case *time.Time:
		if x == nil {
			return nil
		}
		return xlsxCell(*x, k, dtStyle, dStyle, fStyle)
	case time.Time:
		if x.IsZero() {
			return nil
		}
		style := dtStyle
		if k == Date {
			style = dStyle
		}
		return excelize.Cell{StyleID: style, Value: excelSerial(x)}
	}
	return FormatCell(v, k)
}
