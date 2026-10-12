package export

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"strings"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"
)

func sample() *Table {
	jkt, _ := time.LoadLocation("Asia/Jakarta")
	at := time.Date(2026, 10, 1, 21, 30, 0, 0, time.UTC).In(jkt) // 02 Oct 04:30 WIB
	long := strings.Repeat("Penjaga melihat orang mencurigakan di gerbang belakang. ", 80)
	return &Table{
		Title: "Incident log", Subtitle: "Gedung Ä · Oktober", Generated: at,
		Filters: [][2]string{{"Severity", "high"}},
		Columns: []Column{{Name: "Occurred", Kind: Time}, {Name: "Category", Kind: Text},
			{Name: "Photos", Kind: Int}, {Name: "Hours", Kind: Float}, {Name: "Description", Kind: Text, Width: 3}},
		Rows: [][]any{
			{at, "=HYPERLINK(\"x\")", 2, 7.25, "Pencurian kabel — ruang genset 🚨 é ñ"},
			{time.Time{}, nil, 0, 0.0, long},
		},
	}
}

func TestCSVIsFormulaSafe(t *testing.T) {
	out := string(CSV(sample()))
	if !strings.HasPrefix(out, "\xEF\xBB\xBF") {
		t.Fatal("missing BOM")
	}
	if !strings.Contains(out, `"'=HYPERLINK(""x"")"`) || !strings.Contains(out, "2026-10-02 04:30") || !strings.Contains(out, ",7.2,") && !strings.Contains(out, ",7.3,") {
		t.Fatalf("csv:\n%s", out[:200])
	}
}

func TestXLSXTypedCells(t *testing.T) {
	data, err := XLSX(sample())
	if err != nil {
		t.Fatal(err)
	}
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if v, _ := f.GetCellValue("Report", "A1"); v != "Occurred" {
		t.Fatalf("header A1 = %q", v)
	}
	if v, _ := f.GetCellValue("Report", "B2"); v != `'=HYPERLINK("x")` {
		t.Fatalf("B2 = %q (formula must stay text)", v)
	}
	if fm, _ := f.GetCellFormula("Report", "B2"); fm != "" {
		t.Fatalf("B2 has a formula %q", fm)
	}
	if v, _ := f.GetCellValue("Report", "A2"); v != "2026-10-02 04:30" {
		t.Fatalf("A2 = %q, want site-local time", v)
	}
	if typ, _ := f.GetCellType("Report", "C2"); typ != excelize.CellTypeNumber && typ != excelize.CellTypeUnset {
		t.Fatalf("C2 type %v", typ)
	}
	panes, err := f.GetPanes("Report")
	if err != nil || !panes.Freeze || panes.YSplit != 1 {
		t.Fatalf("header not frozen: %+v %v", panes, err)
	}
	if v, _ := f.GetCellValue("Info", "B2"); v != "Gedung Ä · Oktober" {
		t.Fatalf("info subtitle %q", v)
	}
}

func TestPDFWithUnicodeAndImage(t *testing.T) {
	data, err := PDF(sample())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(data, []byte("%PDF-")) || len(data) < 1000 {
		t.Fatalf("not a PDF (%d bytes)", len(data))
	}

	img := image.NewRGBA(image.Rect(0, 0, 3000, 1000))
	for x := 0; x < 3000; x++ {
		img.Set(x, x%1000, color.RGBA{200, 0, 0, 255})
	}
	var src bytes.Buffer
	if err := jpeg.Encode(&src, img, nil); err != nil {
		t.Fatal(err)
	}
	j, w, h, err := PrepareImage(src.Bytes(), 1200)
	if err != nil || w != 1200 || h != 400 {
		t.Fatalf("prepare: %v %dx%d", err, w, h)
	}
	d := NewDoc("Incident 1", false, time.Now())
	d.Heading("Detail", 12)
	d.KeyValues([][2]string{{"Lokasi", "Pos jaga — lantai 2"}})
	d.Image("p1", j, w, h, 120, 90)
	if out, err := d.Bytes(); err != nil || !bytes.HasPrefix(out, []byte("%PDF-")) {
		t.Fatalf("doc: %v", err)
	}
	if _, _, _, err := PrepareImage([]byte("not an image"), 100); err == nil {
		t.Fatal("garbage accepted")
	}
}

func TestOrient(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 4, 2))
	src.Set(0, 0, color.RGBA{255, 0, 0, 255})
	out := orient(src, 6) // 90 degrees clockwise: top-left goes to top-right
	if b := out.Bounds(); b.Dx() != 2 || b.Dy() != 4 {
		t.Fatalf("bounds %v", b)
	}
	if r, _, _, _ := out.At(1, 0).RGBA(); r == 0 {
		t.Fatal("pixel not rotated")
	}
}
