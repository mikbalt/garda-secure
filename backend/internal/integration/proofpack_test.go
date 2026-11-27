//go:build integration

package integration

import (
	"bytes"
	"context"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/securepatrol/backend/internal/model"
	"github.com/securepatrol/backend/internal/proofpack"
	"github.com/securepatrol/backend/internal/repository"
	"github.com/securepatrol/backend/internal/signer"
	"github.com/securepatrol/backend/internal/storage"
)

// Signed monthly proof pack (TAQ-20, D-074).

// memStore is the object store of the scheduled job.
type memStore struct {
	mu   sync.Mutex
	objs map[string][]byte
}

func (m *memStore) Exists(_ context.Context, _, key string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.objs[key]
	return ok, nil
}

func (m *memStore) PutNew(_ context.Context, _, key string, data []byte, _, _ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.objs[key]; ok {
		return storage.ErrExists
	}
	m.objs[key] = data
	return nil
}

// keysOf are the kept objects of one site: the database is shared with the other tests' sites.
func (m *memStore) keysOf(site string) []string {
	var out []string
	for _, k := range m.keys() {
		if strings.Contains(k, "/"+site+"/") {
			out = append(out, k)
		}
	}
	return out
}

func (m *memStore) keys() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []string
	for k := range m.objs {
		out = append(out, k)
	}
	return out
}

func pdfText(t *testing.T, pdf []byte) string { return pdfPageText(t, pdf, 0, 0) }

// pdfPageText is the text of pages first..last (0 = all), without white space.
func pdfPageText(t *testing.T, pdf []byte, first, last int) string {
	t.Helper()
	if _, err := exec.LookPath("pdftotext"); err != nil {
		t.Skip("pdftotext (poppler) is not installed")
	}
	args := []string{"-layout"}
	if first > 0 {
		args = append(args, "-f", strconv.Itoa(first), "-l", strconv.Itoa(last))
	}
	cmd := exec.Command("pdftotext", append(args, "-", "-")...)
	cmd.Stdin = bytes.NewReader(pdf)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("pdftotext: %v", err)
	}
	return strings.Join(strings.Fields(string(out)), "")
}

// packSite is a chained site in realm4 (UTC) with a patrol, an attendance and an incident.
func packSite(t *testing.T, e *env, name string) chainSite {
	t.Helper()
	s := chainSeed(t, e.pool, name)
	ownerExec(t, e.pool, `UPDATE sites SET keycloak_realm = '`+realm4+`', timezone = 'UTC' WHERE id = '`+s.site+`'`)
	s.scan(t, e.pool)
	s.attendance(t, e.pool)
	s.incident(t, e.pool)
	return s
}

func thisMonth() string { return time.Now().UTC().Format("2006-01") }

func nextMonthStart() time.Time {
	n := time.Now().UTC()
	return time.Date(n.Year(), n.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, 1, 0)
}

// signAt signs the site's head with the signer's clock at at.
func signAt(t *testing.T, s *signer.Signer, site string, at time.Time) {
	t.Helper()
	s.Now = func() time.Time { return at }
	if _, err := s.SignSite(context.Background(), site, "hourly"); err != nil {
		t.Fatal(err)
	}
}

func packURL(site, month, extra string) string {
	return "/api/v1/sites/" + site + "/proof-pack?month=" + month + extra
}

func TestProofPackHasEverySectionAndTheDatabaseValues(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	admin := e.web(realm4, "pp-admin", "admin")
	sup := e.web(realm4, "pp-sup", "supervisor")
	s, key := newSigner(t)
	site := packSite(t, e, "Pack Valid")
	signAt(t, s, site.site, nextMonthStart().Add(time.Hour)) // signed after the month ended

	var headHash, sig, keyID string
	var seq int64
	if err := e.pool.QueryRow(ctx, `SELECT seq, encode(entry_hash,'hex'), encode(signature,'hex'), key_id FROM signed_heads WHERE site_id = $1 ORDER BY seq DESC, id DESC LIMIT 1`,
		site.site).Scan(&seq, &headHash, &sig, &keyID); err != nil {
		t.Fatal(err)
	}
	if keyID != pubID(key) {
		t.Fatalf("key id %s != %s", keyID, pubID(key))
	}

	for _, tok := range []string{admin, sup} {
		code, hdr, body := e.raw("GET", packURL(site.site, thisMonth(), ""), tok, nil)
		if code != 200 || hdr.Get("Content-Type") != "application/pdf" || !bytes.HasPrefix(body, []byte("%PDF-")) {
			t.Fatalf("pack: %d %s %.80s", code, hdr.Get("Content-Type"), body)
		}
		if hdr.Get("X-Proofpack-Status") != "signed" || hdr.Get("Cache-Control") != "no-store" ||
			!strings.Contains(hdr.Get("Content-Disposition"), "proof-pack-") {
			t.Fatalf("headers: status %q, %v", hdr.Get("X-Proofpack-Status"), hdr)
		}
	}
	// The pack's own figures are the database's.
	_, _, body := e.raw("GET", packURL(site.site, thisMonth(), ""), admin, nil)
	text := pdfText(t, body)
	for _, want := range []string{headHash, sig, keyID} {
		if !strings.Contains(text, want) {
			t.Errorf("PDF text lacks %q", want)
		}
	}
	// The file name and the dates it tells the reader to use are the ones the export gives.
	first := time.Now().UTC().Format("2006-01") + "-01"
	last := nextMonthStart().AddDate(0, 0, -1).Format("2006-01-02")
	code, hdr, _ := e.raw("GET", "/api/v1/sites/"+site.site+"/chain/export?from="+first+"&to="+last, admin, nil)
	if code != 200 {
		t.Fatalf("export: %d", code)
	}
	name := strings.TrimSuffix(strings.TrimPrefix(hdr.Get("Content-Disposition"), `attachment; filename="`), `"`)
	if !strings.Contains(text, name) || !strings.Contains(text, "--expect-from"+first+"--expect-to"+last) {
		t.Errorf("instructions do not match the export %q (%s..%s)", name, first, last)
	}
	if !strings.Contains(text, "garda-verify--pin"+keyID) {
		t.Errorf("last page instructions lack the pinned command")
	}
	if strings.Contains(pdfPageText(t, body, 1, 1), "UNSIGNEDTAIL") {
		t.Errorf("a pack signed after the month must not say UNSIGNED TAIL on its first page")
	}
}

func TestProofPackAuthorisation(t *testing.T) {
	e := newEnv(t)
	site := packSite(t, e, "Pack Auth")
	url := packURL(site.site, thisMonth(), "")
	for _, role := range []string{"client-readonly", "hr"} {
		if r := e.do("GET", url, e.web(realm4, "pp-"+role, role), "", nil); r.status != 403 {
			t.Errorf("%s: status %d, want 403", role, r.status)
		}
	}
	if r := e.do("GET", url, "", "", nil); r.status != 401 {
		t.Errorf("no token: %d", r.status)
	}
	if r := e.do("GET", url, e.web(realm1, "pp-other-admin", "admin"), "", nil); r.status != 404 {
		t.Errorf("another realm's admin: %d, want 404", r.status)
	}
	admin := e.web(realm4, "pp-admin2", "admin")
	for path, want := range map[string]int{
		packURL(site.site, "2026-13", ""):                                400,
		packURL(site.site, "", ""):                                       400,
		packURL(site.site, "2999-01", ""):                                400,
		packURL(site.site, thisMonth(), "&lang=fr"):                      400,
		packURL("00000000-0000-0000-0000-000000000000", thisMonth(), ""): 404,
	} {
		if r := e.do("GET", path, admin, "", nil); r.status != want {
			t.Errorf("%s: %d, want %d: %s", path, r.status, want, r.raw)
		}
	}
}

func TestProofPackUnsignedTailAndEmptyMonth(t *testing.T) {
	e := newEnv(t)
	admin := e.web(realm4, "pp-admin3", "admin")
	s, _ := newSigner(t)

	// Entries after the newest signed head.
	tail := packSite(t, e, "Pack Tail")
	signAt(t, s, tail.site, time.Now())
	tail.scan(t, e.pool)
	code, hdr, body := e.raw("GET", packURL(tail.site, thisMonth(), "&lang=en"), admin, nil)
	if code != 200 || hdr.Get("X-Proofpack-Status") != "unsigned_tail" {
		t.Fatalf("unsigned tail: %d status %q", code, hdr.Get("X-Proofpack-Status"))
	}
	if txt := pdfPageText(t, body, 1, 1); !strings.Contains(txt, "UNSIGNEDTAIL") {
		t.Errorf("PDF lacks the UNSIGNED TAIL notice")
	}

	// A head that was signed before the month ended covers everything but is not enough either.
	early := packSite(t, e, "Pack Early")
	signAt(t, s, early.site, time.Now())
	if _, hdr, _ := e.raw("GET", packURL(early.site, thisMonth(), ""), admin, nil); hdr.Get("X-Proofpack-Status") != "unsigned_tail" {
		t.Errorf("head signed before the month ended: status %q", hdr.Get("X-Proofpack-Status"))
	}

	// No signed head at all.
	none := packSite(t, e, "Pack Unsigned")
	if _, hdr, _ := e.raw("GET", packURL(none.site, thisMonth(), ""), admin, nil); hdr.Get("X-Proofpack-Status") != "unsigned_tail" {
		t.Errorf("no head: status %q", hdr.Get("X-Proofpack-Status"))
	}

	// A month without activity is a valid pack, not an error, and not "verified".
	quiet := chainSeed(t, e.pool, "Pack Quiet")
	ownerExec(t, e.pool, `UPDATE sites SET keycloak_realm = '`+realm4+`', timezone = 'UTC' WHERE id = '`+quiet.site+`'`)
	code, hdr, body = e.raw("GET", packURL(quiet.site, "2020-01", "&lang=en"), admin, nil)
	if code != 200 || hdr.Get("X-Proofpack-Status") != "no_activity" {
		t.Fatalf("empty month: %d status %q", code, hdr.Get("X-Proofpack-Status"))
	}
	if txt := pdfText(t, body); !strings.Contains(txt, "NOACTIVITY") {
		t.Errorf("empty month lacks the NO ACTIVITY notice")
	}
}

func TestProofPackLanguages(t *testing.T) {
	e := newEnv(t)
	admin := e.web(realm4, "pp-admin4", "admin")
	s, _ := newSigner(t)
	site := packSite(t, e, "Pack Lang")
	signAt(t, s, site.site, nextMonthStart().Add(time.Hour))
	_, _, en := e.raw("GET", packURL(site.site, thisMonth(), "&lang=en"), admin, nil)
	_, _, id := e.raw("GET", packURL(site.site, thisMonth(), "&lang=id"), admin, nil)
	te, ti := pdfText(t, en), pdfText(t, id)
	if !strings.Contains(ti, "Satpam") || strings.Contains(strings.ToLower(ti), "guard") {
		t.Errorf("ID pack must say Satpam and not guard")
	}
	if !strings.Contains(strings.ToLower(te), "guard") || strings.Contains(te, "Satpam") {
		t.Errorf("EN pack must say guard")
	}
	// Default language is the site's (id).
	_, _, dflt := e.raw("GET", packURL(site.site, thisMonth(), ""), admin, nil)
	if !strings.Contains(pdfText(t, dflt), "Satpam") {
		t.Errorf("default language should be the site's (id)")
	}
}

func TestProofPackScheduleGeneratesOncePerMonth(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	s, _ := newSigner(t)
	site := packSite(t, e, "Pack Sched")
	first := nextMonthStart()
	store := &memStore{objs: map[string][]byte{}}
	month := thisMonth()

	// Not the first days of a month: nothing to do.
	if err := e.sched.RunProofPacks(ctx, store, first.AddDate(0, 0, 9)); err != nil || len(store.keysOf(site.site)) != 0 {
		t.Fatalf("day 10: %v %v", err, store.keysOf(site.site))
	}
	// Before 07:00 on day 1: not yet (as the recap).
	if err := e.sched.RunProofPacks(ctx, store, first.Add(2*time.Hour)); err != nil || len(store.keysOf(site.site)) != 0 {
		t.Fatalf("day 1 02:00: %v %v", err, store.keysOf(site.site))
	}

	// Day 1, no signature after the month yet: an unsigned copy is kept, once.
	now := first.Add(8 * time.Hour)
	for i := 0; i < 2; i++ {
		if err := e.sched.RunProofPacks(ctx, store, now); err != nil {
			t.Fatal(err)
		}
	}
	mine := store.keysOf(site.site)
	if len(mine) != 1 || !strings.HasSuffix(mine[0], "/"+month+".unsigned.pdf") || !bytes.HasPrefix(store.objs[mine[0]], []byte("%PDF-")) {
		t.Fatalf("after day-1 run: %v", mine)
	}

	// The signer signs after the month ended: the next run keeps the signed pack, once.
	signAt(t, s, site.site, first.Add(7*time.Hour+30*time.Minute))
	for i := 0; i < 3; i++ {
		if err := e.sched.RunProofPacks(ctx, store, now.Add(time.Duration(i)*10*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	signed := store.keysOf(site.site)
	if len(signed) != 2 || !contains(signed, "proof-packs/"+site.site+"/"+month+".pdf") {
		t.Fatalf("after signing: %v", signed)
	}
	// Re-running changes nothing.
	before := len(store.keysOf(site.site))
	for i := 0; i < 2; i++ {
		if err := e.sched.RunProofPacks(ctx, store, now.Add(time.Duration(i+1)*time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	if after := store.keysOf(site.site); len(after) != before {
		t.Fatalf("idempotence: %v -> %v", before, after)
	}
}

func contains(l []string, s string) bool {
	for _, v := range l {
		if v == s {
			return true
		}
	}
	return false
}

// The kept copy is what Collect says, so a client can compare it with a fresh export.
func TestProofPackCollectMatchesTheChain(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	s, _ := newSigner(t)
	site := packSite(t, e, "Pack Collect")
	signAt(t, s, site.site, nextMonthStart().Add(time.Hour))
	from, to := time.Now().UTC().AddDate(0, 0, -time.Now().UTC().Day()+1).Truncate(24*time.Hour), nextMonthStart()
	b, err := exportOf(t, e.pool, site.site, from, to, 10_000)
	if err != nil {
		t.Fatal(err)
	}
	d, err := proofpack.Collect(ctx, e.pool, siteOf(t, e, site.site), thisMonth(), b, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if d.Chain.Status != proofpack.StatusSigned || d.Chain.Head == nil || d.Chain.Head.Seq != int64(len(b.Entries))+b.Manifest.AnchorSeq {
		t.Fatalf("chain section: %+v", d.Chain)
	}
	if d.Chain.Entries != 3 || d.IncidentsTotal != 1 || d.Totals.ScheduledShifts != 1 {
		t.Fatalf("figures: entries %d incidents %d shifts %d", d.Chain.Entries, d.IncidentsTotal, d.Totals.ScheduledShifts)
	}
	if d.Chain.TipHash == "" || d.Chain.Fingerprint != d.Chain.Head.KeyID {
		t.Fatalf("chain head hash %q fingerprint %q", d.Chain.TipHash, d.Chain.Fingerprint)
	}
}

func siteOf(t *testing.T, e *env, id string) *model.Site {
	t.Helper()
	site, err := repository.GetSite(context.Background(), e.pool, id)
	if err != nil {
		t.Fatal(err)
	}
	return site
}
