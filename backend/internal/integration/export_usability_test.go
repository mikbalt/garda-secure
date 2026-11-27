//go:build integration

package integration

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/securepatrol/backend/internal/chainproof"
	"github.com/securepatrol/backend/internal/config"
	"github.com/securepatrol/backend/internal/handler"
	"github.com/securepatrol/backend/internal/signer"
)

// Verification usability (TAQ-58, D-075): the export asks the signer for a
// fresh head, bundles cover the entries that amend their records, the online
// check tells "not proven yet" from "broken", and every export is audited.

// signerService serves POST /sign of s as the signer container does.
func signerService(t *testing.T, s *signer.Signer) string {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /sign", signer.SignHandler(s, nil))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL
}

func withSigner(url string) func(*config.Config) { return func(c *config.Config) { c.SignerURL = url } }

// Entries made today are "yesterday's range" for a signer whose clock is two
// days ahead: the signature then post-dates the range, as it would the day after.
func later() time.Time { return time.Now().Add(48 * time.Hour) }

func today() string { return time.Now().UTC().Format("2006-01-02") }

// exportHTTP downloads the bundle as an admin of realm4 and reads it.
func exportHTTP(t *testing.T, e *env, site string) (*chainproof.Bundle, http.Header) {
	t.Helper()
	code, hdr, raw := e.raw("GET", "/api/v1/sites/"+site+"/chain/export?from="+today()+"&to="+today(), e.web(realm4, "exporter", "admin"), nil)
	if code != 200 {
		t.Fatalf("export: %d %s", code, raw)
	}
	b, err := chainproof.Read(raw)
	if err != nil {
		t.Fatal(err)
	}
	return b, hdr
}

func verifyLater(b *chainproof.Bundle, key string) chainproof.Result {
	return chainproof.Verify(b, chainproof.Options{Pins: []string{key}, MaxAge: 26 * time.Hour, Now: later().Add(time.Minute),
		ExpectFrom: today(), ExpectTo: today()})
}

// amend changes the site's incident and chains the change: a later entry for the same record.
func amend(t *testing.T, e *env, id string) {
	t.Helper()
	ownerExec(t, e.pool, `UPDATE incidents SET status = 'resolved' WHERE id = '`+id+`'`, `SELECT chain_append('incident', '`+id+`')`)
}

func TestExportSignsAFreshHeadSoTheBundleVerifies(t *testing.T) {
	s, key := newSigner(t)
	s.Now = later
	e := newEnv(t, withSigner(signerService(t, s)))
	site := packSite(t, e, "sign on export") // three entries, no signed head at all

	b, hdr := exportHTTP(t, e, site.site)
	if hdr.Get("X-Bundle-Signing") != signer.OutcomeSigned || b.Manifest.Signing != signer.OutcomeSigned {
		t.Fatalf("signing %q / %q", hdr.Get("X-Bundle-Signing"), b.Manifest.Signing)
	}
	r := verifyLater(b, pubID(key))
	if r.Status != chainproof.StatusValid || !r.RangeAuthenticated || r.Code() != 0 {
		t.Fatalf("an honest export with a healthy signer: %s (code %d) %+v", r.Status, r.Code(), r.Break)
	}
}

func TestExportWithoutAnswerFromTheSignerIsAnHonestUnsignedTail(t *testing.T) {
	s, key := newSigner(t)
	s.Now = later
	dead := httptest.NewServer(http.NotFoundHandler())
	dead.Close() // nothing listens there any more
	e := newEnv(t, withSigner(dead.URL))
	site := packSite(t, e, "signer down")
	signAt(t, s, site.site, later()) // an older head exists: the three entries are signed...
	e.scanMore(t, site)              // ...and this one is not

	b, hdr := exportHTTP(t, e, site.site)
	if hdr.Get("X-Bundle-Signing") != signer.OutcomeUnreachable || b.Manifest.SigningReason == "" {
		t.Fatalf("signing %q reason %q", hdr.Get("X-Bundle-Signing"), b.Manifest.SigningReason)
	}
	r := verifyLater(b, pubID(key))
	if r.Status != chainproof.StatusUnsignedTail || r.Code() != chainproof.CodeUnsignedTail || !strings.Contains(r.Break.Reason, "unreachable") {
		t.Fatalf("a signer outage must be an honest unsigned tail with the reason: %s %+v", r.Status, r.Break)
	}
}

func TestExportWhenTheSignerRefusesNeverFakesASignature(t *testing.T) {
	s, key := newSigner(t)
	s.Now = later
	refuse := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "signer: different hash at an already signed seq: seq 3", http.StatusConflict)
	}))
	t.Cleanup(refuse.Close)
	e := newEnv(t, withSigner(refuse.URL))
	site := packSite(t, e, "signer refuses")
	signAt(t, s, site.site, later())
	e.scanMore(t, site)

	b, hdr := exportHTTP(t, e, site.site)
	if hdr.Get("X-Bundle-Signing") != signer.OutcomeRefused || !strings.Contains(b.Manifest.SigningReason, "different hash") {
		t.Fatalf("signing %q reason %q", hdr.Get("X-Bundle-Signing"), b.Manifest.SigningReason)
	}
	if r := verifyLater(b, pubID(key)); r.Status != chainproof.StatusUnsignedTail || !strings.Contains(r.Break.Reason, "refused") {
		t.Fatalf("%s %+v", r.Status, r.Break)
	}
}

func (e *env) scanMore(t *testing.T, s chainSite) { t.Helper(); s.scan(t, e.pool) }

// amendedSite has three entries, a signed head on them (signed after the range
// asked for), and then a fourth entry that amends the incident of the third,
// made after the range ended. mid is a time between the two.
func amendedSite(t *testing.T, e *env, s *signer.Signer, name string) (site chainSite, mid time.Time) {
	t.Helper()
	site = packSite(t, e, name)
	var inc string
	if err := e.pool.QueryRow(context.Background(), `SELECT id::text FROM incidents WHERE site_id = $1`, site.site).Scan(&inc); err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	mid = time.Now()
	time.Sleep(20 * time.Millisecond)
	signAt(t, s, site.site, later()) // seq 3, signed after the range
	amend(t, e, inc)                 // seq 4, beyond the newest head and beyond the range
	return site, mid
}

func rangeBundle(t *testing.T, e *env, site string, to time.Time) *chainproof.Bundle {
	t.Helper()
	b, err := exportOf(t, rolePool(t, "sp_runtime"), site, to.Add(-time.Hour), to, 1000)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := offline(t, b)
	return got
}

func TestAmendedRecordIsVerifiedEndToEnd(t *testing.T) {
	s, key := newSigner(t)
	e := newEnv(t)
	site, mid := amendedSite(t, e, s, "amended")
	opts := chainproof.Options{Pins: []string{pubID(key)}, MaxAge: 26 * time.Hour, Now: later(), NoRangeCheck: true}

	// The signer is not asked: the bundle still runs on to the entry that
	// amends the record, so nothing is left unverified (exit 8 is not routine).
	b := rangeBundle(t, e, site.site, mid)
	if b.Manifest.LastSeq != 4 {
		t.Fatalf("last_seq %d: the bundle must cover the amending entry", b.Manifest.LastSeq)
	}
	if r := chainproof.Verify(b, opts); r.Status != chainproof.StatusUnsignedTail || r.RecordsNotChecked != 0 {
		t.Fatalf("%s %+v not checked %d", r.Status, r.Break, r.RecordsNotChecked)
	}
	// With a fresh signed head on the tip: valid, end to end.
	s.Now = later
	if _, err := s.SignSite(context.Background(), site.site, signer.ReasonOnDemand); err != nil {
		t.Fatal(err)
	}
	b = rangeBundle(t, e, site.site, mid)
	if r := chainproof.Verify(b, opts); r.Status != chainproof.StatusValid || r.RecordsNotChecked != 0 || r.Code() != 0 {
		t.Fatalf("an amended record in range must verify end to end: %s %+v", r.Status, r.Break)
	}
}

// The amending entry is only believed when it is in the bundle: a bundle cut
// back to the head before it, with the record's label forged, is still exit 8.
func TestForgedSupersededLabelStillExits8(t *testing.T) {
	s, key := newSigner(t)
	e := newEnv(t)
	site, mid := amendedSite(t, e, s, "forged label")
	b := rangeBundle(t, e, site.site, mid)
	// cut the bundle back to the signed head at seq 3
	b.Entries, b.Heads = b.Entries[:3], b.Heads[:1]
	b.Manifest.LastSeq, b.Manifest.ChainTipSeq, b.Manifest.ChainTipHash = 3, 3, b.Entries[2].EntryHash
	// the incident (seq 3) really was changed again by seq 4, which is not provable here
	if b.Entries[2].RecordState != chainproof.StateSuperseded || b.Entries[2].NextSeq != 4 {
		t.Fatalf("setup: %+v", b.Entries[2])
	}
	r := chainproof.Verify(b, chainproof.Options{Pins: []string{pubID(key)}, MaxAge: 26 * time.Hour, Now: later(), NoRangeCheck: true})
	if r.Status != chainproof.StatusUnverified || r.Code() != chainproof.CodeUnverified {
		t.Fatalf("%s code %d: %+v", r.Status, r.Code(), r.Break)
	}
}

func TestOnlineCheckStatesAreDistinguishable(t *testing.T) {
	s, key := newSigner(t)
	e := newEnv(t)
	site := packSite(t, e, "online states")
	admin := e.web(realm4, "online-admin", "admin")
	url := func(pin string) string {
		u := "/api/v1/sites/" + site.site + "/chain/verify?from=" + today() + "&to=" + today()
		if pin != "" {
			u += "&pin=" + pin
		}
		return u
	}
	state := func(pin string) (string, any, string) {
		r := e.do("GET", url(pin), admin, "", nil)
		if r.status != 200 {
			t.Fatalf("verify: %d %s", r.status, r.raw)
		}
		sum, _ := r.obj()["summary"].(map[string]any)
		return sum["state"].(string), r.obj()["valid"], sum["tone"].(string)
	}
	// entries, no signed head at all: not broken
	if st, valid, tone := state(pubID(key)); st != chainproof.StateUnsignedTail || valid != nil || tone != "notice" {
		t.Fatalf("unsigned: %s %v %s", st, valid, tone)
	}
	signAt(t, s, site.site, time.Now())
	if st, valid, _ := state(pubID(key)); st != chainproof.StateValidPinned || valid != true {
		t.Fatalf("pinned: %s %v", st, valid)
	}
	if st, valid, _ := state(""); st != chainproof.StateUnverifiedTrust || valid != nil {
		t.Fatalf("no pin: %s %v", st, valid)
	}
	s.Now = time.Now
	e.scanMore(t, site) // an entry after the head
	if st, valid, _ := state(pubID(key)); st != chainproof.StateUnsignedTail || valid != nil {
		t.Fatalf("unsigned tail: %s %v", st, valid)
	}
	// only a real break is broken
	ownerExec(t, e.pool, `UPDATE incidents SET description = 'nothing happened' WHERE site_id = '`+site.site+`'`)
	if st, valid, tone := state(pubID(key)); st != chainproof.StateBroken || valid != false || tone != "broken" {
		t.Fatalf("broken: %s %v %s", st, valid, tone)
	}
	// an empty range
	empty := chainSeed(t, e.pool, "online empty")
	ownerExec(t, e.pool, `UPDATE sites SET keycloak_realm = '`+realm4+`', timezone = 'UTC' WHERE id = '`+empty.site+`'`)
	r := e.do("GET", "/api/v1/sites/"+empty.site+"/chain/verify?from="+today()+"&to="+today(), admin, "", nil)
	if sum, _ := r.obj()["summary"].(map[string]any); sum["state"] != chainproof.StateNoEntries || r.obj()["valid"] != nil {
		t.Fatalf("no entries: %s", r.raw)
	}
}

func TestEveryExportWritesOneAuditRowAndOnlyOpsOfTheRealmReadIt(t *testing.T) {
	e := newEnv(t)
	site := packSite(t, e, "export audit")
	admin := e.web(realm4, "auditor-admin", "admin")
	before := func() int {
		r := e.do("GET", "/api/v1/sites/"+site.site+"/chain/exports", admin, "", nil)
		if r.status != 200 {
			t.Fatalf("list: %d %s", r.status, r.raw)
		}
		return len(r.obj()["rows"].([]any))
	}
	if n := before(); n != 0 {
		t.Fatalf("rows before any export: %d", n)
	}
	exportHTTP(t, e, site.site)
	if r := e.do("GET", packURL(site.site, thisMonth(), ""), admin, "", nil); r.status != 200 {
		t.Fatalf("proof pack: %d", r.status)
	}
	r := e.do("GET", "/api/v1/sites/"+site.site+"/chain/exports", admin, "", nil)
	rows := r.obj()["rows"].([]any)
	if len(rows) != 2 {
		t.Fatalf("rows: %s", r.raw)
	}
	newest, oldest := rows[0].(map[string]any), rows[1].(map[string]any)
	if oldest["kind"] != "bundle" || newest["kind"] != "proof_pack" || oldest["actor"] != "exporter" || newest["actor"] != "auditor-admin" ||
		oldest["result"] != "ok" || oldest["bytes"].(float64) < 100 || oldest["signing"] != signer.OutcomeNotSetup || oldest["site_id"] != site.site {
		t.Fatalf("rows: %s", r.raw)
	}
	if strings.Contains(string(r.raw), "scanned_at") || strings.Contains(string(r.raw), "photo") {
		t.Fatalf("an audit row must not carry record content: %s", r.raw)
	}

	// who may read it
	if r := e.do("GET", "/api/v1/sites/"+site.site+"/chain/exports", e.web(realm4, "sup", "supervisor"), "", nil); r.status != 200 {
		t.Errorf("supervisor: %d", r.status)
	}
	for _, role := range []string{"client-readonly", "hr"} {
		if r := e.do("GET", "/api/v1/sites/"+site.site+"/chain/exports", e.web(realm4, "u-"+role, role), "", nil); r.status != 403 {
			t.Errorf("%s: %d, want 403", role, r.status)
		}
	}
	if r := e.do("GET", "/api/v1/sites/"+site.site+"/chain/exports", e.web(realm1, "other-admin", "admin"), "", nil); r.status != 404 {
		t.Errorf("another realm's admin: %d, want 404", r.status)
	}

	// a refused export is audited as well, and the table is append-only
	old := handler.ChainLimits.ExportMaxEntries
	handler.ChainLimits.ExportMaxEntries = 1
	defer func() { handler.ChainLimits.ExportMaxEntries = old }()
	code, _, _ := e.raw("GET", "/api/v1/sites/"+site.site+"/chain/export?from="+today()+"&to="+today(), e.web(realm4, "other-exporter", "admin"), nil)
	if code != 400 {
		t.Fatalf("too large: %d", code)
	}
	var result string
	if err := e.pool.QueryRow(context.Background(), `SELECT result FROM export_audit WHERE site_id = $1 AND actor = 'other-exporter'`, site.site).Scan(&result); err != nil || result != "too_large" {
		t.Fatalf("refused export: %q %v", result, err)
	}
	rt := rolePool(t, "sp_runtime")
	for _, q := range []string{`UPDATE export_audit SET actor = 'x'`, `DELETE FROM export_audit`, `TRUNCATE export_audit`} {
		if _, err := rt.Exec(context.Background(), q); err == nil {
			t.Errorf("%s was allowed", q)
		}
	}
}

// The three cases of TAQ-58 with the real garda-verify binary on the bundles
// the API serves: honest export, amended record, signer down; then the audit.
func TestUsabilityWalkthroughWithTheRealVerifier(t *testing.T) {
	s, key := newSigner(t)
	s.Now = later
	mux := http.NewServeMux()
	var down bool
	mux.HandleFunc("POST /sign", func(w http.ResponseWriter, r *http.Request) {
		if down {
			http.Error(w, "down", http.StatusBadGateway)
			return
		}
		signer.SignHandler(s, nil)(w, r)
	})
	svc := httptest.NewServer(mux)
	t.Cleanup(svc.Close)
	e := newEnv(t, withSigner(svc.URL))
	site := packSite(t, e, "walkthrough")
	admin := e.web(realm4, "walker", "admin")

	dir := t.TempDir()
	bin := filepath.Join(dir, "garda-verify")
	if out, err := exec.Command("go", "build", "-o", bin, "../../cmd/garda-verify").CombinedOutput(); err != nil {
		t.Fatalf("build garda-verify: %v\n%s", err, out)
	}
	check := func(label string) (string, int) {
		code, _, raw := e.raw("GET", "/api/v1/sites/"+site.site+"/chain/export?from="+today()+"&to="+today(), admin, nil)
		if code != 200 {
			t.Fatalf("%s: export %d", label, code)
		}
		name := filepath.Join(dir, strings.ReplaceAll(label, " ", "-")+".zip")
		if err := os.WriteFile(name, raw, 0o600); err != nil {
			t.Fatal(err)
		}
		// the verifier's clock is the real one: the head was signed "the day after" (signer clock +48h), so no max-age
		out, err := exec.Command(bin, "--pin", pubID(key), "--expect-from", today(), "--expect-to", today(), "--max-age", "0", name).CombinedOutput()
		exit := 0
		if ee, ok := err.(*exec.ExitError); ok {
			exit = ee.ExitCode()
		}
		t.Logf("--- %s: exit %d\n%s", label, exit, out)
		return string(out), exit
	}

	if out, exit := check("honest export"); exit != 0 || !strings.Contains(out, "RESULT: VALID") {
		t.Fatalf("honest export: exit %d\n%s", exit, out)
	}
	var inc string
	if err := e.pool.QueryRow(context.Background(), `SELECT id::text FROM incidents WHERE site_id = $1`, site.site).Scan(&inc); err != nil {
		t.Fatal(err)
	}
	time.Sleep(signer.ReuseWindow + 100*time.Millisecond) // the first head is not reused for this export
	amend(t, e, inc)
	if out, exit := check("amended record"); exit != 0 {
		t.Fatalf("amended record: exit %d\n%s", exit, out)
	}
	time.Sleep(signer.ReuseWindow + 100*time.Millisecond)
	e.scanMore(t, site)
	down = true
	if out, exit := check("signer down"); exit != chainproof.CodeUnsignedTail || !strings.Contains(out, "signer") {
		t.Fatalf("signer down: exit %d\n%s", exit, out)
	}
	r := e.do("GET", "/api/v1/sites/"+site.site+"/chain/exports", admin, "", nil)
	t.Logf("export audit: %s", r.raw)
	if rows := r.obj()["rows"].([]any); len(rows) != 3 {
		t.Fatalf("audit rows: %d", len(rows))
	}
}
