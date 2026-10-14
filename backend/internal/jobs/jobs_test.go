package jobs

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/securepatrol/backend/internal/model"
	"github.com/securepatrol/backend/internal/repository"
)

func TestBackupDueUsesBackupTimeZone(t *testing.T) {
	jkt, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		t.Fatal(err)
	}
	// 19:30 UTC = 02:30 WIB the next day: due for BACKUP_HOUR=2 in Jakarta,
	// although the UTC (container) hour is 19.
	now := time.Date(2026, 10, 9, 19, 30, 0, 0, time.UTC).In(jkt)
	day, due := backupDue(now, 2)
	if !due {
		t.Fatal("backup not due at 02:30 WIB")
	}
	if want := time.Date(2026, 10, 10, 0, 0, 0, 0, jkt); !day.Equal(want) {
		t.Errorf("day start = %s, want %s", day, want)
	}
	// 18:30 UTC = 01:30 WIB: not yet.
	if _, due := backupDue(time.Date(2026, 10, 9, 18, 30, 0, 0, time.UTC).In(jkt), 2); due {
		t.Error("backup due before BACKUP_HOUR in Jakarta")
	}
}

func TestSchedulerDefaultsBackupLocation(t *testing.T) {
	s := &Scheduler{}
	if loc := s.backupLocation(); loc.String() != "Asia/Jakarta" {
		t.Errorf("default backup TZ = %s", loc)
	}
	if s.backupTimeout() != time.Hour {
		t.Errorf("default backup timeout = %s", s.backupTimeout())
	}
}

func TestRecapDueCatchesUp(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Jakarta")
	cases := []struct {
		at    time.Time
		month string
		due   bool
	}{
		{time.Date(2026, 11, 1, 6, 59, 0, 0, loc), "", false},
		{time.Date(2026, 11, 1, 7, 0, 0, 0, loc), "2026-10", true},
		{time.Date(2026, 11, 3, 2, 0, 0, 0, loc), "2026-10", true}, // worker was down on the 1st
		{time.Date(2026, 11, 5, 23, 0, 0, 0, loc), "2026-10", true},
		{time.Date(2026, 11, 6, 0, 0, 0, 0, loc), "", false},
		{time.Date(2027, 1, 2, 9, 0, 0, 0, loc), "2026-12", true}, // year boundary
		{time.Date(2026, 3, 1, 8, 0, 0, 0, loc), "2026-02", true},
	}
	for _, c := range cases {
		month, due := recapDue(c.at)
		if due != c.due || month != c.month {
			t.Errorf("%s: got %q %v, want %q %v", c.at, month, due, c.month, c.due)
		}
	}
}

func TestPgDumpTargetKeepsPasswordOutOfArgv(t *testing.T) {
	cases := []struct{ dsn, db, pw string }{
		{"postgres://sp:s3cr%2Ft@postgres:5432/securepatrol?sslmode=disable",
			"postgres://sp@postgres:5432/securepatrol?sslmode=disable", "s3cr/t"},
		{"postgresql://sp@db/x", "postgresql://sp@db/x", ""},
		{"postgres://db/x?user=sp&password=pw&sslmode=require", "postgres://db/x?sslmode=require&user=sp", "pw"},
		{"host=db user=sp password='a b' dbname=keycloak", "host=db user=sp dbname=keycloak", "a b"},
	}
	for _, c := range cases {
		db, pw, err := pgDumpTarget(c.dsn)
		if err != nil {
			t.Fatalf("%s: %v", c.dsn, err)
		}
		if db != c.db || pw != c.pw {
			t.Errorf("%s: got (%q, %q), want (%q, %q)", c.dsn, db, pw, c.db, c.pw)
		}
		if c.pw != "" && strings.Contains(db, c.pw) {
			t.Errorf("%s: password left in --dbname", c.dsn)
		}
	}
	if _, _, err := pgDumpTarget("postgres://sp:bad%zz@db/x"); err == nil {
		t.Error("invalid URL accepted")
	}
}

func TestKeycloakObjectKey(t *testing.T) {
	got := keycloakObjectKey("postgres/securepatrol-20261009-020000.dump")
	if got != "keycloak/keycloak-20261009-020000.dump" {
		t.Errorf("keycloak key = %q", got)
	}
}

func TestEachIsolatedContinuesAfterFailures(t *testing.T) {
	var ran []int
	err := eachIsolated(zerolog.Nop(), "flag absent", 4, func(i int) error {
		ran = append(ran, i)
		if i == 1 || i == 2 {
			return errors.New("site broken")
		}
		return nil
	})
	if len(ran) != 4 {
		t.Fatalf("ran %v, want all 4 items", ran)
	}
	if err == nil || !strings.Contains(err.Error(), "2/4 failed") {
		t.Errorf("err = %v", err)
	}
	if err := eachIsolated(zerolog.Nop(), "x", 3, func(int) error { return nil }); err != nil {
		t.Errorf("no failures: %v", err)
	}
}

// recapFixture: Budi works 20 days in 22 shifts; Sari has days still to come.
func recapFixture() ([]repository.MonthlyRecapRow, repository.MonthlyRecapTotals) {
	emp := "E1"
	rows := []repository.MonthlyRecapRow{
		{GuardName: "Budi", EmployeeID: &emp, RecapCounts: repository.RecapCounts{
			ScheduledDays: 20, DaysPresent: 17, DaysLate: 2, DaysAbsent: 1, EarlyOutDays: 1,
			ScheduledShifts: 22, Present: 19, Late: 2, Absent: 1, TotalHours: 160.5, PatrolSessions: 4, PatrolCompletePct: 95}},
		{GuardName: "Sari <S>", RecapCounts: repository.RecapCounts{
			ScheduledDays: 5, DaysPresent: 3, DaysUpcoming: 2, ScheduledShifts: 5, Present: 3, Pending: 2, TotalHours: 24}},
	}
	return rows, repository.RecapTotals(rows)
}

func TestRecapTextForWhatsApp(t *testing.T) {
	rows, totals := recapFixture()
	for _, c := range []struct {
		lang string
		want []string
	}{
		{"id", []string{"Rekap absensi 2026-10 — Gudang A (2 petugas)",
			"- Budi (E1): 20 hari: hadir 17, terlambat 2, absen 1, pulang awal 1; 160,5 jam; patroli 95%",
			"- Sari <S>: 5 hari: hadir 3, terlambat 0, absen 0, belum berjalan 2; 24,0 jam\n",
			"Total: 25 hari: hadir 20, terlambat 2, absen 1, belum berjalan 2, pulang awal 1; 184,5 jam; patroli 95%",
			"https://sp.example/reports?site=s1&month=2026-10"}},
		{"en", []string{"Attendance recap 2026-10 — Gudang A (2 guards)",
			"- Budi (E1): 20 days: present 17, late 2, absent 1, early out 1; 160.5 h; patrol 95%",
			"Total: 25 days: present 20, late 2, absent 1, upcoming 2, early out 1; 184.5 h; patrol 95%"}},
	} {
		site := &model.Site{ID: "s1", Name: "Gudang A", ConfigJSON: []byte(`{"language":"` + c.lang + `"}`)}
		txt := recapText(site, "2026-10", rows, totals, "https://sp.example")
		for _, want := range c.want {
			if !strings.Contains(txt, want) {
				t.Errorf("%s recap text lacks %q:\n%s", c.lang, want, txt)
			}
		}
		if strings.Contains(txt, "<td") || strings.Contains(txt, "shift") {
			t.Errorf("%s recap text: %s", c.lang, txt)
		}
	}
}

func TestRecapHTMLCountsDays(t *testing.T) {
	rows, totals := recapFixture()
	site := &model.Site{ID: "s1", Name: "Gudang A", ConfigJSON: []byte(`{"language":"en"}`)}
	h := recapHTML(site, "2026-10", rows, totals, "https://sp.example")
	for _, want := range []string{
		"<th>Days</th><th>Present</th><th>Late</th><th>Absent</th><th>Upcoming</th><th>Early-out days</th><th>Hours</th><th>Shifts</th>",
		"<td>Budi</td><td>E1</td><td>20</td><td>17</td><td>2</td><td>1</td><td>0</td><td>1</td><td>160.5</td><td>22</td><td>95.0</td>",
		"<td>Sari &lt;S&gt;</td>",
		`<tr style="font-weight:bold"><td>Total (2 guards)</td><td></td><td>25</td><td>20</td><td>2</td><td>1</td><td>2</td><td>1</td><td>184.5</td><td>27</td><td>95.0</td>`,
		"Counted per day", "Open in GARDA",
	} {
		if !strings.Contains(h, want) {
			t.Errorf("recap HTML lacks %q:\n%s", want, h)
		}
	}
	// Indonesian headers; no upcoming column when nothing is upcoming.
	site.ConfigJSON = []byte(`{"language":"id"}`)
	rows = rows[:1]
	h = recapHTML(site, "2026-10", rows, repository.RecapTotals(rows), "")
	for _, want := range []string{"<th>Petugas</th>", "<th>Hari</th><th>Hadir</th><th>Terlambat</th><th>Absen</th><th>Hari pulang awal</th>", "<td>160,5</td>", "Total (1 petugas)"} {
		if !strings.Contains(h, want) {
			t.Errorf("ID recap HTML lacks %q:\n%s", want, h)
		}
	}
	if strings.Contains(h, "Belum berjalan") || strings.Contains(h, "<a ") {
		t.Errorf("ID recap HTML: %s", h)
	}
}
