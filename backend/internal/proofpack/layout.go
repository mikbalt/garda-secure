package proofpack

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/securepatrol/backend/internal/export"
)

type item struct {
	kind  string // h, p, kv, table, banner, mono, bold, page
	text  string
	style string // banner: alert, warn, info
	size  float64
	kv    [][2]string
	cols  []export.Column
	rows  [][]any
}

// Layout is the pack as an ordered list of blocks: what Render draws and what
// the tests read, so the words in the PDF are the words checked.
type Layout struct {
	title string
	items []item
}

func (l *Layout) add(i item)               { l.items = append(l.items, i) }
func (l *Layout) h(text string, n float64) { l.add(item{kind: "h", text: text, size: n}) }
func (l *Layout) p(text string)            { l.add(item{kind: "p", text: text}) }
func (l *Layout) banner(text, style string) {
	l.add(item{kind: "banner", text: text, style: style})
}

func (i item) plain() string {
	switch i.kind {
	case "kv":
		var b strings.Builder
		for _, p := range i.kv {
			fmt.Fprintf(&b, "%s: %s\n", p[0], p[1])
		}
		return strings.TrimRight(b.String(), "\n")
	case "table":
		var b strings.Builder
		for _, c := range i.cols {
			b.WriteString(c.Name + " | ")
		}
		for _, r := range i.rows {
			b.WriteString("\n")
			for k, v := range r {
				b.WriteString(export.FormatCell(v, i.cols[k].Kind) + " | ")
			}
		}
		return b.String()
	case "banner":
		return i.text
	}
	return i.text
}

// pages splits the plain text at the page breaks.
func (l *Layout) pages() []string {
	out := []string{""}
	for _, i := range l.items {
		if i.kind == "page" {
			out = append(out, "")
			continue
		}
		out[len(out)-1] += i.plain() + "\n"
	}
	return out
}

// Text is the whole pack as plain text.
func (l *Layout) Text() string { return strings.Join(l.pages(), "\n") }

// Pages is the number of planned pages (a long table can add more).
func (l *Layout) Pages() int { return len(l.pages()) }

// FirstPageText and LastPageText are the text of the first and the last planned page.
func (l *Layout) FirstPageText() string { return l.pages()[0] }
func (l *Layout) LastPageText() string  { p := l.pages(); return p[len(p)-1] }

// HasNotice reports whether a banner contains s.
func (l *Layout) HasNotice(s string) bool {
	for _, i := range l.items {
		if i.kind == "banner" && strings.Contains(i.text, s) {
			return true
		}
	}
	return false
}

// ErrLastPage means the verification instructions no longer fit on one page.
var ErrLastPage = errors.New("proofpack: the verification instructions run over more than one page")

// Render draws the pack. The last section starts on its own page and must
// stay on it.
func (l *Layout) Render(generated time.Time) ([]byte, error) {
	d := export.NewDoc(l.title, false, generated)
	lastStart := 1
	for _, i := range l.items {
		switch i.kind {
		case "h":
			d.Heading(i.text, i.size)
		case "p":
			d.Paragraph(i.text)
		case "bold":
			d.Bold(i.text)
		case "mono":
			d.Mono(i.text)
		case "banner":
			d.Banner(i.text, i.style)
		case "kv":
			d.KeyValues(i.kv)
		case "table":
			d.Table(i.cols, i.rows)
			d.Ln(2)
		case "page":
			d.NewPage()
			lastStart = d.PageNo()
		}
	}
	if d.PageNo() != lastStart {
		return nil, ErrLastPage
	}
	return d.Bytes()
}

func itoa(n int) string { return strconv.Itoa(n) }

func dec1(v float64, lang string) string {
	s := strconv.FormatFloat(v, 'f', 1, 64)
	if lang != "en" {
		s = strings.Replace(s, ".", ",", 1)
	}
	return s
}

// Compose lays out the pack in lang ("en" or "id").
func Compose(d *Data, lang string) *Layout {
	w := wordsFor(lang)
	l := &Layout{title: w.title + " " + d.Month + " - " + d.SiteName}

	l.h(w.title, 16)
	l.h(d.SiteName+" - "+d.Month, 12)
	l.add(item{kind: "kv", kv: [][2]string{
		{w.site, d.SiteName},
		{w.period, d.FirstDay + " - " + d.LastDay},
		{w.zone, d.Timezone},
		{w.generated, d.Generated.Format("2006-01-02 15:04 MST")},
	}})

	c := d.Chain
	switch c.Status {
	case StatusSigned:
		l.banner(w.signed, "info")
	case StatusUnsignedTail:
		l.banner(fmt.Sprintf(w.unsignedTail, w.tailReason(c.VerifyStatus)), "warn")
	case StatusFailed:
		l.banner(fmt.Sprintf(w.failed, c.Detail), "alert")
	case StatusNoActivity:
		l.banner(w.noActivity, "info")
	default:
		l.banner(fmt.Sprintf(w.notProven, orDash(c.Detail, c.VerifyStatus)), "warn")
	}
	if d.MonthOpen {
		l.banner(fmt.Sprintf(w.monthOpen, d.Generated.Format("2006-01-02")), "info")
	}

	t := d.Totals
	l.h(w.summary, 12)
	l.h(w.attendance, 10)
	l.p(fmt.Sprintf(w.attLine, t.ScheduledDays, t.DaysPresent, t.DaysLate, t.DaysAbsent, dec1(t.TotalHours, lang)))
	l.p(fmt.Sprintf(w.attShifts, t.ScheduledShifts, t.Present, t.Late, t.Absent, t.Pending))
	l.h(w.patrol, 10)
	l.p(fmt.Sprintf(w.patLine, d.PatrolSessions, dec1(d.PatrolAvgPct, lang)))
	l.p(fmt.Sprintf(w.patIssues, d.SLABreaches, d.FlaggedScans))
	l.h(w.incidents, 10)
	if d.IncidentsTotal == 0 {
		l.p(w.incNone)
	} else {
		l.p(fmt.Sprintf(w.incLine, d.IncidentsTotal) + " " + fmt.Sprintf(w.incBy, severities(d.IncidentsBySeverity)))
	}
	l.h(w.flags, 10)
	l.p(w.flagsIntro)
	if len(d.Flags) == 0 {
		l.p(w.flagsNone)
	} else {
		rows := make([][]any, 0, len(d.Flags))
		for _, f := range d.Flags {
			rows = append(rows, []any{f.Flag, f.Count, f.Reviewed})
		}
		l.add(item{kind: "table", cols: []export.Column{{Name: w.flagCols[0], Width: 2}, {Name: w.flagCols[1], Kind: export.Int}, {Name: w.flagCols[2], Kind: export.Int}}, rows: rows})
		if d.FlagsTruncated {
			l.p(fmt.Sprintf(w.flagsMore, maxFlags, maxFlags))
		}
	}

	l.h(w.perDay, 12)
	l.p(w.perDayNote)
	dayRows := make([][]any, 0, len(d.Days))
	for _, x := range d.Days {
		dayRows = append(dayRows, []any{x.Date, x.Present, x.Late, x.Absent, x.PatrolSessions, x.Incidents})
	}
	dc := w.dayCols
	l.add(item{kind: "table", rows: dayRows, cols: []export.Column{{Name: dc[0], Kind: export.Text, Width: 1.4},
		{Name: dc[1], Kind: export.Int}, {Name: dc[2], Kind: export.Int}, {Name: dc[3], Kind: export.Int}, {Name: dc[4], Kind: export.Int}, {Name: dc[5], Kind: export.Int}}})

	l.h(w.perGuard, 12)
	gc := w.guardCols
	guardRows := make([][]any, 0, len(d.Guards)+1)
	for _, g := range d.Guards {
		guardRows = append(guardRows, []any{g.Name, g.ScheduledDays, g.DaysPresent, g.DaysLate, g.DaysAbsent, dec1(g.Hours, lang),
			itoa(g.PatrolSessions) + " / " + dec1(g.PatrolPct, lang), g.Incidents})
	}
	guardRows = append(guardRows, []any{w.total, t.ScheduledDays, t.DaysPresent, t.DaysLate, t.DaysAbsent, dec1(t.TotalHours, lang),
		itoa(d.PatrolSessions) + " / " + dec1(d.PatrolAvgPct, lang), d.IncidentsTotal})
	l.add(item{kind: "table", rows: guardRows, cols: []export.Column{{Name: gc[0], Width: 2.4}, {Name: gc[1], Kind: export.Int}, {Name: gc[2], Kind: export.Int},
		{Name: gc[3], Kind: export.Int}, {Name: gc[4], Kind: export.Int}, {Name: gc[5], Width: 0.8}, {Name: gc[6], Width: 1.6}, {Name: gc[7], Kind: export.Int}}})

	l.h(w.chain, 12)
	l.p(w.chainNote)
	kv := [][2]string{
		{w.chainEntries, itoa(c.Entries)},
		{w.chainTip, fmt.Sprintf("seq %d  %s", c.TipSeq, orDash(c.TipHash, "-"))},
		{w.chainChecked, c.VerifyStatus},
	}
	if c.Head != nil {
		kv = append(kv, [2]string{w.chainSeq, strconv.FormatInt(c.Head.Seq, 10)}, [2]string{w.chainHeadHash, c.Head.EntryHash},
			[2]string{w.chainSigned, c.Head.SignedAt}, [2]string{w.chainKey, c.Head.KeyID}, [2]string{w.chainSig, c.Head.Signature})
	} else {
		kv = append(kv, [2]string{w.chainHead, w.chainNoHead})
	}
	if c.Fingerprint != "" {
		kv = append(kv, [2]string{w.chainFp, c.Fingerprint})
	}
	l.add(item{kind: "kv", kv: kv})

	// The last page: how to check, and what a pass does not prove.
	l.add(item{kind: "page"})
	l.h(w.howTo, 13)
	l.p(w.howIntro)
	if c.Status == StatusUnsignedTail {
		l.banner(w.unsignedAgain, "warn")
	}
	l.p(w.step1)
	l.add(item{kind: "mono", text: c.BundleName})
	l.p(w.step2)
	l.p(w.step3)
	l.add(item{kind: "mono", text: fmt.Sprintf("garda-verify --pin %s --expect-from %s --expect-to %s %s", orDash(c.Fingerprint, "<fingerprint>"), c.ExpectFrom, c.ExpectTo, c.BundleName)})
	l.p(w.step4)
	for _, s := range w.codes {
		l.p("• " + s)
	}
	l.add(item{kind: "bold", text: w.limitsTitle})
	for _, s := range w.limits {
		l.p("• " + s)
	}
	return l
}

func orDash(s, alt string) string {
	if s == "" {
		return alt
	}
	return s
}

func severities(m map[string]int) string {
	var parts []string
	for _, k := range []string{"critical", "high", "medium", "low"} {
		if n := m[k]; n > 0 {
			parts = append(parts, fmt.Sprintf("%s %d", k, n))
		}
	}
	for k, n := range m {
		if k != "critical" && k != "high" && k != "medium" && k != "low" {
			parts = append(parts, fmt.Sprintf("%s %d", k, n))
		}
	}
	return strings.Join(parts, ", ")
}
