package jobs

import (
	"fmt"
	"html"
	"strconv"
	"strings"

	"github.com/securepatrol/backend/internal/model"
	"github.com/securepatrol/backend/internal/repository"
)

// recapLabels are the monthly recap texts in the site's language. The
// recap counts guard-days (FR-REP-05); shifts are shown for transparency.
type recapLabels struct {
	guard, employee, days, present, late, absent, upcoming, earlyOut, hours, shifts, patrol string
	total, guards, perDay, open, title                                                      string
}

var recapLabelsByLang = map[string]recapLabels{
	"en": {
		guard: "Guard", employee: "Employee ID", days: "Days", present: "Present", late: "Late", absent: "Absent",
		upcoming: "Upcoming", earlyOut: "Early-out days", hours: "Hours", shifts: "Shifts", patrol: "Patrol %",
		total: "Total", guards: "guards", open: "Open in GARDA", title: "Attendance recap",
		perDay: "Counted per day (site time zone; a night shift counts on the day it starts). A day is absent if any shift was missed, else late if any clock-in was late.",
	},
	"id": {
		guard: "Petugas", employee: "No. karyawan", days: "Hari", present: "Hadir", late: "Terlambat", absent: "Absen",
		upcoming: "Belum berjalan", earlyOut: "Hari pulang awal", hours: "Jam", shifts: "Shift", patrol: "Patroli %",
		total: "Total", guards: "petugas", open: "Buka di GARDA", title: "Rekap absensi",
		perDay: "Dihitung per hari (zona waktu lokasi; shift malam dihitung pada hari mulainya). Satu hari dihitung absen bila ada shift yang tidak dihadiri, terlambat bila ada absen masuk yang terlambat.",
	},
}

func labelsFor(site *model.Site) (recapLabels, bool) {
	if site.Config().Language == "en" {
		return recapLabelsByLang["en"], true
	}
	return recapLabelsByLang["id"], false
}

// dec1 formats a number with one decimal (decimal comma in Indonesian).
func dec1(h float64, en bool) string {
	s := strconv.FormatFloat(h, 'f', 1, 64)
	if !en {
		s = strings.Replace(s, ".", ",", 1)
	}
	return s
}

func recapHTML(site *model.Site, month string, rows []repository.MonthlyRecapRow, totals repository.MonthlyRecapTotals, appURL string) string {
	l, en := labelsFor(site)
	upcoming := totals.DaysUpcoming > 0
	var b strings.Builder
	fmt.Fprintf(&b, "<h2>%s — %s</h2>", html.EscapeString(site.Name), month)
	fmt.Fprintf(&b, "<p>%s</p>", html.EscapeString(l.perDay))
	b.WriteString(`<table border="1" cellpadding="4" cellspacing="0" style="border-collapse:collapse;font-family:sans-serif;font-size:13px">`)
	head := []string{l.guard, l.employee, l.days, l.present, l.late, l.absent}
	if upcoming {
		head = append(head, l.upcoming)
	}
	head = append(head, l.earlyOut, l.hours, l.shifts, l.patrol)
	b.WriteString("<tr>")
	for _, h := range head {
		fmt.Fprintf(&b, "<th>%s</th>", html.EscapeString(h))
	}
	b.WriteString("</tr>")
	row := func(name, emp string, c repository.RecapCounts, bold bool) {
		cells := []string{html.EscapeString(name), html.EscapeString(emp), strconv.Itoa(c.ScheduledDays),
			strconv.Itoa(c.DaysPresent), strconv.Itoa(c.DaysLate), strconv.Itoa(c.DaysAbsent)}
		if upcoming {
			cells = append(cells, strconv.Itoa(c.DaysUpcoming))
		}
		patrol := "-"
		if c.PatrolSessions > 0 {
			patrol = dec1(c.PatrolCompletePct, en)
		}
		cells = append(cells, strconv.Itoa(c.EarlyOutDays), dec1(c.TotalHours, en), strconv.Itoa(c.ScheduledShifts), patrol)
		if bold {
			b.WriteString(`<tr style="font-weight:bold">`)
		} else {
			b.WriteString("<tr>")
		}
		for _, v := range cells {
			fmt.Fprintf(&b, "<td>%s</td>", v)
		}
		b.WriteString("</tr>")
	}
	for _, r := range rows {
		emp := ""
		if r.EmployeeID != nil {
			emp = *r.EmployeeID
		}
		row(r.GuardName, emp, r.RecapCounts, false)
	}
	row(fmt.Sprintf("%s (%d %s)", l.total, totals.Guards, l.guards), "", totals.RecapCounts, true)
	b.WriteString("</table>")
	if appURL != "" {
		fmt.Fprintf(&b, `<p><a href="%s/reports?site=%s&amp;month=%s">%s</a></p>`,
			html.EscapeString(appURL), site.ID, month, html.EscapeString(l.open))
	}
	return b.String()
}

// recapLine is one plain-text line of day counts.
func recapLine(c repository.RecapCounts, en bool) string {
	var s string
	if en {
		s = fmt.Sprintf("%d days: present %d, late %d, absent %d", c.ScheduledDays, c.DaysPresent, c.DaysLate, c.DaysAbsent)
		if c.DaysUpcoming > 0 {
			s += fmt.Sprintf(", upcoming %d", c.DaysUpcoming)
		}
		if c.EarlyOutDays > 0 {
			s += fmt.Sprintf(", early out %d", c.EarlyOutDays)
		}
		s += "; " + dec1(c.TotalHours, en) + " h"
		if c.PatrolSessions > 0 {
			s += fmt.Sprintf("; patrol %.0f%%", c.PatrolCompletePct)
		}
		return s
	}
	s = fmt.Sprintf("%d hari: hadir %d, terlambat %d, absen %d", c.ScheduledDays, c.DaysPresent, c.DaysLate, c.DaysAbsent)
	if c.DaysUpcoming > 0 {
		s += fmt.Sprintf(", belum berjalan %d", c.DaysUpcoming)
	}
	if c.EarlyOutDays > 0 {
		s += fmt.Sprintf(", pulang awal %d", c.EarlyOutDays)
	}
	s += "; " + dec1(c.TotalHours, en) + " jam"
	if c.PatrolSessions > 0 {
		s += fmt.Sprintf("; patroli %.0f%%", c.PatrolCompletePct)
	}
	return s
}

// recapText is the plain-text recap for WhatsApp (which cannot render HTML):
// one line per guard plus the site total, in the site's language.
func recapText(site *model.Site, month string, rows []repository.MonthlyRecapRow, totals repository.MonthlyRecapTotals, appURL string) string {
	l, en := labelsFor(site)
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s — %s (%d %s)\n", l.title, month, site.Name, len(rows), l.guards)
	for _, r := range rows {
		name := r.GuardName
		if r.EmployeeID != nil && *r.EmployeeID != "" {
			name += " (" + *r.EmployeeID + ")"
		}
		fmt.Fprintf(&b, "- %s: %s\n", name, recapLine(r.RecapCounts, en))
	}
	fmt.Fprintf(&b, "%s: %s\n", l.total, recapLine(totals.RecapCounts, en))
	if appURL != "" {
		fmt.Fprintf(&b, "%s/reports?site=%s&month=%s", appURL, site.ID, month)
	}
	return strings.TrimSpace(b.String())
}
