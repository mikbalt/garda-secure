package proofpack

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/securepatrol/backend/internal/repository"
)

const (
	tipHash  = "aa11aa11aa11aa11aa11aa11aa11aa11aa11aa11aa11aa11aa11aa11aa11aa11"
	headHash = "bb22bb22bb22bb22bb22bb22bb22bb22bb22bb22bb22bb22bb22bb22bb22bb22"
	headSig  = "cc33cc33cc33cc33cc33cc33cc33cc33cc33cc33cc33cc33cc33cc33cc33cc33cc33cc33cc33cc33cc33cc33cc33cc33cc33cc33cc33cc33cc33cc33cc33cc33cc33cc33"
	keyID    = "dd44dd44dd44dd44dd44dd44dd44dd44dd44dd44dd44dd44dd44dd44dd44dd44"
)

func sample(status Status) *Data {
	d := &Data{
		SiteID: "5f1c2e3a-0000-4000-8000-000000000001", SiteName: "Gudang Utama", Timezone: "Asia/Jakarta", Month: "2026-10",
		FirstDay: "2026-10-01", LastDay: "2026-10-31", Generated: time.Date(2026, 11, 1, 8, 0, 0, 0, time.UTC),
		Days:           []Day{{Date: "2026-10-01", Present: 2, Late: 1, Absent: 0, PatrolSessions: 3, Incidents: 1}, {Date: "2026-10-02"}},
		Guards:         []GuardRow{{Name: "Budi Santoso", ScheduledDays: 20, DaysPresent: 18, DaysLate: 1, DaysAbsent: 1, Hours: 160.5, PatrolSessions: 40, PatrolPct: 97.5, Incidents: 2}},
		Totals:         repository.MonthlyRecapTotals{Guards: 1, RecapCounts: repository.RecapCounts{ScheduledDays: 20, DaysPresent: 18, DaysLate: 1, DaysAbsent: 1, ScheduledShifts: 20, TotalHours: 160.5}},
		PatrolSessions: 40, PatrolAvgPct: 97.5, SLABreaches: 2, FlaggedScans: 3,
		IncidentsTotal: 2, IncidentsBySeverity: map[string]int{"high": 1, "low": 1},
		Flags: []FlagCount{{Flag: "late_sync", Count: 4, Reviewed: 1}},
		Chain: Chain{Status: status, Entries: 120, TipSeq: 130, TipHash: tipHash, VerifyStatus: "valid",
			BundleName: "garda-chain-5f1c2e3a-2026-10-01_2026-10-31.zip", ExpectFrom: "2026-10-01", ExpectTo: "2026-10-31"},
	}
	if status != StatusNoActivity {
		d.Chain.Head = &Head{Seq: 128, EntryHash: headHash, SignedAt: "2026-11-01T02:00:00Z", KeyID: keyID, Signature: headSig}
		d.Chain.Fingerprint = keyID
	}
	switch status {
	case StatusUnsignedTail:
		d.Chain.VerifyStatus, d.Chain.Detail = "unsigned_tail", "2 entries after the newest signed head"
	case StatusFailed:
		d.Chain.VerifyStatus, d.Chain.Detail = "broken", "record changed after it was chained"
	case StatusNoActivity:
		d.Chain.Entries, d.Chain.VerifyStatus = 0, "no_entries"
	}
	return d
}

func TestComposeHasEverySectionInBothLanguages(t *testing.T) {
	for _, lang := range []string{"en", "id"} {
		l := Compose(sample(StatusSigned), lang)
		txt := l.Text()
		for _, want := range []string{"Gudang Utama", "2026-10", "Budi Santoso", tipHash, headHash, headSig, keyID, "128", "garda-verify"} {
			if !strings.Contains(txt, want) {
				t.Errorf("%s: pack text lacks %q", lang, want)
			}
		}
		if l.Pages() < 2 {
			t.Errorf("%s: want a summary page and a last page, got %d", lang, l.Pages())
		}
	}
	en, id := Compose(sample(StatusSigned), "en").Text(), Compose(sample(StatusSigned), "id").Text()
	if en == id {
		t.Fatal("EN and ID packs are identical")
	}
	if !strings.Contains(id, "Satpam") || strings.Contains(strings.ToLower(id), "guard") {
		t.Errorf("ID pack must say Satpam and never the English word guard")
	}
	if !strings.Contains(strings.ToLower(en), "guard") {
		t.Errorf("EN pack should say guard")
	}
}

func TestLastPageHasInstructionsAndLimits(t *testing.T) {
	for _, lang := range []string{"en", "id"} {
		last := Compose(sample(StatusSigned), lang).LastPageText()
		for _, want := range []string{"garda-verify", "--pin " + keyID, "--expect-from 2026-10-01", "--expect-to 2026-10-31",
			"garda-chain-5f1c2e3a-2026-10-01_2026-10-31.zip", "7", "12"} {
			if !strings.Contains(last, want) {
				t.Errorf("%s: last page lacks %q", lang, want)
			}
		}
		// The limits are on the last page, next to the instructions.
		n := strings.Count(last, "•")
		if n < 6 {
			t.Errorf("%s: last page lists %d limits, want the whole 'does NOT prove' list", lang, n)
		}
		if strings.Contains(Compose(sample(StatusSigned), lang).FirstPageText(), "garda-verify --pin") {
			t.Errorf("%s: instructions belong on the last page", lang)
		}
	}
}

func TestUnsignedTailIsNeverPresentedAsVerified(t *testing.T) {
	for _, lang := range []string{"en", "id"} {
		l := Compose(sample(StatusUnsignedTail), lang)
		first := l.FirstPageText()
		if !strings.Contains(first, "UNSIGNED TAIL") {
			t.Errorf("%s: first page lacks the UNSIGNED TAIL notice:\n%s", lang, first)
		}
		if !strings.Contains(l.LastPageText(), "UNSIGNED TAIL") {
			t.Errorf("%s: last page lacks the UNSIGNED TAIL notice", lang)
		}
		// The page that states the result never says valid or verified without a "not".
		for _, bad := range []string{"VALID", "VERIFIED", "TERVERIFIKASI", "PASSED", "LOLOS"} {
			if strings.Contains(first, bad) {
				t.Errorf("%s: unsigned first page says %q", lang, bad)
			}
		}
	}
	if Compose(sample(StatusSigned), "en").HasNotice("UNSIGNED TAIL") {
		t.Error("a signed pack must not carry the unsigned notice")
	}
}

func TestFailedAndNoActivityPacks(t *testing.T) {
	failed := Compose(sample(StatusFailed), "en").FirstPageText()
	if !strings.Contains(failed, "CHAIN CHECK FAILED") || !strings.Contains(failed, "record changed after it was chained") {
		t.Errorf("failed pack:\n%s", failed)
	}
	none := Compose(sample(StatusNoActivity), "en")
	txt := none.Text()
	if !strings.Contains(txt, "NO ACTIVITY") || strings.Contains(txt, "Signed head") && strings.Contains(txt, headHash) {
		t.Errorf("no-activity pack:\n%s", txt)
	}
	if none.Pages() < 2 {
		t.Error("no-activity pack still has the instructions page")
	}
}

func TestRenderProducesAPDF(t *testing.T) {
	for _, lang := range []string{"en", "id"} {
		l := Compose(sample(StatusUnsignedTail), lang)
		out, err := l.Render(sample(StatusUnsignedTail).Generated)
		if err != nil || !bytes.HasPrefix(out, []byte("%PDF-")) || len(out) < 3000 {
			t.Fatalf("%s: render: %v (%d bytes)", lang, err, len(out))
		}
		if _, err := exec.LookPath("pdftotext"); err != nil {
			continue
		}
		cmd := exec.Command("pdftotext", "-layout", "-", "-")
		cmd.Stdin = bytes.NewReader(out)
		text, err := cmd.Output()
		if err != nil {
			t.Fatalf("pdftotext: %v", err)
		}
		flat := strings.Join(strings.Fields(string(text)), "")
		for _, want := range []string{headHash, keyID, "UNSIGNEDTAIL"} {
			if !strings.Contains(flat, want) {
				t.Errorf("%s: PDF text lacks %q", lang, want)
			}
		}
	}
}
