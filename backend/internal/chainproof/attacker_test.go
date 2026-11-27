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
	r := f.verify(Options{})
	if r.Status != StatusValid || r.RangeAuthenticated {
		t.Fatalf("%s: a cut at a signed head looks fine without an expected range, and says the range is not authenticated", r.Status)
	}
	if !hasWarning(r, "range is not authenticated") {
		t.Fatalf("no warning: %v", r.Warnings)
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
	// an explicit opt-out still says so
	r = f.verify(Options{AllowUnverified: true})
	if r.Status != StatusValid || !hasWarning(r, "NOT checked") {
		t.Fatalf("opt-out: %s %v", r.Status, r.Warnings)
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
	r := f.verify(Options{MaxAge: 26 * time.Hour, Now: t0.Add(40 * 24 * time.Hour)})
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
