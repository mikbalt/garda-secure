package chainproof

import (
	"strings"
	"testing"
	"time"
)

// These tests are written as the attacker: whoever builds the bundle (the
// exporter, or anyone on the way) forges one and asks for a plain "valid".
// None of them may come out as StatusValid with exit code 0.

func mustNotBeValid(t *testing.T, r Result, status string, code int) {
	t.Helper()
	if r.Status == StatusValid || r.Code() == CodeValid {
		t.Fatalf("FORGED BUNDLE ACCEPTED: status %s code %d", r.Status, r.Code())
	}
	if r.Status != status || r.Code() != code {
		t.Fatalf("status %s code %d (%+v), want %s code %d", r.Status, r.Code(), r.Break, status, code)
	}
}

// 1. No entries, no heads, a patched manifest.
func TestAttackerEmptyBundle(t *testing.T) {
	f := newFixture(t, 8, 8)
	f.b.Entries, f.b.Heads = nil, nil
	f.b.Manifest.FirstSeq, f.b.Manifest.LastSeq, f.b.Manifest.ChainTipSeq, f.b.Manifest.ChainTipHash = 0, 0, 0, ""
	f.b.Manifest.Entries, f.b.Manifest.Heads = 0, 0
	mustNotBeValid(t, f.verify(Options{}), StatusNoEntries, CodeNoEntries)
}

// 2. The bundle stops before the tamper: later heads dropped, manifest patched.
func TestAttackerTailTruncation(t *testing.T) {
	f := newFixture(t, 8, 4, 8)
	f.b.Entries[6].Record = `{"tampered": true}` // the record the attacker hides is seq 7: it is cut off
	f.b.Entries = f.b.Entries[:6]
	f.b.Heads = f.b.Heads[:1] // head 8 dropped, only head 4 stays
	f.b.Manifest.LastSeq, f.b.Manifest.ChainTipSeq = 6, 6
	f.b.Manifest.ChainTipHash = f.b.Entries[5].EntryHash
	r := f.verify(Options{})
	mustNotBeValid(t, r, StatusUnsignedTail, CodeUnsignedTail)
	if !strings.Contains(r.Break.Reason, "2 entries after the newest signed head") {
		t.Fatalf("reason %q", r.Break.Reason)
	}
}

// 2b. The same cut exactly at an older signed head: only the range the caller
// expects shows it (the newest signed head is older than the end of the range).
func TestAttackerCutAtAnOlderHead(t *testing.T) {
	f := newFixture(t, 8, 4, 8)
	f.b.Manifest.From, f.b.Manifest.To = "2026-11-01T00:00:00Z", "2026-12-01T00:00:00Z"
	f.b.Entries, f.b.Heads = f.b.Entries[:4], f.b.Heads[:1]
	f.b.Manifest.LastSeq, f.b.Manifest.ChainTipSeq = 4, 4
	f.b.Manifest.ChainTipHash = f.b.Entries[3].EntryHash
	// no flags: never a silent exit 0
	mustNotBeValid(t, f.verifyStrict(Options{}), StatusRangeNotAuth, CodeRangeNotAuth)
	// the explicit opt-out: valid, but marked as "range not checked"
	r := f.verify(Options{NoRangeCheck: true})
	if r.Status != StatusValid || r.RangeAuthenticated || !r.RangeNotChecked || !hasWarning(r, "range is not authenticated") {
		t.Fatalf("%s: opt-out must say the range was not checked: %v", r.Status, r.Warnings)
	}
	r = f.verify(Options{ExpectFrom: "2026-11-01", ExpectTo: "2026-11-30"})
	mustNotBeValid(t, r, StatusRange, CodeRange)
	if !strings.Contains(r.Break.Reason, "newest signed head") {
		t.Fatalf("reason %q", r.Break.Reason)
	}
}

// 3. A record edited in the database is labelled "superseded by something after the range".
func TestAttackerSupersededEscape(t *testing.T) {
	f := newFixture(t, 6, 6)
	e := &f.b.Entries[2]
	e.Record, e.RecordState, e.NextSeq = `{"edited": "by the exporter"}`, StateSuperseded, 7 // LastSeq+1
	e.Record = ""
	r := f.verify(Options{})
	mustNotBeValid(t, r, StatusUnverified, CodeUnverified)
	if r.RecordsNotChecked != 1 {
		t.Fatalf("records not checked %d", r.RecordsNotChecked)
	}
	// the flag does not make it exit 0 (a client adding it out of habit)
	r = f.verify(Options{AllowUnverified: true})
	mustNotBeValid(t, r, StatusUnverifiedOK, CodeUnverifiedOK)
	if r.RecordsNotChecked != 1 || !hasWarning(r, "1 of 6") {
		t.Fatalf("count and share: %d %v", r.RecordsNotChecked, r.Warnings)
	}
}

// 4. The anchor head is placed after the start of the range, to skip the early part.
func TestAttackerLateAnchor(t *testing.T) {
	f := newFixture(t, 12, 4, 8, 12).checkpoint(8)
	f.b.Manifest.From, f.b.Manifest.To = "2026-11-01T00:00:00Z", "2026-12-01T00:00:00Z"
	mustNotBeValid(t, f.verify(Options{ExpectFrom: "2026-11-01", ExpectTo: "2026-11-30"}), StatusRange, CodeRange)
	// an honest bundle: from the start of the chain, a head signed after the end of the range
	g := newFixture(t, 8)
	g.b.Manifest.From, g.b.Manifest.To = "2026-11-01T00:00:00Z", "2026-12-01T00:00:00Z"
	g.b.Heads = []Head{g.signHead(t, g.b, 8, g.b.Entries[7].EntryHash, time.Date(2026, 12, 2, 0, 0, 0, 0, time.UTC))}
	r := g.verify(Options{ExpectFrom: "2026-11-01", ExpectTo: "2026-11-30"})
	if r.Status != StatusValid || !r.RangeAuthenticated {
		t.Fatalf("honest bundle: %s %+v", r.Status, r.Break)
	}
}

// 4b. The manifest claims a range the entries are not in.
func TestAttackerManifestRangeLies(t *testing.T) {
	f := newFixture(t, 8, 8)
	f.b.Manifest.From, f.b.Manifest.To = "2026-03-01T00:00:00Z", "2026-04-01T00:00:00Z"
	r := f.verify(Options{})
	if r.Status != StatusBundle || r.Code() != CodeBundle || !strings.Contains(r.Break.Reason, "range") {
		t.Fatalf("%s %+v", r.Status, r.Break)
	}
}

// 5. An old genuine bundle shown as current.
func TestAttackerReplay(t *testing.T) {
	f := newFixture(t, 8, 8)
	f.b.Manifest.ExportedAt = t0.Add(2 * time.Hour).Format(time.RFC3339Nano)
	r := f.verifyStrict(Options{MaxAge: 26 * time.Hour, Now: t0.Add(40 * 24 * time.Hour)})
	mustNotBeValid(t, r, StatusRangeNotAuth, CodeRangeNotAuth) // an old bundle is not "valid" without a range to hold it to
	if !hasWarning(r, "exported") || r.ExportedAt == "" || r.NewestHeadSignedAt == "" {
		t.Fatalf("replay not shown: %v", r.Warnings)
	}
}

func hasWarning(r Result, part string) bool {
	for _, w := range r.Warnings {
		if strings.Contains(w, part) {
			return true
		}
	}
	return false
}

// The contract: no combination of opt-in flags alone gives plain VALID, exit 0,
// for a bundle that has something unproven.
func TestFlagsAloneNeverGivePlainValid(t *testing.T) {
	cases := map[string]func() *fixture{
		"superseded escape": func() *fixture {
			f := newFixture(t, 6, 6)
			f.b.Entries[2].Record, f.b.Entries[2].RecordState, f.b.Entries[2].NextSeq = "", StateSuperseded, 7
			return f
		},
		"cut at an older head": func() *fixture {
			f := newFixture(t, 8, 4, 8)
			f.b.Entries, f.b.Heads = f.b.Entries[:4], f.b.Heads[:1]
			f.b.Manifest.LastSeq, f.b.Manifest.ChainTipSeq, f.b.Manifest.ChainTipHash = 4, 4, f.b.Entries[3].EntryHash
			return f
		},
		"honest bundle, no range given": func() *fixture { return newFixture(t, 8, 8) },
	}
	for name, mk := range cases {
		for _, o := range []Options{{}, {AllowUnverified: true}} {
			if r := mk().verifyStrict(o); r.Code() == CodeValid {
				t.Errorf("%s with %+v: exit 0, %s", name, o, r.Status)
			}
		}
	}
	// only the pair of explicit opt-outs reaches exit 0 for the range, and
	// the superseded records still keep it non-zero
	f := cases["superseded escape"]()
	if r := f.verify(Options{AllowUnverified: true, NoRangeCheck: true}); r.Code() == CodeValid {
		t.Errorf("both flags give exit 0 with an unchecked record")
	}
}

// Two heads for the same seq: a forged one cannot hide behind a genuine one.
func TestTwoHeadsAtTheSameSeq(t *testing.T) {
	f := newFixture(t, 8, 8)
	dup := f.b.Heads[0]
	f.b.Heads = append(f.b.Heads, dup) // the same head twice is harmless
	wantStatus(t, f.verify(Options{}), StatusValid, 0, 0, "")

	g := newFixture(t, 8, 8)
	forged := g.signHead(t, g.b, 8, strings.Repeat("cd", 32), time.Time{}) // validly signed, other hash
	g.b.Heads = append(g.b.Heads, forged)
	wantStatus(t, g.verify(Options{}), StatusBroken, 4, 8, "differs from the signed head")
	// and in the other order
	g.b.Heads[0], g.b.Heads[1] = g.b.Heads[1], g.b.Heads[0]
	wantStatus(t, g.verify(Options{}), StatusBroken, 4, 8, "differs from the signed head")
}
