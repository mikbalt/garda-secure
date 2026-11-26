//go:build integration

package integration

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/securepatrol/backend/internal/db"
	"github.com/securepatrol/backend/internal/repository"
	"github.com/securepatrol/backend/internal/signer"
)

// Signed chain heads (TAQ-56, D-071): the signer anchors each site's head
// outside the app, so a past rewritten and re-chained afterwards is detected.

func rolePool(t *testing.T, user string) *pgxpool.Pool {
	t.Helper()
	u := strings.Replace(dsn, "postgres:postgres@", user+":"+user+"-pw@", 1)
	pool, err := db.New(context.Background(), u, db.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func newKey(t *testing.T) ed25519.PrivateKey {
	t.Helper()
	_, k, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

// A signer with its own key, its own role, and a keyring starting from key.
// The keyring is install-wide, so each test gets a fresh one.
func newSigner(t *testing.T) (*signer.Signer, ed25519.PrivateKey) {
	t.Helper()
	resetKeyring(t)
	key := newKey(t)
	s, err := signer.New(rolePool(t, "sp_signer"), key, stateFile(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Bootstrap(context.Background()); err != nil {
		t.Fatal(err)
	}
	return s, key
}

// resetKeyring empties the (append-only) signer tables as the owner, like a fresh install.
func resetKeyring(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	pool := chainPool(t)
	for _, q := range []string{
		`ALTER TABLE signed_heads DISABLE TRIGGER USER`, `ALTER TABLE keyring DISABLE TRIGGER USER`,
		`DELETE FROM signed_heads`, `DELETE FROM keyring`,
		`ALTER TABLE signed_heads ENABLE TRIGGER USER`, `ALTER TABLE keyring ENABLE TRIGGER USER`,
	} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
}

func pubID(k ed25519.PrivateKey) string { return signer.KeyID(k.Public().(ed25519.PublicKey)) }

func seedChain(t *testing.T, pool *pgxpool.Pool, name string) (chainSite, string) {
	t.Helper()
	s := chainSeed(t, pool, name)
	s.scan(t, pool)
	inc := s.incident(t, pool)
	s.scan(t, pool)
	return s, inc
}

func wantHeadsOK(t *testing.T, pool *pgxpool.Pool, site string, pins ...string) {
	t.Helper()
	b, err := signer.VerifyHeads(context.Background(), pool, site, pins)
	if err != nil {
		t.Fatal(err)
	}
	if b != nil {
		t.Fatalf("signed heads should verify, got break at seq %d: %s", b.Seq, b.Reason)
	}
}

func TestSignerSignsAndStoresHead(t *testing.T) {
	ctx := context.Background()
	pool := chainPool(t)
	s, key := newSigner(t)
	site, _ := seedChain(t, pool, "sig basic")

	h, err := s.SignSite(ctx, site.site, "hourly")
	if err != nil {
		t.Fatal(err)
	}
	if h.Seq != 3 || h.KeyID != pubID(key) {
		t.Fatalf("signed head = seq %d key %s, want seq 3 key %s", h.Seq, h.KeyID, pubID(key))
	}
	var n int
	var hash []byte
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM signed_heads WHERE site_id = $1`, site.site).Scan(&n); err != nil || n != 1 {
		t.Fatalf("stored heads = %d (%v), want 1", n, err)
	}
	_ = pool.QueryRow(ctx, `SELECT entry_hash FROM signed_heads WHERE site_id = $1`, site.site).Scan(&hash)
	var head []byte
	_ = pool.QueryRow(ctx, `SELECT last_hash FROM record_chain_heads WHERE site_id = $1`, site.site).Scan(&head)
	if string(hash) != string(head) {
		t.Fatal("signed entry_hash is not the chain head")
	}
	wantHeadsOK(t, pool, site.site)

	// Signing the same head again is a no-op; a new entry gets a new head.
	again, err := s.SignSite(ctx, site.site, "on_demand")
	if err != nil || again.ID != h.ID {
		t.Fatalf("re-sign of an unchanged head: %v %v, want the existing row", again, err)
	}
	site.scan(t, pool)
	h2, err := s.SignSite(ctx, site.site, "day_end")
	if err != nil || h2.Seq != 4 {
		t.Fatalf("new head: %v %v", h2, err)
	}
	wantHeadsOK(t, pool, site.site)
}

func TestSignerRefusesLowerSeq(t *testing.T) {
	ctx := context.Background()
	pool := chainPool(t)
	s, _ := newSigner(t)
	site, _ := seedChain(t, pool, "sig lower")
	if _, err := s.SignSite(ctx, site.site, "hourly"); err != nil {
		t.Fatal(err)
	}
	// The newest entry (and the head) is dropped outside the app.
	dropNewestEntry(t, pool, site.site)
	if _, err := s.SignSite(ctx, site.site, "hourly"); !errors.Is(err, signer.ErrLowerSeq) {
		t.Fatalf("lower head: err = %v, want ErrLowerSeq", err)
	}
	// The table itself refuses a lower seq from the signer role.
	_, err := rolePool(t, "sp_signer").Exec(ctx, `INSERT INTO signed_heads (site_id, seq, entry_hash, signed_at, reason, key_id, signature)
		SELECT site_id, 1, entry_hash, now(), 'hourly', key_id, signature FROM signed_heads WHERE site_id = $1`, site.site)
	if err == nil {
		t.Fatal("signed_heads accepted a lower seq")
	}
}

func dropNewestEntry(t *testing.T, pool *pgxpool.Pool, site string) {
	t.Helper()
	ctx := context.Background()
	for _, q := range []string{
		`ALTER TABLE record_chain DISABLE TRIGGER USER`, `ALTER TABLE record_chain_heads DISABLE TRIGGER USER`,
		`DELETE FROM record_chain WHERE site_id = $1 AND seq = (SELECT max(seq) FROM record_chain WHERE site_id = $1)`,
		`UPDATE record_chain_heads h SET last_seq = c.seq, last_hash = c.entry_hash FROM record_chain c
		  WHERE h.site_id = $1 AND c.site_id = h.site_id AND c.seq = (SELECT max(seq) FROM record_chain WHERE site_id = $1)`,
		`ALTER TABLE record_chain ENABLE TRIGGER USER`, `ALTER TABLE record_chain_heads ENABLE TRIGGER USER`,
	} {
		var args []any
		if strings.Contains(q, "$1") {
			args = []any{site}
		}
		if _, err := pool.Exec(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSignerRefusesConflictingHashAtSignedSeq(t *testing.T) {
	ctx := context.Background()
	pool := chainPool(t)
	s, _ := newSigner(t)
	site, _ := seedChain(t, pool, "sig conflict")
	h, err := s.SignSite(ctx, site.site, "hourly")
	if err != nil {
		t.Fatal(err)
	}
	// Entry 3 is replaced by a different one with the same seq (head follows).
	for _, q := range []string{
		`ALTER TABLE record_chain DISABLE TRIGGER USER`, `ALTER TABLE record_chain_heads DISABLE TRIGGER USER`,
		`UPDATE record_chain SET entry_hash = '\xdeadbeef' WHERE site_id = $1 AND seq = 3`,
		`UPDATE record_chain_heads SET last_hash = '\xdeadbeef' WHERE site_id = $1`,
		`ALTER TABLE record_chain ENABLE TRIGGER USER`, `ALTER TABLE record_chain_heads ENABLE TRIGGER USER`,
	} {
		var args []any
		if strings.Contains(q, "$1") {
			args = []any{site.site}
		}
		if _, err := pool.Exec(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.SignSite(ctx, site.site, "hourly"); !errors.Is(err, signer.ErrConflict) {
		t.Fatalf("conflicting head: err = %v, want ErrConflict", err)
	}
	// The table refuses it too, whatever the signer process does.
	_, err = rolePool(t, "sp_signer").Exec(ctx, `INSERT INTO signed_heads (site_id, seq, entry_hash, signed_at, reason, key_id, signature)
		VALUES ($1, 3, decode(repeat('ab', 32), 'hex'), now(), 'hourly', $2, $3)`, site.site, h.KeyID, h.Signature)
	if err == nil {
		t.Fatal("signed_heads accepted a second hash for a signed seq")
	}
}

// The attack of D-069: the past is edited, then the whole chain is rebuilt so
// chain_first_break is satisfied. Only the signed head still disagrees.
func TestSignedHeadsDetectRewrittenPast(t *testing.T) {
	ctx := context.Background()
	pool := chainPool(t)
	s, _ := newSigner(t)
	site, inc := seedChain(t, pool, "sig rewrite")
	if _, err := s.SignSite(ctx, site.site, "hourly"); err != nil {
		t.Fatal(err)
	}
	wantHeadsOK(t, pool, site.site)

	rewriteAndRechain(t, pool, site.site, inc)
	site.scan(t, pool) // life goes on, the chain is longer than the signed head
	wantValid(t, pool, site.site)

	b, err := signer.VerifyHeads(ctx, pool, site.site, nil)
	if err != nil {
		t.Fatal(err)
	}
	if b == nil || b.Seq != 3 {
		t.Fatalf("rewritten past not detected at the signed seq: %+v", b)
	}
	// The chain verify of the app includes the signature check.
	if cb, err := repository.VerifyChain(ctx, pool, site.site, nil, signer.VerifyOptions{}); err != nil || cb == nil || cb.Seq != 3 {
		t.Fatalf("VerifyChain after a re-chained rewrite: %+v %v, want a break at seq 3", cb, err)
	}
	// The signer refuses to anchor the rewritten chain as well.
	if _, err := s.SignSite(ctx, site.site, "hourly"); !errors.Is(err, signer.ErrPastRewritten) {
		t.Fatalf("sign after rewrite: err = %v, want ErrPastRewritten", err)
	}
}

// Edit incident `inc`, drop the site's chain and head, and chain every record again.
func rewriteAndRechain(t *testing.T, pool *pgxpool.Pool, site, inc string) {
	t.Helper()
	ctx := context.Background()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{sql: `ALTER TABLE record_chain DISABLE TRIGGER USER`}, {sql: `ALTER TABLE record_chain_heads DISABLE TRIGGER USER`},
		{`UPDATE incidents SET description = 'nothing happened' WHERE id = $1`, []any{inc}},
		{`DELETE FROM record_chain WHERE site_id = $1`, []any{site}}, {`DELETE FROM record_chain_heads WHERE site_id = $1`, []any{site}},
		{`SELECT chain_append(kind, record_id, 'live') FROM (
		   SELECT * FROM (
		     SELECT 'scan' AS kind, s.id AS record_id, 1 AS o, s.received_at AS ts FROM checkpoint_scans s JOIN routes r ON r.id = s.route_id WHERE r.site_id = $1
		     UNION ALL SELECT 'incident', id, 2, created_at FROM incidents WHERE site_id = $1) u ORDER BY o, ts OFFSET 0) x`, []any{site}},
		{sql: `ALTER TABLE record_chain ENABLE TRIGGER USER`}, {sql: `ALTER TABLE record_chain_heads ENABLE TRIGGER USER`},
	} {
		if _, err := pool.Exec(ctx, q.sql, q.args...); err != nil {
			t.Fatalf("%s: %v", q.sql, err)
		}
	}
}

func TestSignerKeyRotationAndHandover(t *testing.T) {
	ctx := context.Background()
	pool := chainPool(t)
	sA, keyA := newSigner(t)
	site, _ := seedChain(t, pool, "sig rotate")
	if _, err := sA.SignSite(ctx, site.site, "hourly"); err != nil {
		t.Fatal(err)
	}
	keyB := newKey(t)
	sigPool := rolePool(t, "sp_signer")

	// The handover needs the current key: a stranger's old key is refused.
	if err := signer.Rotate(ctx, sigPool, newKey(t), keyB, time.Now()); err == nil {
		t.Fatal("rotation accepted an old key that is not the current one")
	}
	if err := signer.Rotate(ctx, sigPool, keyA, keyB, time.Now()); err != nil {
		t.Fatal(err)
	}
	sB, _ := signer.New(sigPool, keyB, stateFile(t))
	if err := sB.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	site.scan(t, pool)
	h, err := sB.SignSite(ctx, site.site, "hourly")
	if err != nil || h.KeyID != pubID(keyB) {
		t.Fatalf("sign with the new key: %v %v", h, err)
	}
	// Only the first key is pinned: the rotation vouches for the second, and the old head still verifies.
	wantHeadsOK(t, pool, site.site, pubID(keyA))
	// A head signed by a key that is not in the keyring never verifies; the old key stays valid for its heads.
	if err := func() *signer.Signer { x, _ := signer.New(sigPool, newKey(t), stateFile(t)); return x }().Bootstrap(ctx); !errors.Is(err, signer.ErrKeyNotInRing) {
		t.Fatalf("unknown key bootstrap: %v, want ErrKeyNotInRing", err)
	}
}

func TestSignerCompromisedKeyStartsNewEpoch(t *testing.T) {
	ctx := context.Background()
	pool := chainPool(t)
	sA, keyA := newSigner(t)
	site, _ := seedChain(t, pool, "sig epoch")
	if _, err := sA.SignSite(ctx, site.site, "hourly"); err != nil {
		t.Fatal(err)
	}
	keyC := newKey(t)
	sigPool := rolePool(t, "sp_signer")
	if err := signer.NewEpoch(ctx, sigPool, keyC, "key A leaked", time.Now()); err != nil {
		t.Fatal(err)
	}
	sC, _ := signer.New(sigPool, keyC, stateFile(t))
	if err := sC.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	site.scan(t, pool)
	if _, err := sC.SignSite(ctx, site.site, "hourly"); err != nil {
		t.Fatal(err)
	}
	var epoch int
	if err := pool.QueryRow(ctx, `SELECT epoch FROM keyring WHERE key_id = $1`, pubID(keyC)).Scan(&epoch); err != nil || epoch != 2 {
		t.Fatalf("new key epoch = %d (%v), want 2", epoch, err)
	}
	// A new epoch is not vouched for by the old key: until the client re-pins it, its heads are untrusted.
	b, err := signer.VerifyHeads(ctx, pool, site.site, []string{pubID(keyA)})
	if err != nil || b == nil || b.Seq != 4 {
		t.Fatalf("epoch 2 head without re-pin: %+v %v, want a break at seq 4", b, err)
	}
	wantHeadsOK(t, pool, site.site, pubID(keyA), pubID(keyC))
	// Whoever has the leaked key cannot add a key of epoch 1 afterwards.
	if err := signer.Rotate(ctx, sigPool, keyA, newKey(t), time.Now()); err == nil {
		t.Fatal("the compromised key could still rotate after a new epoch started")
	}
}

func TestRolesAfterHardening(t *testing.T) {
	ctx := context.Background()
	owner := chainPool(t)
	rt := rolePool(t, "sp_runtime")
	sg := rolePool(t, "sp_signer")
	s, _ := seedChain(t, owner, "sig roles")
	_ = s

	mustFail := func(p *pgxpool.Pool, who, sql string) {
		t.Helper()
		if _, err := p.Exec(ctx, sql); err == nil {
			t.Errorf("%s: %q succeeded, want permission denied", who, sql)
		}
	}
	// The runtime role keeps what the app needs: normal writes still chain (chain_append is
	// executable on purpose, D-071) ...
	site := chainSeed(t, owner, "sig roles rt")
	site.scan(t, rt)
	if n := chainLen(t, owner, site.site); n != 1 {
		t.Fatalf("a write through the runtime role chained %d entries, want 1", n)
	}
	wantValid(t, rt, site.site)
	// ... but it cannot reach the chain tables, the hash helper or the triggers directly.
	for _, q := range []string{
		`SELECT chain_entry_hash(gen_random_uuid(), 1, 'scan', gen_random_uuid(), '\x00', '\x00', 1::smallint, 'live', now())`,
		`INSERT INTO record_chain (site_id, seq, kind, record_id, record_hash, prev_hash, entry_hash) VALUES (gen_random_uuid(), 1, 'scan', gen_random_uuid(), '\x00', '\x00', '\x00')`,
		`UPDATE record_chain_heads SET last_seq = last_seq + 1`,
		`DELETE FROM record_chain`,
		`ALTER TABLE record_chain DISABLE TRIGGER USER`,
		`ALTER TABLE incidents DISABLE TRIGGER USER`,
		`INSERT INTO signed_heads (site_id, seq, entry_hash, signed_at, reason, key_id, signature) VALUES (gen_random_uuid(), 1, '\x00', now(), 'hourly', 'x', '\x00')`,
		`INSERT INTO keyring (key_id, epoch, kind, public_key, new_key_sig) VALUES ('x', 1, 'genesis', '\x00', '\x00')`,
	} {
		mustFail(rt, "runtime", q)
	}
	// The signer reads chain tables and writes only its own tables.
	if _, err := sg.Exec(ctx, `SELECT count(*) FROM record_chain`); err != nil {
		t.Errorf("signer cannot read the chain: %v", err)
	}
	for _, q := range []string{
		`SELECT count(*) FROM attendance_records`, `SELECT count(*) FROM incidents`, `UPDATE sites SET name = name`,
		`SELECT chain_append('scan', gen_random_uuid())`,
		`DELETE FROM record_chain`, `UPDATE record_chain_heads SET last_seq = 0`,
		`UPDATE signed_heads SET reason = 'x'`, `DELETE FROM signed_heads`, `DELETE FROM keyring`,
	} {
		mustFail(sg, "signer", q)
	}
	// The app can read signed heads (to verify them) but not write them.
	if _, err := rt.Exec(ctx, `SELECT count(*) FROM signed_heads`); err != nil {
		t.Errorf("runtime cannot read signed_heads: %v", err)
	}
	// Tables added by later migrations are reachable by the runtime role without extra grants.
	if _, err := owner.Exec(ctx, `CREATE TABLE zz_default_priv_probe (id int)`); err != nil {
		t.Fatal(err)
	}
	if _, err := rt.Exec(ctx, `INSERT INTO zz_default_priv_probe VALUES (1)`); err != nil {
		t.Errorf("default privileges missing for the runtime role: %v", err)
	}
	_, _ = owner.Exec(ctx, `DROP TABLE zz_default_priv_probe`)
}

// ── Review fixes (PR #66): the owner role must not be able to make the signer forget ──

func stateFile(t *testing.T) string { return filepath.Join(t.TempDir(), "signed-heads.jsonl") }

func newStateSigner(t *testing.T) (*signer.Signer, ed25519.PrivateKey, string) {
	t.Helper()
	resetKeyring(t)
	key := newKey(t)
	state := stateFile(t)
	s, err := signer.New(rolePool(t, "sp_signer"), key, state)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Bootstrap(context.Background()); err != nil {
		t.Fatal(err)
	}
	return s, key, state
}

func wipeSignedHeads(t *testing.T, pool *pgxpool.Pool, site string) {
	t.Helper()
	ctx := context.Background()
	for _, q := range []string{`ALTER TABLE signed_heads DISABLE TRIGGER USER`, `DELETE FROM signed_heads WHERE site_id = '` + site + `'`, `ALTER TABLE signed_heads ENABLE TRIGGER USER`} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
}

// The owner wipes signed_heads (its memory in the database), rewrites the past and
// re-chains it. The signer must still refuse, from its own record on its volume.
func TestSignerRemembersWhenTheOwnerWipesSignedHeads(t *testing.T) {
	ctx := context.Background()
	pool := chainPool(t)
	s, _, state := newStateSigner(t)
	site, inc := seedChain(t, pool, "sig wipe rewrite")
	if _, err := s.SignSite(ctx, site.site, "hourly"); err != nil {
		t.Fatal(err)
	}
	wipeSignedHeads(t, pool, site.site)
	rewriteAndRechain(t, pool, site.site, inc)
	site.scan(t, pool)
	wantValid(t, pool, site.site) // the chain itself looks intact

	// 1. It refuses to sign and says why: heads it has on disk are missing from the database.
	if _, err := s.SignSite(ctx, site.site, "hourly"); !errors.Is(err, signer.ErrHeadsLost) {
		t.Fatalf("after the wipe: err = %v, want ErrHeadsLost", err)
	}
	if n := countHeads(t, pool, site.site); n != 0 {
		t.Fatalf("the signer stored %d heads while refusing", n)
	}
	// 2. Verify, given the signer's own record, reports the rewritten past.
	local, err := signer.LoadHighWater(state)
	if err != nil {
		t.Fatal(err)
	}
	b, err := signer.Verify(ctx, pool, site.site, nil, signer.VerifyOptions{Local: local.Heads()})
	if err != nil || b == nil || b.Seq != 3 {
		t.Fatalf("verify with the signer's record: %+v %v, want a break at seq 3", b, err)
	}
	// 3. Even once the lost heads are restored, the rewritten past is not anchored.
	if n, err := s.RestoreHeads(ctx); err != nil || n != 1 {
		t.Fatalf("restore: %d %v, want 1", n, err)
	}
	if _, err := s.SignSite(ctx, site.site, "hourly"); !errors.Is(err, signer.ErrPastRewritten) {
		t.Fatalf("after restore: err = %v, want ErrPastRewritten", err)
	}
}

// No database memory at all and a shorter chain: the on-disk record still refuses a lower seq.
func TestSignerRefusesLowerSeqFromItsOwnRecord(t *testing.T) {
	ctx := context.Background()
	pool := chainPool(t)
	s, _, _ := newStateSigner(t)
	site, _ := seedChain(t, pool, "sig wipe lower")
	if _, err := s.SignSite(ctx, site.site, "hourly"); err != nil {
		t.Fatal(err)
	}
	wipeSignedHeads(t, pool, site.site)
	dropNewestEntry(t, pool, site.site)
	if _, err := s.RestoreHeads(ctx); err != nil {
		t.Fatal(err)
	}
	wipeSignedHeads(t, pool, site.site) // and wiped again right before the next tick
	if _, err := s.SignSite(ctx, site.site, "hourly"); !errors.Is(err, signer.ErrHeadsLost) {
		t.Fatalf("err = %v, want ErrHeadsLost", err)
	}
	if _, err := s.RestoreHeads(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SignSite(ctx, site.site, "hourly"); !errors.Is(err, signer.ErrLowerSeq) {
		t.Fatalf("err = %v, want ErrLowerSeq", err)
	}
}

func countHeads(t *testing.T, pool *pgxpool.Pool, site string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM signed_heads WHERE site_id = $1`, site).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// A crash between the disk write and the database insert is repaired, not blocked.
func TestSignerRepairsAHeadWrittenToDiskButNotStored(t *testing.T) {
	ctx := context.Background()
	pool := chainPool(t)
	s, _, _ := newStateSigner(t)
	site, _ := seedChain(t, pool, "sig crash gap")
	h, err := s.SignSite(ctx, site.site, "hourly")
	if err != nil {
		t.Fatal(err)
	}
	wipeSignedHeads(t, pool, site.site) // as if the insert never happened; the chain is untouched
	site.scan(t, pool)
	h2, err := s.SignSite(ctx, site.site, "hourly")
	if err != nil || h2.Seq != 4 {
		t.Fatalf("sign after a crash gap: %v %v", h2, err)
	}
	if n := countHeads(t, pool, site.site); n != 2 {
		t.Fatalf("heads after repair = %d (first was seq %d), want 2", n, h.Seq)
	}
	wantHeadsOK(t, pool, site.site)
}

func TestSignerExportsHeadsForOfflineAnchoring(t *testing.T) {
	ctx := context.Background()
	pool := chainPool(t)
	s, key, _ := newStateSigner(t)
	site, _ := seedChain(t, pool, "sig export")
	h, err := s.SignSite(ctx, site.site, "day_end")
	if err != nil {
		t.Fatal(err)
	}
	wipeSignedHeads(t, pool, site.site) // the export does not depend on the database
	var buf bytes.Buffer
	if err := s.Export(ctx, &buf, ""); err != nil {
		t.Fatal(err)
	}
	var out struct {
		Heads []struct {
			SiteID    string `json:"site_id"`
			EntryHash string `json:"entry_hash"`
			SignedAt  string `json:"signed_at"`
			KeyID     string `json:"key_id"`
			Signature string `json:"signature"`
			Seq       int64  `json:"seq"`
		} `json:"heads"`
		Keyring []struct {
			KeyID     string `json:"key_id"`
			PublicKey string `json:"public_key"`
		} `json:"keyring"`
	}
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("export is not JSON: %v\n%s", err, buf.String())
	}
	if len(out.Heads) != 1 || out.Heads[0].Seq != h.Seq || out.Heads[0].KeyID != pubID(key) || len(out.Keyring) != 1 {
		t.Fatalf("export = %+v", out)
	}
	hash, _ := hex.DecodeString(out.Heads[0].EntryHash)
	sig, _ := hex.DecodeString(out.Heads[0].Signature)
	at, _ := time.Parse(time.RFC3339Nano, out.Heads[0].SignedAt)
	if !ed25519.Verify(key.Public().(ed25519.PublicKey), signer.HeadPayload(out.Heads[0].SiteID, h.Seq, hash, at, pubID(key)), sig) {
		t.Fatal("an exported head does not verify with the public key")
	}
}

// A second handover from the same key is a fork: neither successor is trusted.
func TestVerifyRejectsAForkedHandover(t *testing.T) {
	ctx := context.Background()
	pool := chainPool(t)
	sA, keyA, _ := newStateSigner(t)
	site, _ := seedChain(t, pool, "sig fork")
	if _, err := sA.SignSite(ctx, site.site, "hourly"); err != nil {
		t.Fatal(err)
	}
	keyB, keyC := newKey(t), newKey(t)
	sigPool := rolePool(t, "sp_signer")
	if err := signer.Rotate(ctx, sigPool, keyA, keyB, time.Now()); err != nil {
		t.Fatal(err)
	}
	sB, err := signer.New(sigPool, keyB, stateFile(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := sB.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	site.scan(t, pool)
	if _, err := sB.SignSite(ctx, site.site, "hourly"); err != nil {
		t.Fatal(err)
	}
	wantHeadsOK(t, pool, site.site, pubID(keyA))
	// The old key (leaked) vouches for a second successor; the owner inserts it into the keyring.
	insertForkedRotation(t, pool, keyA, keyC)
	b, err := signer.VerifyHeads(ctx, pool, site.site, []string{pubID(keyA)})
	if err != nil || b == nil || !strings.Contains(b.Reason, "fork") {
		t.Fatalf("forked handover: %+v %v, want a break mentioning the fork", b, err)
	}
	// A key reachable from nothing pinned is refused even when it has a valid self-signature.
	b, err = signer.VerifyHeads(ctx, pool, site.site, []string{pubID(newKey(t))})
	if err != nil || b == nil {
		t.Fatalf("unpinned root: %+v %v, want a break", b, err)
	}
}

func insertForkedRotation(t *testing.T, pool *pgxpool.Pool, oldKey, newKey ed25519.PrivateKey) {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Microsecond)
	pub := newKey.Public().(ed25519.PublicKey)
	id, prev := signer.KeyID(pub), pubID(oldKey)
	payload := signer.KeyPayload(signer.KindRotation, 1, id, pub, prev, now)
	if _, err := pool.Exec(context.Background(), `INSERT INTO keyring (key_id, epoch, kind, public_key, prev_key_id, created_at, new_key_sig, prev_key_sig)
		VALUES ($1, 1, 'rotation', $2, $3, $4, $5, $6)`, id, []byte(pub), prev, now, ed25519.Sign(newKey, payload), ed25519.Sign(oldKey, payload)); err != nil {
		t.Fatal(err)
	}
}

func TestSignerCanonicalisesTheSiteID(t *testing.T) {
	ctx := context.Background()
	pool := chainPool(t)
	s, _, _ := newStateSigner(t)
	site, _ := seedChain(t, pool, "sig upper")
	upper := strings.ToUpper(site.site)
	h, err := s.SignSite(ctx, upper, "on_demand")
	if err != nil {
		t.Fatal(err)
	}
	if h.SiteID != site.site {
		t.Fatalf("stored site id %q, want the lowercase %q", h.SiteID, site.site)
	}
	for _, id := range []string{upper, site.site} {
		wantHeadsOK(t, pool, id)
	}
	for _, bad := range []string{"", "not-a-uuid", site.site + "x", "{" + site.site + "}", strings.ReplaceAll(site.site, "-", "")} {
		if _, err := s.SignSite(ctx, bad, "on_demand"); err == nil {
			t.Errorf("SignSite accepted site id %q", bad)
		}
	}
	// The table refuses shapes that cannot be a real signature or key id.
	for _, q := range []string{
		`INSERT INTO signed_heads (site_id, seq, entry_hash, signed_at, reason, key_id, signature) SELECT gen_random_uuid(), 1, '\x00', now(), 'hourly', key_id, signature FROM signed_heads LIMIT 1`,
		`INSERT INTO signed_heads (site_id, seq, entry_hash, signed_at, reason, key_id, signature) SELECT gen_random_uuid(), 1, entry_hash, now(), 'hourly', key_id, '\x00' FROM signed_heads LIMIT 1`,
	} {
		if _, err := rolePool(t, "sp_signer").Exec(ctx, q); err == nil {
			t.Errorf("signed_heads accepted a malformed row: %s", q)
		}
	}
}

func TestVerifyReportsUnsignedAndStale(t *testing.T) {
	ctx := context.Background()
	pool := chainPool(t)
	s, _, _ := newStateSigner(t)
	site, _ := seedChain(t, pool, "sig unsigned")

	// A chain with entries and no signed head is not "fine".
	b, err := signer.VerifyHeads(ctx, pool, site.site, nil)
	if err != nil || b == nil || b.Status != signer.StatusUnsigned {
		t.Fatalf("unsigned chain: %+v %v, want status UNSIGNED", b, err)
	}
	// An empty chain has nothing to sign.
	empty := chainSeed(t, pool, "sig empty")
	if b, err := signer.VerifyHeads(ctx, pool, empty.site, nil); err != nil || b != nil {
		t.Fatalf("empty chain: %+v %v, want ok", b, err)
	}

	if _, err := s.SignSite(ctx, site.site, "hourly"); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(3 * time.Hour)
	opts := signer.VerifyOptions{MaxAge: 2 * time.Hour, Now: later}
	// Quiet site: the newest head still covers the whole chain, however old it is.
	if b, err := signer.Verify(ctx, pool, site.site, nil, opts); err != nil || b != nil {
		t.Fatalf("quiet site: %+v %v, want ok", b, err)
	}
	// Entries after the newest head, and that head older than the limit: stale.
	site.scan(t, pool)
	b, err = signer.Verify(ctx, pool, site.site, nil, opts)
	if err != nil || b == nil || b.Status != signer.StatusStale {
		t.Fatalf("stale: %+v %v, want status STALE", b, err)
	}
	if b, err := signer.Verify(ctx, pool, site.site, nil, signer.VerifyOptions{MaxAge: 2 * time.Hour}); err != nil || b != nil {
		t.Fatalf("fresh head within the limit: %+v %v, want ok", b, err)
	}
}

// The signer crashed after the disk write and before the commit, and the site has had no
// new entry since: the repair must be committed, not rolled back with the early return.
func TestSignerRepairsAQuietSiteAfterACrash(t *testing.T) {
	ctx := context.Background()
	pool := chainPool(t)
	s, _, state := newStateSigner(t)
	site, _ := seedChain(t, pool, "sig crash quiet")
	h, err := s.SignSite(ctx, site.site, "hourly")
	if err != nil {
		t.Fatal(err)
	}
	wipeSignedHeads(t, pool, site.site) // the insert never committed; the chain has not moved

	// On-demand signing returns the head and the repair sticks.
	h2, err := s.SignSite(ctx, site.site, "on_demand")
	if err != nil || h2.Seq != h.Seq {
		t.Fatalf("on-demand sign on a quiet site after a crash: %v %v", h2, err)
	}
	if n := countHeads(t, pool, site.site); n != 1 {
		t.Fatalf("heads in the database after the repair = %d, want 1 (the repair was rolled back)", n)
	}
	local, _ := signer.LoadHighWater(state)
	if b, err := signer.Verify(ctx, pool, site.site, nil, signer.VerifyOptions{Local: local.Heads()}); err != nil || b != nil {
		t.Fatalf("verify after the repair: %+v %v", b, err)
	}

	// Same through the periodic tick, for a second site.
	site2, _ := seedChain(t, pool, "sig crash quiet 2")
	if _, err := s.SignSite(ctx, site2.site, "hourly"); err != nil {
		t.Fatal(err)
	}
	wipeSignedHeads(t, pool, site2.site)
	_, _ = s.SignDue(ctx) // other tests leave broken chains in the shared database; only this site matters
	if n := countHeads(t, pool, site2.site); n != 1 {
		t.Fatalf("heads after a tick = %d, want 1", n)
	}
}
