//go:build integration

package integration

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/securepatrol/backend/internal/chainproof"
	"github.com/securepatrol/backend/internal/handler"
)

// Verification endpoint, export bundle and offline check (TAQ-18, D-073).

func wideRange() (time.Time, time.Time) {
	return time.Now().Add(-24 * time.Hour), time.Now().Add(24 * time.Hour)
}

// exportOf reads the bundle as the API does: runtime role, one read-only snapshot.
func exportOf(t *testing.T, pool *pgxpool.Pool, site string, from, to time.Time, max int64) (*chainproof.Bundle, error) {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	return chainproof.Export(ctx, tx, site, "UTC", from, to, time.Now(), max)
}

// offline writes the bundle to bytes and reads it back, as garda-verify gets it.
func offline(t *testing.T, b *chainproof.Bundle) (*chainproof.Bundle, []byte) {
	t.Helper()
	var buf bytes.Buffer
	if err := b.Write(&buf); err != nil {
		t.Fatal(err)
	}
	got, err := chainproof.Read(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	return got, buf.Bytes()
}

func verifyExport(t *testing.T, pool *pgxpool.Pool, site, pin string) chainproof.Result {
	t.Helper()
	from, to := wideRange()
	b, err := exportOf(t, pool, site, from, to, 1_000_000)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := offline(t, b)
	return chainproof.Verify(got, chainproof.Options{Pins: []string{pin}, MaxAge: 26 * time.Hour})
}

func ownerExec(t *testing.T, pool *pgxpool.Pool, steps ...string) {
	t.Helper()
	for _, q := range steps {
		if _, err := pool.Exec(context.Background(), q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
}

// A site with all four kinds of record, one of them changed again (so the
// bundle holds a superseded entry), and a signed head.
func seedAllKinds(t *testing.T, pool *pgxpool.Pool, name string) (chainSite, map[string]string) {
	t.Helper()
	ctx := context.Background()
	s := chainSeed(t, pool, name)
	ids := map[string]string{"scan": s.scan(t, pool), "incident": s.incident(t, pool), "attendance": s.attendance(t, pool)}
	if err := pool.QueryRow(ctx, `INSERT INTO incident_amendments (incident_id, changes, reason, amended_by)
		VALUES ($1, '{"severity": "high"}', 'checked', 'admin') RETURNING id`, ids["incident"]).Scan(new(string)); err != nil {
		t.Fatal(err)
	}
	var am string
	if err := pool.QueryRow(ctx, `SELECT id FROM incident_amendments WHERE incident_id = $1`, ids["incident"]).Scan(&am); err != nil {
		t.Fatal(err)
	}
	ids["incident_amendment"] = am
	ownerExec(t, pool,
		`SELECT chain_append('incident_amendment', '`+am+`')`,
		`UPDATE incidents SET status = 'resolved' WHERE id = '`+ids["incident"]+`'`,
		`SELECT chain_append('incident', '`+ids["incident"]+`')`)
	return s, ids
}

// The export repeats chain_record_hash's column lists in chain_record_canonical:
// they must stay the same text.
func TestExportCanonicalMatchesRecordHash(t *testing.T) {
	pool := chainPool(t)
	_, ids := seedAllKinds(t, pool, "canon")
	for kind, id := range ids {
		var canon string
		var want, got []byte
		if err := pool.QueryRow(context.Background(),
			`SELECT chain_record_canonical($1, $2::uuid), chain_record_hash($1, $2::uuid)`, kind, id).Scan(&canon, &want); err != nil {
			t.Fatal(kind, err)
		}
		sum := sha256.Sum256([]byte(kind + ":" + canon))
		got = sum[:]
		if !bytes.Equal(got, want) {
			t.Errorf("%s: sha256(kind:canonical) = %x, chain_record_hash = %x", kind, got, want)
		}
	}
}

func TestChainBundleVerifiesOffline(t *testing.T) {
	ctx := context.Background()
	pool := chainPool(t)
	rt := rolePool(t, "sp_runtime")
	s, key := newSigner(t)
	site, _ := seedAllKinds(t, pool, "bundle ok")
	if _, err := s.SignSite(ctx, site.site, "hourly"); err != nil {
		t.Fatal(err)
	}

	from, to := wideRange()
	b, err := exportOf(t, rt, site.site, from, to, 1000)
	if err != nil {
		t.Fatal(err)
	}
	got, raw := offline(t, b)
	r := chainproof.Verify(got, chainproof.Options{Pins: []string{pubID(key)}, MaxAge: 26 * time.Hour})
	if r.Status != chainproof.StatusValid || r.Entries != 5 || r.Heads != 1 {
		t.Fatalf("valid bundle: %s %+v entries %d heads %d", r.Status, r.Break, r.Entries, r.Heads)
	}
	if bytes.Contains(raw, []byte(hex.EncodeToString(key.Seed()))) {
		t.Fatal("the private key seed is in the bundle")
	}
	// The incident changed twice (created, amended-by-status): its first entry is superseded.
	var superseded, latest int
	for _, e := range got.Entries {
		switch e.RecordState {
		case chainproof.StateSuperseded:
			superseded++
			if e.Record != "" || e.NextSeq <= e.Seq {
				t.Errorf("superseded entry %d: %+v", e.Seq, e)
			}
		case chainproof.StateLatest:
			latest++
		}
	}
	if superseded != 1 || latest != 4 {
		t.Fatalf("superseded %d latest %d, want 1 and 4", superseded, latest)
	}
	// A key that is not the pinned one is refused.
	if r := chainproof.Verify(got, chainproof.Options{Pins: []string{pubID(newKey(t))}}); r.Code() != chainproof.CodeHead {
		t.Fatalf("wrong fingerprint: %s %+v", r.Status, r.Break)
	}
}

func TestChainBundleDetectsTamperingInTheDatabase(t *testing.T) {
	ctx := context.Background()
	pool := chainPool(t)
	rt := rolePool(t, "sp_runtime")
	s, key := newSigner(t)
	pin := pubID(key)
	noTriggers := func(q ...string) []string {
		return append(append([]string{`ALTER TABLE record_chain DISABLE TRIGGER USER`, `ALTER TABLE record_chain_heads DISABLE TRIGGER USER`}, q...),
			`ALTER TABLE record_chain ENABLE TRIGGER USER`, `ALTER TABLE record_chain_heads ENABLE TRIGGER USER`)
	}
	cases := []struct {
		name   string
		tamper func(ids map[string]string, site string) []string
		code   int
		reason string
		kind   string // "" = not checked
		seq    int64  // 0 = not checked
	}{
		{"edited record", func(ids map[string]string, _ string) []string {
			return []string{`UPDATE attendance_records SET status = 'absent' WHERE id = '` + ids["attendance"] + `'`}
		}, chainproof.CodeChain, "record modified", "attendance", 3},
		{"deleted record", func(ids map[string]string, _ string) []string {
			return []string{`DELETE FROM checkpoint_scans WHERE id = '` + ids["scan"] + `'`}
		}, chainproof.CodeChain, "record deleted", "scan", 1},
		{"edited chain entry", func(_ map[string]string, site string) []string {
			return noTriggers(`UPDATE record_chain SET origin = 'backfill' WHERE site_id = '` + site + `' AND seq = 2`)
		}, chainproof.CodeChain, "entry content", "", 2},
		{"deleted chain entry", func(_ map[string]string, site string) []string {
			return noTriggers(`DELETE FROM record_chain WHERE site_id = '` + site + `' AND seq = 2`)
		}, chainproof.CodeChain, "entry missing", "", 2},
		{"deleted newest entry (the incident goes back to its older state)", func(_ map[string]string, site string) []string {
			return noTriggers(`DELETE FROM record_chain WHERE site_id = '`+site+`' AND seq = 5`,
				`UPDATE record_chain_heads h SET last_seq = c.seq, last_hash = c.entry_hash FROM record_chain c WHERE h.site_id = '`+site+`' AND c.site_id = h.site_id AND c.seq = 4`)
		}, chainproof.CodeChain, "record modified", "incident", 2}, // the unit test covers the bare dropped tail
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			site, ids := seedAllKinds(t, pool, "tamper "+tc.name)
			if _, err := s.SignSite(ctx, site.site, "hourly"); err != nil {
				t.Fatal(err)
			}
			if r := verifyExport(t, rt, site.site, pin); r.Status != chainproof.StatusValid {
				t.Fatalf("before tampering: %s %+v", r.Status, r.Break)
			}
			ownerExec(t, pool, tc.tamper(ids, site.site)...)
			r := verifyExport(t, rt, site.site, pin)
			if r.Status != chainproof.StatusBroken || r.Code() != tc.code || r.Break == nil || !strings.Contains(r.Break.Reason, tc.reason) {
				t.Fatalf("after tampering: %s code %d %+v", r.Status, r.Code(), r.Break)
			}
			if r.Break.Seq != tc.seq || (tc.kind != "" && r.Break.Kind != tc.kind) {
				t.Fatalf("first break seq %d kind %s, want seq %d kind %s", r.Break.Seq, r.Break.Kind, tc.seq, tc.kind)
			}
		})
	}

	t.Run("rewritten and re-chained past", func(t *testing.T) {
		site, inc := seedChain(t, pool, "tamper rechain")
		if _, err := s.SignSite(ctx, site.site, "hourly"); err != nil {
			t.Fatal(err)
		}
		rewriteAndRechain(t, pool, site.site, inc)
		if b, err := repoFirstBreak(pool, site.site); err != nil || b {
			t.Fatalf("the chain alone should look intact: broken=%v err=%v", b, err)
		}
		r := verifyExport(t, rt, site.site, pin)
		if r.Status != chainproof.StatusBroken || r.Code() != chainproof.CodeHead || r.Break.Seq != 3 || !strings.Contains(r.Break.Reason, "differs from the signed head") {
			t.Fatalf("re-chained past: %s %+v", r.Status, r.Break)
		}
	})
}

func repoFirstBreak(pool *pgxpool.Pool, site string) (bool, error) {
	var seq int64
	err := pool.QueryRow(context.Background(), `SELECT seq FROM chain_first_break($1::uuid)`, site).Scan(&seq)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func TestChainBundleUnsignedAndStale(t *testing.T) {
	pool := chainPool(t)
	rt := rolePool(t, "sp_runtime")
	_, key := newSigner(t)
	site, _ := seedAllKinds(t, pool, "unsigned")
	r := verifyExport(t, rt, site.site, pubID(key))
	if r.Status != chainproof.StatusUnsigned || r.Code() != chainproof.CodeUnsigned {
		t.Fatalf("no head: %s %+v", r.Status, r.Break)
	}
}

// A range by checkpoint (starting from a signed head) gives the same answer as
// checking the whole chain, and checks fewer entries.
func TestChainBundleRangeByCheckpoint(t *testing.T) {
	ctx := context.Background()
	pool := chainPool(t)
	rt := rolePool(t, "sp_runtime")
	s, key := newSigner(t)
	pin := pubID(key)
	site := chainSeed(t, pool, "checkpoint")
	sign := func() {
		if _, err := s.SignSite(ctx, site.site, "hourly"); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 3; i++ {
		site.scan(t, pool)
	}
	sign() // head at 3
	for i := 0; i < 3; i++ {
		site.scan(t, pool)
	}
	sign() // head at 6
	for i := 0; i < 2; i++ {
		site.scan(t, pool)
	}
	sign() // head at 8
	var from, to time.Time
	if err := pool.QueryRow(ctx, `SELECT (SELECT created_at FROM record_chain WHERE site_id = $1 AND seq = 4),
		(SELECT created_at FROM record_chain WHERE site_id = $1 AND seq = 7)`, site.site).Scan(&from, &to); err != nil {
		t.Fatal(err)
	}
	part := func() chainproof.Result {
		b, err := exportOf(t, rt, site.site, from, to, 1000)
		if err != nil {
			t.Fatal(err)
		}
		got, _ := offline(t, b)
		if got.Manifest.AnchorSeq != 3 || got.Manifest.FirstSeq != 4 || got.Manifest.LastSeq != 6 {
			t.Fatalf("range 4..6 exported as anchor %d, seq %d..%d", got.Manifest.AnchorSeq, got.Manifest.FirstSeq, got.Manifest.LastSeq)
		}
		return chainproof.Verify(got, chainproof.Options{Pins: []string{pin}})
	}
	full := func() chainproof.Result { return verifyExport(t, rt, site.site, pin) }

	if p, f := part(), full(); p.Status != chainproof.StatusValid || f.Status != chainproof.StatusValid || p.Entries != 3 || f.Entries != 8 {
		t.Fatalf("valid data: range %s (%d entries), full %s (%d)", p.Status, p.Entries, f.Status, f.Entries)
	}
	// An entry inside the range: both find it, at the same seq.
	ownerExec(t, pool, `ALTER TABLE record_chain DISABLE TRIGGER USER`,
		`UPDATE record_chain SET origin = 'backfill' WHERE site_id = '`+site.site+`' AND seq = 5`, `ALTER TABLE record_chain ENABLE TRIGGER USER`)
	p, f := part(), full()
	if p.Break == nil || f.Break == nil || p.Break.Seq != 5 || f.Break.Seq != 5 || p.Code() != f.Code() {
		t.Fatalf("edit at 5: range %+v, full %+v", p.Break, f.Break)
	}
	// An entry before the checkpoint is not in this range's proof; the full check finds it.
	ownerExec(t, pool, `ALTER TABLE record_chain DISABLE TRIGGER USER`,
		`UPDATE record_chain SET origin = 'live' WHERE site_id = '`+site.site+`' AND seq = 5`,
		`UPDATE record_chain SET origin = 'backfill' WHERE site_id = '`+site.site+`' AND seq = 2`, `ALTER TABLE record_chain ENABLE TRIGGER USER`)
	if p := part(); p.Status != chainproof.StatusValid {
		t.Fatalf("edit before the checkpoint: range %s %+v", p.Status, p.Break)
	}
	if f := full(); f.Break == nil || f.Break.Seq != 2 {
		t.Fatalf("edit at 2: full %+v", f.Break)
	}
}

func TestChainBundleTooLarge(t *testing.T) {
	pool := chainPool(t)
	rt := rolePool(t, "sp_runtime")
	newSigner(t)
	site, _ := seedAllKinds(t, pool, "too large")
	from, to := wideRange()
	_, err := exportOf(t, rt, site.site, from, to, 2)
	var big *chainproof.TooLargeError
	if !errors.As(err, &big) || big.Entries != 5 || big.Limit != 2 {
		t.Fatalf("err = %v, want TooLargeError{5, 2}", err)
	}
}

func TestChainEndpoints(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	pool := e.pool
	s, key := newSigner(t)
	site, ids := seedAllKinds(t, pool, "endpoint")
	ownerExec(t, pool, `UPDATE sites SET keycloak_realm = '`+realm1+`' WHERE id = '`+site.site+`'`)
	if _, err := s.SignSite(ctx, site.site, "hourly"); err != nil {
		t.Fatal(err)
	}
	day := time.Now().UTC().Format("2006-01-02")
	q := "?from=" + day + "&to=" + day
	base := "/api/v1/sites/" + site.site + "/chain/"
	admin := e.web(realm1, "chain-admin", "admin")

	r := e.expect(e.do("GET", base+"verify"+q+"&pin="+pubID(key), admin, "", nil), 200, "verify")
	if r.obj()["valid"] != true || r.str("status") != "valid" || r.str("trust") != "pinned" {
		t.Fatalf("verify = %s", r.raw)
	}
	if strings.Contains(string(r.raw), "description") || strings.Contains(string(r.raw), "gps_") {
		t.Fatalf("the verify result carries record content: %s", r.raw)
	}
	if r := e.expect(e.do("GET", base+"verify"+q, admin, "", nil), 200, "verify first use"); r.str("trust") != "first_use" {
		t.Fatalf("no pin: %s", r.raw)
	}
	if r := e.expect(e.do("GET", base+"verify"+q+"&pin="+strings.Repeat("0", 64), admin, "", nil), 200, "wrong pin"); r.obj()["valid"] != false ||
		r.obj()["first_break"].(map[string]any)["class"] != "head" {
		t.Fatalf("wrong fingerprint = %s", r.raw)
	}
	e.expect(e.do("GET", base+"verify"+q+"&pin=zz", admin, "", nil), 400, "bad pin")
	e.expect(e.do("GET", base+"verify?from=2026-01-01", admin, "", nil), 400, "range required")
	e.expect(e.do("GET", base+"verify?from=2026-01-01&to=2027-01-01", admin, "", nil), 400, "range too long")

	// Roles: ops and the client read the result; only ops export the records.
	for _, c := range []struct {
		role        string
		verify, exp int
	}{{"supervisor", 200, 200}, {"client-readonly", 200, 403}, {"hr", 403, 403}} {
		tok := e.web(realm1, "chain-"+c.role, c.role)
		e.expect(e.do("GET", base+"verify"+q, tok, "", nil), c.verify, c.role+" verify")
		e.expect(e.do("GET", base+"export"+q, tok, "", nil), c.exp, c.role+" export")
	}
	e.expect(e.do("GET", base+"verify"+q, "", "", nil), 401, "no token")
	e.expect(e.do("GET", base+"verify"+q, e.web(realm2, "other", "admin"), "", nil), 404, "other realm")

	x := e.expect(e.do("GET", base+"export"+q, admin, "", nil), 200, "export")
	got, err := chainproof.Read(x.raw)
	if err != nil {
		t.Fatal(err)
	}
	if res := chainproof.Verify(got, chainproof.Options{Pins: []string{pubID(key)}}); res.Status != chainproof.StatusValid {
		t.Fatalf("exported bundle: %s %+v", res.Status, res.Break)
	}

	// An edited record is reported with its seq, kind and id.
	ownerExec(t, pool, `UPDATE attendance_records SET status = 'absent' WHERE id = '`+ids["attendance"]+`'`)
	r = e.expect(e.do("GET", base+"verify"+q+"&pin="+pubID(key), admin, "", nil), 200, "verify after edit")
	fb, _ := r.obj()["first_break"].(map[string]any)
	if r.obj()["valid"] != false || fb["kind"] != "attendance" || fb["id"] != ids["attendance"] || fb["seq"] != float64(3) || fb["reason"] != "record modified" {
		t.Fatalf("verify after edit = %s", r.raw)
	}

	// Too large: a clear answer that points to the offline bundle.
	old := handler.ChainLimits
	t.Cleanup(func() { handler.ChainLimits = old })
	handler.ChainLimits.VerifyMaxEntries = 2
	r = e.expect(e.do("GET", base+"verify"+q, admin, "", nil), 400, "too large")
	if r.str("code") != "verify_too_large" || !strings.Contains(r.str("error"), "garda-verify") {
		t.Fatalf("too large = %s", r.raw)
	}
	handler.ChainLimits.VerifyMaxEntries = old.VerifyMaxEntries

	// Timeout: the database gives up (a lock held on the chain head table) and the answer is the same.
	handler.ChainLimits.VerifyTimeout = 300 * time.Millisecond
	lock, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lock.Rollback(ctx) }()
	if _, err := lock.Exec(ctx, `LOCK TABLE record_chain_heads IN ACCESS EXCLUSIVE MODE`); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	r = e.expect(e.do("GET", base+"verify"+q, admin, "", nil), 400, "timeout")
	if r.str("code") != "verify_too_large" || time.Since(start) > 10*time.Second {
		t.Fatalf("timeout = %s after %s", r.raw, time.Since(start))
	}
}
