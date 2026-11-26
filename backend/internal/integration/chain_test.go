//go:build integration

package integration

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/securepatrol/backend/internal/db"
	"github.com/securepatrol/backend/internal/model"
	"github.com/securepatrol/backend/internal/repository"
)

// Hash-chained record log (TAQ-17): attendance, scans and incidents of a site
// are chained; any edit or deletion is found by repository.FirstChainBreak.

type chainSite struct{ site, guard, route, session string }

func chainSeed(t *testing.T, pool *pgxpool.Pool, name string) chainSite {
	t.Helper()
	ctx := context.Background()
	var s chainSite
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(pool.QueryRow(ctx, `INSERT INTO sites (name, keycloak_realm) VALUES ($1, 'chain') RETURNING id`, name).Scan(&s.site))
	must(pool.QueryRow(ctx, `INSERT INTO guards (name, role, site_id) VALUES ('G', 'guard', $1) RETURNING id`, s.site).Scan(&s.guard))
	must(pool.QueryRow(ctx, `INSERT INTO routes (site_id, name) VALUES ($1, 'r') RETURNING id`, s.site).Scan(&s.route))
	must(pool.QueryRow(ctx, `INSERT INTO patrol_sessions (route_id, site_id, guard_id, started_at) VALUES ($1, $2, $3, now()) RETURNING id`,
		s.route, s.site, s.guard).Scan(&s.session))
	return s
}

func chainPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, err := db.New(context.Background(), dsn, db.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func (s chainSite) scan(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	sc, _, err := repository.InsertScan(context.Background(), pool, model.CheckpointScan{
		SessionID: &s.session, RouteID: s.route, CheckpointID: "cp1", GuardID: s.guard,
		ScannedAt: time.Now(), Method: "qr",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return sc.ID
}

func (s chainSite) incident(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	i, _, err := repository.CreateIncident(context.Background(), pool, model.Incident{
		SiteID: s.site, GuardID: s.guard, Type: "theft", Severity: "low", OccurredAt: time.Now(),
		PhotoObjectKeys: []string{},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return i.ID
}

func (s chainSite) attendance(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	ctx := context.Background()
	var shift string
	if err := pool.QueryRow(ctx, `INSERT INTO shifts (guard_id, site_id, start_time, end_time)
		VALUES ($1, $2, now(), now() + interval '8 hours') RETURNING id`, s.guard, s.site).Scan(&shift); err != nil {
		t.Fatal(err)
	}
	r, err := repository.InsertClockIn(ctx, pool, repository.ClockInParams{
		ShiftID: shift, GuardID: s.guard, SiteID: s.site, ClockInAt: time.Now(), Status: "present", ReceivedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return r.ID
}

func chainLen(t *testing.T, pool *pgxpool.Pool, site string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM record_chain WHERE site_id = $1`, site).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func wantValid(t *testing.T, pool *pgxpool.Pool, site string) {
	t.Helper()
	b, err := repository.FirstChainBreak(context.Background(), pool, site)
	if err != nil {
		t.Fatal(err)
	}
	if b != nil {
		t.Fatalf("chain should be valid, first break: seq %d (%s)", b.Seq, b.Reason)
	}
}

func wantBreakAt(t *testing.T, pool *pgxpool.Pool, site string, seq int64) {
	t.Helper()
	b, err := repository.FirstChainBreak(context.Background(), pool, site)
	if err != nil {
		t.Fatal(err)
	}
	if b == nil {
		t.Fatalf("tampering not detected, want break at seq %d", seq)
	}
	if b.Seq != seq {
		t.Fatalf("first break at seq %d (%s), want %d", b.Seq, b.Reason, seq)
	}
}

func TestChainLinksPerSite(t *testing.T) {
	pool := chainPool(t)
	a, b := chainSeed(t, pool, "chain A"), chainSeed(t, pool, "chain B")
	wantValid(t, pool, a.site) // empty chain

	a.attendance(t, pool)
	a.scan(t, pool)
	a.incident(t, pool)
	b.scan(t, pool)

	if n := chainLen(t, pool, a.site); n != 3 {
		t.Fatalf("site A chain length = %d, want 3", n)
	}
	if n := chainLen(t, pool, b.site); n != 1 {
		t.Fatalf("site B chain length = %d, want 1 (chains are per site)", n)
	}
	// Each entry's prev_hash is the previous entry's hash; the first links to the genesis value.
	var linked int
	if err := pool.QueryRow(context.Background(), `
		SELECT count(*) FROM record_chain c
		JOIN record_chain p ON p.site_id = c.site_id AND p.seq = c.seq - 1
		WHERE c.site_id = $1 AND c.prev_hash = p.entry_hash`, a.site).Scan(&linked); err != nil {
		t.Fatal(err)
	}
	if linked != 2 {
		t.Fatalf("linked entries = %d, want 2", linked)
	}
	wantValid(t, pool, a.site)
	wantValid(t, pool, b.site)
}

func TestChainLegitimateChangesStayValid(t *testing.T) {
	pool := chainPool(t)
	ctx := context.Background()
	s := chainSeed(t, pool, "chain updates")
	att := s.attendance(t, pool)
	inc := s.incident(t, pool)
	sc := s.scan(t, pool)

	if _, err := repository.SetClockOut(ctx, pool, att, time.Now(), 0, nil, nil, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.UpdateIncidentStatus(ctx, pool, inc, "resolved"); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.CreateAmendment(ctx, pool, inc, []byte(`{"severity":"high"}`), "typo", "admin"); err != nil {
		t.Fatal(err)
	}
	if err := repository.ClearScanPhoto(ctx, pool, sc); err != nil {
		t.Fatal(err)
	}
	// 3 creates + clock-out + status + amendment + photo clear: every change is a new entry.
	if n := chainLen(t, pool, s.site); n != 7 {
		t.Fatalf("chain length = %d, want 7", n)
	}
	wantValid(t, pool, s.site)
}

func TestChainDetectsEditedRecord(t *testing.T) {
	pool := chainPool(t)
	ctx := context.Background()
	s := chainSeed(t, pool, "chain edit")
	s.scan(t, pool)
	inc := s.incident(t, pool)
	s.scan(t, pool)
	wantValid(t, pool, s.site)

	// Edit outside the application: the incident (seq 2) is rewritten in place.
	if _, err := pool.Exec(ctx, `UPDATE incidents SET description = 'nothing happened' WHERE id = $1`, inc); err != nil {
		t.Fatal(err)
	}
	wantBreakAt(t, pool, s.site, 2)
}

func TestChainDetectsEditedChainEntry(t *testing.T) {
	pool := chainPool(t)
	ctx := context.Background()
	s := chainSeed(t, pool, "chain edit entry")
	s.scan(t, pool)
	s.scan(t, pool)
	s.scan(t, pool)
	withoutGuard(t, pool, "record_chain", func() {
		if _, err := pool.Exec(ctx, `UPDATE record_chain SET record_hash = sha256('x'::bytea) WHERE site_id = $1 AND seq = 2`, s.site); err != nil {
			t.Fatal(err)
		}
	})
	wantBreakAt(t, pool, s.site, 2)
}

func TestChainDetectsDeletedRecord(t *testing.T) {
	pool := chainPool(t)
	ctx := context.Background()
	s := chainSeed(t, pool, "chain delete record")
	s.scan(t, pool)
	doomed := s.scan(t, pool)
	s.scan(t, pool)
	if _, err := pool.Exec(ctx, `DELETE FROM checkpoint_scans WHERE id = $1`, doomed); err != nil {
		t.Fatal(err)
	}
	wantBreakAt(t, pool, s.site, 2)
}

func TestChainDetectsDeletedChainEntry(t *testing.T) {
	pool := chainPool(t)
	ctx := context.Background()
	s := chainSeed(t, pool, "chain delete entry")
	for range 4 {
		s.scan(t, pool)
	}
	withoutGuard(t, pool, "record_chain", func() {
		if _, err := pool.Exec(ctx, `DELETE FROM record_chain WHERE site_id = $1 AND seq = 2`, s.site); err != nil {
			t.Fatal(err)
		}
	})
	wantBreakAt(t, pool, s.site, 2) // the missing entry

	// Dropping the newest entries is caught against the chain head.
	s2 := chainSeed(t, pool, "chain truncate tail")
	for range 3 {
		s2.scan(t, pool)
	}
	withoutGuard(t, pool, "record_chain", func() {
		if _, err := pool.Exec(ctx, `DELETE FROM record_chain WHERE site_id = $1 AND seq = 3`, s2.site); err != nil {
			t.Fatal(err)
		}
	})
	wantBreakAt(t, pool, s2.site, 3)
}

func TestChainConcurrentWritesStayValid(t *testing.T) {
	pool := chainPool(t)
	s := chainSeed(t, pool, "chain concurrent")
	const n = 24
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var err error
			if i%2 == 0 {
				_, _, err = repository.InsertScan(context.Background(), pool, model.CheckpointScan{
					SessionID: &s.session, RouteID: s.route, CheckpointID: fmt.Sprintf("cp%d", i), GuardID: s.guard,
					ScannedAt: time.Now(), Method: "qr",
				}, nil)
			} else {
				_, _, err = repository.CreateIncident(context.Background(), pool, model.Incident{
					SiteID: s.site, GuardID: s.guard, Type: "t", Severity: "low", OccurredAt: time.Now(),
					PhotoObjectKeys: []string{},
				}, nil)
			}
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if got := chainLen(t, pool, s.site); got != n {
		t.Fatalf("chain length = %d, want %d", got, n)
	}
	var maxSeq int
	if err := pool.QueryRow(context.Background(), `SELECT max(seq) FROM record_chain WHERE site_id = $1`, s.site).Scan(&maxSeq); err != nil || maxSeq != n {
		t.Fatalf("max seq = %d (%v), want %d (no gaps)", maxSeq, err, n)
	}
	wantValid(t, pool, s.site)
}

// withoutGuard lifts the append-only trigger of a table for one tamper step,
// standing in for an attacker with database superuser rights.
func withoutGuard(t *testing.T, pool *pgxpool.Pool, table string, fn func()) {
	t.Helper()
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `ALTER TABLE `+table+` DISABLE TRIGGER USER`); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := pool.Exec(ctx, `ALTER TABLE `+table+` ENABLE TRIGGER USER`); err != nil {
			t.Fatal(err)
		}
	}()
	fn()
}

func TestChainTableIsAppendOnly(t *testing.T) {
	pool := chainPool(t)
	s := chainSeed(t, pool, "chain guard")
	s.scan(t, pool)
	if _, err := pool.Exec(context.Background(), `UPDATE record_chain SET kind = 'x' WHERE site_id = $1`, s.site); err == nil {
		t.Fatal("record_chain UPDATE must be rejected")
	}
	if _, err := pool.Exec(context.Background(), `DELETE FROM record_chain WHERE site_id = $1`, s.site); err == nil {
		t.Fatal("record_chain DELETE must be rejected")
	}
}

func wantBreakReason(t *testing.T, pool *pgxpool.Pool, site, reason string) {
	t.Helper()
	b, err := repository.FirstChainBreak(context.Background(), pool, site)
	if err != nil {
		t.Fatal(err)
	}
	if b == nil || b.Reason != reason {
		t.Fatalf("break = %+v, want reason %q", b, reason)
	}
}

func TestChainDetectsForgedInsert(t *testing.T) {
	pool := chainPool(t)
	ctx := context.Background()
	s := chainSeed(t, pool, "chain forged")
	s.scan(t, pool)
	wantValid(t, pool, s.site)
	// A row inserted behind the application's back never gets a chain entry.
	if _, err := pool.Exec(ctx, `INSERT INTO checkpoint_scans (session_id, route_id, checkpoint_id, guard_id, scanned_at)
		VALUES ($1, $2, 'forged', $3, now())`, s.session, s.route, s.guard); err != nil {
		t.Fatal(err)
	}
	wantBreakAt(t, pool, s.site, 2)
	wantBreakReason(t, pool, s.site, "record has no chain entry")
	// Same for an incident, and for a site that has no chain at all yet.
	s2 := chainSeed(t, pool, "chain forged empty")
	if _, err := pool.Exec(ctx, `INSERT INTO incidents (site_id, guard_id, type, severity, occurred_at)
		VALUES ($1, $2, 'theft', 'low', now())`, s2.site, s2.guard); err != nil {
		t.Fatal(err)
	}
	wantBreakAt(t, pool, s2.site, 1)
}

func TestChainDetectsReorderedEntries(t *testing.T) {
	pool := chainPool(t)
	ctx := context.Background()
	s := chainSeed(t, pool, "chain reorder")
	for range 4 {
		s.scan(t, pool)
	}
	withoutGuard(t, pool, "record_chain", func() {
		for _, q := range []string{ // swap seq 2 and 3 through a free value
			`UPDATE record_chain SET seq = 99 WHERE site_id = $1 AND seq = 2`,
			`UPDATE record_chain SET seq = 2 WHERE site_id = $1 AND seq = 3`,
			`UPDATE record_chain SET seq = 3 WHERE site_id = $1 AND seq = 99`,
		} {
			if _, err := pool.Exec(ctx, q, s.site); err != nil {
				t.Fatal(err)
			}
		}
	})
	wantBreakAt(t, pool, s.site, 2)
}

func TestChainDetectsTamperedMetadata(t *testing.T) {
	pool := chainPool(t)
	ctx := context.Background()
	for _, set := range []string{`origin = 'backfill'`, `created_at = created_at - interval '1 day'`} {
		s := chainSeed(t, pool, "chain meta "+set[:6])
		s.scan(t, pool)
		s.scan(t, pool)
		withoutGuard(t, pool, "record_chain", func() {
			if _, err := pool.Exec(ctx, `UPDATE record_chain SET `+set+` WHERE site_id = $1 AND seq = 2`, s.site); err != nil {
				t.Fatal(err)
			}
		})
		wantBreakAt(t, pool, s.site, 2)
	}
}

func TestChainDetectsDeletedHead(t *testing.T) {
	pool := chainPool(t)
	ctx := context.Background()

	s := chainSeed(t, pool, "head only")
	s.scan(t, pool)
	s.scan(t, pool)
	withoutGuard(t, pool, "record_chain_heads", func() {
		if _, err := pool.Exec(ctx, `DELETE FROM record_chain_heads WHERE site_id = $1`, s.site); err != nil {
			t.Fatal(err)
		}
	})
	wantBreakReason(t, pool, s.site, "chain head missing")

	// Head and the newest entry gone: the dropped tail is still found.
	s2 := chainSeed(t, pool, "head and tail")
	s2.scan(t, pool)
	s2.scan(t, pool)
	withoutGuard(t, pool, "record_chain", func() {
		if _, err := pool.Exec(ctx, `DELETE FROM record_chain WHERE site_id = $1 AND seq = 2`, s2.site); err != nil {
			t.Fatal(err)
		}
	})
	withoutGuard(t, pool, "record_chain_heads", func() {
		if _, err := pool.Exec(ctx, `DELETE FROM record_chain_heads WHERE site_id = $1`, s2.site); err != nil {
			t.Fatal(err)
		}
	})
	wantBreakAt(t, pool, s2.site, 2) // the second scan has no entry

	// Head and every entry gone: the records without entries give it away.
	s3 := chainSeed(t, pool, "head and all")
	s3.scan(t, pool)
	s3.incident(t, pool)
	withoutGuard(t, pool, "record_chain", func() {
		if _, err := pool.Exec(ctx, `DELETE FROM record_chain WHERE site_id = $1`, s3.site); err != nil {
			t.Fatal(err)
		}
	})
	withoutGuard(t, pool, "record_chain_heads", func() {
		if _, err := pool.Exec(ctx, `DELETE FROM record_chain_heads WHERE site_id = $1`, s3.site); err != nil {
			t.Fatal(err)
		}
	})
	wantBreakAt(t, pool, s3.site, 1)
	wantBreakReason(t, pool, s3.site, "record has no chain entry")
}

func TestChainHeadIsNotMutableByTheApp(t *testing.T) {
	pool := chainPool(t)
	ctx := context.Background()
	s := chainSeed(t, pool, "head guard")
	s.scan(t, pool)
	for _, q := range []string{
		`DELETE FROM record_chain_heads WHERE site_id = $1`,
		`UPDATE record_chain_heads SET last_seq = 0 WHERE site_id = $1`,
		`UPDATE record_chain_heads SET last_hash = sha256('x'::bytea) WHERE site_id = $1`,
		`UPDATE record_chain_heads SET last_seq = last_seq + 5 WHERE site_id = $1`,
	} {
		if _, err := pool.Exec(ctx, q, s.site); err == nil {
			t.Fatalf("must be rejected: %s", q)
		}
	}
	if _, err := pool.Exec(ctx, `TRUNCATE record_chain_heads`); err == nil {
		t.Fatal("TRUNCATE record_chain_heads must be rejected")
	}
	s.scan(t, pool) // normal appends still work
	wantValid(t, pool, s.site)
}

func TestChainCoversOtherWritePaths(t *testing.T) {
	pool := chainPool(t)
	ctx := context.Background()
	s := chainSeed(t, pool, "chain paths")

	// FlagAbsentees: a shift that started 2 h ago with no clock-in.
	if _, err := pool.Exec(ctx, `INSERT INTO shifts (guard_id, site_id, start_time, end_time)
		VALUES ($1, $2, now() - interval '2 hours', now() + interval '6 hours')`, s.guard, s.site); err != nil {
		t.Fatal(err)
	}
	flags, err := repository.FlagAbsentees(ctx, pool, s.site, 30, 24*time.Hour)
	if err != nil || len(flags) != 1 {
		t.Fatalf("FlagAbsentees = %v, %v", flags, err)
	}
	if n := chainLen(t, pool, s.site); n != 1 {
		t.Fatalf("chain length after FlagAbsentees = %d, want 1", n)
	}
	wantValid(t, pool, s.site)

	// Override of that record, and of a shift that has no record yet.
	var shift string
	if err := pool.QueryRow(ctx, `INSERT INTO shifts (guard_id, site_id, start_time, end_time)
		VALUES ($1, $2, now(), now() + interval '8 hours') RETURNING id`, s.guard, s.site).Scan(&shift); err != nil {
		t.Fatal(err)
	}
	for _, sh := range []string{flags[0].ShiftID, shift} {
		if _, err := repository.OverrideAttendance(ctx, pool, repository.OverrideParams{
			ShiftID: sh, GuardID: s.guard, SiteID: s.site, Status: "present", Reason: "sick cover", Actor: "admin",
		}); err != nil {
			t.Fatal(err)
		}
	}
	if n := chainLen(t, pool, s.site); n != 3 {
		t.Fatalf("chain length after overrides = %d, want 3", n)
	}
	wantValid(t, pool, s.site)

	// Bulk retention clearing of GPS chains every changed row.
	att := s.attendance(t, pool)
	sc := s.scan(t, pool)
	if _, err := pool.Exec(ctx, `UPDATE attendance_records SET gps_lat = 1, gps_lng = 2 WHERE id = $1`, att); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE checkpoint_scans SET gps_lat = 1, gps_lng = 2 WHERE id = $1`, sc); err != nil {
		t.Fatal(err)
	}
	wantBreakAt(t, pool, s.site, 4) // the out-of-band GPS edits are seen...
	before := chainLen(t, pool, s.site)
	// ...and the retention job's own clearing is chained, so a rewrite through it is not a break.
	cutoff := time.Now().Add(time.Hour)
	if _, err := repository.ClearAttendanceGPS(ctx, pool, cutoff, 100); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.ClearScanGPS(ctx, pool, cutoff, 100); err != nil {
		t.Fatal(err)
	}
	if n := chainLen(t, pool, s.site); n <= before {
		t.Fatalf("retention did not chain: %d -> %d", before, n)
	}
	wantValid(t, pool, s.site)
}

func TestChainRolledBackWriteLeavesNoEntry(t *testing.T) {
	pool := chainPool(t)
	ctx := context.Background()
	s := chainSeed(t, pool, "chain rollback")
	s.scan(t, pool)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := repository.InsertScan(ctx, tx, model.CheckpointScan{
		SessionID: &s.session, RouteID: s.route, CheckpointID: "rolled", GuardID: s.guard, ScannedAt: time.Now(), Method: "qr",
	}, nil); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if n := chainLen(t, pool, s.site); n != 1 {
		t.Fatalf("chain length after rollback = %d, want 1", n)
	}
	var head int
	if err := pool.QueryRow(ctx, `SELECT last_seq FROM record_chain_heads WHERE site_id = $1`, s.site).Scan(&head); err != nil || head != 1 {
		t.Fatalf("head seq = %d (%v), want 1", head, err)
	}
	s.scan(t, pool) // next write takes seq 2, no gap
	wantValid(t, pool, s.site)

	// A failed chained write inside a caller's transaction rolls back only itself (savepoint).
	tx, err = pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.CreateAmendment(ctx, tx, "00000000-0000-0000-0000-000000000000", []byte(`{}`), "r", "a"); err == nil {
		t.Fatal("amendment of an unknown incident must fail")
	}
	s2 := s.scanIn(t, tx)
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	_ = s2
	wantValid(t, pool, s.site)
}

// Rows written before migration 011 are chained by the migration itself.
func TestChainMigrationBackfillsExistingRows(t *testing.T) {
	ctx := context.Background()
	admin := chainPool(t)
	if _, err := admin.Exec(ctx, `DROP DATABASE IF EXISTS chain_mig`); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, `CREATE DATABASE chain_mig`); err != nil {
		t.Fatal(err)
	}
	migDSN := dsn[:len(dsn)-len("postgres?sslmode=disable")] + "chain_mig?sslmode=disable"
	m, err := migrate.New("file://../../migrations", migDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if err := m.Migrate(10); err != nil {
		t.Fatal(err)
	}
	pool, err := db.New(ctx, migDSN, db.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	// Two sites of pre-chain data, written with plain SQL.
	var sites [2]string
	for i := range sites {
		var guard, route, session, shift string
		for _, q := range []struct {
			sql string
			dst *string
			arg []any
		}{
			{`INSERT INTO sites (name, keycloak_realm) VALUES ($1, 'chain') RETURNING id`, &sites[i], []any{fmt.Sprintf("old %d", i)}},
			{`INSERT INTO guards (name, role, site_id) VALUES ('G', 'guard', $1) RETURNING id`, &guard, []any{&sites[i]}},
			{`INSERT INTO routes (site_id, name) VALUES ($1, 'r') RETURNING id`, &route, []any{&sites[i]}},
			{`INSERT INTO patrol_sessions (route_id, site_id, guard_id, started_at) VALUES ($1, $2, $3, now()) RETURNING id`, &session, []any{&route, &sites[i], &guard}},
			{`INSERT INTO shifts (guard_id, site_id, start_time, end_time) VALUES ($1, $2, now(), now() + interval '1 hour') RETURNING id`, &shift, []any{&guard, &sites[i]}},
		} {
			args := make([]any, len(q.arg))
			for k, a := range q.arg {
				if p, ok := a.(*string); ok {
					args[k] = *p
				} else {
					args[k] = a
				}
			}
			if err := pool.QueryRow(ctx, q.sql, args...).Scan(q.dst); err != nil {
				t.Fatal(err)
			}
		}
		// IDs are test-generated UUIDs, so formatting them into the SQL is safe.
		for _, sql := range []string{
			`INSERT INTO attendance_records (shift_id, guard_id, site_id, status, clock_in_at) VALUES ('%[4]s', '%[2]s', '%[1]s', 'present', now())`,
			`INSERT INTO checkpoint_scans (session_id, route_id, checkpoint_id, guard_id, scanned_at) VALUES ('%[5]s', '%[3]s', 'cp', '%[2]s', now())`,
			`INSERT INTO checkpoint_scans (session_id, route_id, checkpoint_id, guard_id, scanned_at) VALUES ('%[5]s', '%[3]s', 'cp2', '%[2]s', now())`,
			`INSERT INTO incidents (site_id, guard_id, type, severity, occurred_at) VALUES ('%[1]s', '%[2]s', 'theft', 'low', now())`,
		} {
			if _, err := pool.Exec(ctx, fmt.Sprintf(sql, sites[i], guard, route, shift, session)); err != nil {
				t.Fatalf("%s: %v", sql, err)
			}
		}
	}

	if err := m.Up(); err != nil {
		t.Fatal(err)
	}
	for _, site := range sites {
		if n := chainLen(t, pool, site); n != 4 {
			t.Fatalf("backfilled chain length = %d, want 4", n)
		}
		var backfilled int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM record_chain WHERE site_id = $1 AND origin = 'backfill'`, site).Scan(&backfilled); err != nil || backfilled != 4 {
			t.Fatalf("backfill-origin entries = %d (%v), want 4", backfilled, err)
		}
		wantValid(t, pool, site)
	}

	// The down migration works and the chain can be rebuilt by migrating up again.
	if err := m.Steps(-1); err != nil {
		t.Fatal(err)
	}
	if err := m.Up(); err != nil {
		t.Fatal(err)
	}
	for _, site := range sites {
		wantValid(t, pool, site)
	}

	// New writes continue the same chain, and tampering with old data is found.
	if _, err := pool.Exec(ctx, `UPDATE incidents SET description = 'edited' WHERE site_id = $1`, sites[0]); err != nil {
		t.Fatal(err)
	}
	b, err := repository.FirstChainBreak(ctx, pool, sites[0])
	if err != nil || b == nil {
		t.Fatalf("edit of a backfilled record not detected: %v %v", b, err)
	}
}

func (s chainSite) scanIn(t *testing.T, db repository.DBTX) string {
	t.Helper()
	sc, _, err := repository.InsertScan(context.Background(), db, model.CheckpointScan{
		SessionID: &s.session, RouteID: s.route, CheckpointID: "in-tx", GuardID: s.guard, ScannedAt: time.Now(), Method: "qr",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return sc.ID
}
