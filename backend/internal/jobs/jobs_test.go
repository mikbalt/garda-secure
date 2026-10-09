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

func TestRecapTextForWhatsApp(t *testing.T) {
	site := &model.Site{ID: "s1", Name: "Gudang A"}
	emp := "E1"
	rows := []repository.MonthlyRecapRow{{GuardName: "Budi", EmployeeID: &emp, ScheduledShifts: 20, Present: 18,
		Late: 1, Absent: 1, TotalHours: 160, PatrolCompletePct: 95}}
	txt := recapText(site, "2026-10", rows, "https://sp.example")
	for _, want := range []string{"Rekap absensi 2026-10", "Gudang A", "Budi", "hadir 18", "absen 1", "https://sp.example/reports?site=s1&month=2026-10"} {
		if !strings.Contains(txt, want) {
			t.Errorf("recap text lacks %q:\n%s", want, txt)
		}
	}
	if strings.Contains(txt, "<") {
		t.Errorf("recap text contains HTML: %s", txt)
	}
}
