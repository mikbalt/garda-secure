package chainproof

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/securepatrol/backend/internal/signer"
)

const testSite = "11111111-2222-4333-8444-555555555555"

var t0 = time.Date(2026, 11, 1, 8, 0, 0, 0, time.UTC)

type fixture struct {
	key  ed25519.PrivateKey
	pin  string
	b    *Bundle
	seed string
}

func hexs(b []byte) string { return hex.EncodeToString(b) }

// newFixture builds a valid chain of n entries, a genesis keyring and heads
// signed at the given seqs (each one hour after its entry).
func newFixture(t *testing.T, n int, headSeqs ...int64) *fixture {
	t.Helper()
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{key: key, pin: signer.KeyID(pub), seed: hexs(key.Seed())}
	b := &Bundle{Manifest: Manifest{SiteID: testSite, SiteTimezone: "UTC", From: t0.Format(time.RFC3339), To: t0.Add(30 * 24 * time.Hour).Format(time.RFC3339), ExportedAt: t0.Format(time.RFC3339Nano)}}
	created := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	kinds := []string{"attendance", "scan", "incident", "incident_amendment"}
	prev := Genesis(testSite)
	for i := 1; i <= n; i++ {
		kind := kinds[i%len(kinds)]
		id := fmt.Sprintf("00000000-0000-4000-8000-%012d", i)
		rec := fmt.Sprintf(`{"id": "%s", "n": %d}`, id, i)
		rh := sha256.Sum256([]byte(kind + ":" + rec))
		at := created.Add(time.Duration(i) * time.Minute)
		e := Entry{Seq: int64(i), Kind: kind, RecordID: id, RecordHash: hexs(rh[:]), PrevHash: hexs(prev), HashVersion: 1, Origin: "live",
			CreatedAt: at.Format(tsLayout), RecordState: StateLatest, Record: rec}
		eh, err := EntryHash(testSite, e)
		if err != nil {
			t.Fatal(err)
		}
		e.EntryHash = hexs(eh)
		prev = eh
		b.Entries = append(b.Entries, e)
	}
	b.Manifest.FirstSeq, b.Manifest.LastSeq = 1, int64(n)
	b.Manifest.ChainTipSeq, b.Manifest.ChainTipHash = int64(n), hexs(prev)
	gen := KeyEntry(key, t0.Add(-time.Hour))
	b.Keyring = []Key{gen}
	for _, s := range headSeqs {
		b.Heads = append(b.Heads, f.signHead(t, b, s, b.Entries[s-1].EntryHash, time.Time{}))
	}
	f.b = b
	return f
}

func (f *fixture) signHead(t *testing.T, b *Bundle, seq int64, entryHash string, at time.Time) Head {
	t.Helper()
	if at.IsZero() {
		at = time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC).Add(time.Duration(seq)*time.Minute + time.Hour)
	}
	eh, _ := hex.DecodeString(entryHash)
	sig := ed25519.Sign(f.key, signer.HeadPayload(testSite, seq, eh, at, f.pin))
	return Head{SiteID: testSite, Seq: seq, EntryHash: entryHash, SignedAt: at.Format(time.RFC3339Nano), Reason: "hourly", KeyID: f.pin, Signature: hexs(sig)}
}

// rechain rewrites the chain from seq on, after mutate changed the entries: a
// forger who also fixes every hash behind the edit.
func rechain(t *testing.T, b *Bundle, from int) {
	t.Helper()
	prev := Genesis(testSite)
	if from > 0 {
		prev, _ = hex.DecodeString(b.Entries[from-1].EntryHash)
	}
	for i := from; i < len(b.Entries); i++ {
		e := &b.Entries[i]
		rh := sha256.Sum256([]byte(e.Kind + ":" + e.Record))
		e.RecordHash, e.PrevHash = hexs(rh[:]), hexs(prev)
		eh, err := EntryHash(testSite, *e)
		if err != nil {
			t.Fatal(err)
		}
		e.EntryHash = hexs(eh)
		prev = eh
	}
	b.Manifest.ChainTipHash = hexs(prev)
}

// verify opts out of the range check (most tests are not about the range);
// verifyStrict does not, as a client who gave no flags.
func (f *fixture) verify(opts Options) Result {
	opts.NoRangeCheck = opts.NoRangeCheck || opts.ExpectFrom == ""
	return f.verifyStrict(opts)
}

func (f *fixture) verifyStrict(opts Options) Result {
	if opts.Pins == nil {
		opts.Pins = []string{f.pin}
	}
	if opts.Now.IsZero() {
		opts.Now = t0.Add(2 * time.Hour)
	}
	return Verify(f.b, opts)
}

func wantStatus(t *testing.T, r Result, status string, code int, seq int64, reasonPart string) {
	t.Helper()
	if r.Status != status || r.Code() != code {
		t.Fatalf("status %s code %d (%+v), want %s code %d", r.Status, r.Code(), r.Break, status, code)
	}
	if status == StatusValid {
		return
	}
	if r.Break == nil || r.Break.Seq != seq || !strings.Contains(r.Break.Reason, reasonPart) {
		t.Fatalf("break %+v, want seq %d reason containing %q", r.Break, seq, reasonPart)
	}
}

func TestValidBundle(t *testing.T) {
	f := newFixture(t, 8, 4, 8)
	r := f.verify(Options{})
	wantStatus(t, r, StatusValid, 0, 0, "")
	if r.Entries != 8 || r.Heads != 2 {
		t.Fatalf("counted %d entries %d heads", r.Entries, r.Heads)
	}
}

func TestEmptyBundleProvesNothing(t *testing.T) {
	f := newFixture(t, 0)
	wantStatus(t, f.verify(Options{}), StatusNoEntries, 9, 0, "proves nothing")
}

func TestEditedEntryIsFirstBreak(t *testing.T) {
	f := newFixture(t, 8, 8)
	f.b.Entries[2].RecordHash = strings.Repeat("ab", 32)
	r := f.verify(Options{})
	wantStatus(t, r, StatusBroken, 1, 3, "entry content does not match its hash")
	if r.Break.Kind != f.b.Entries[2].Kind || r.Break.ID != f.b.Entries[2].RecordID {
		t.Fatalf("break names %s %s", r.Break.Kind, r.Break.ID)
	}
}

func TestEditedRecordIsFirstBreak(t *testing.T) {
	f := newFixture(t, 8, 8)
	f.b.Entries[4].Record = strings.Replace(f.b.Entries[4].Record, `"n": 5`, `"n": 500`, 1)
	wantStatus(t, f.verify(Options{}), StatusBroken, 1, 5, "record modified")
}

func TestDeletedRecordIsFirstBreak(t *testing.T) {
	f := newFixture(t, 8, 8)
	f.b.Entries[5].RecordState, f.b.Entries[5].Record = StateDeleted, ""
	wantStatus(t, f.verify(Options{}), StatusBroken, 1, 6, "record deleted")
}

func TestDeletedEntryIsFirstBreak(t *testing.T) {
	f := newFixture(t, 8, 8)
	f.b.Entries = append(f.b.Entries[:2], f.b.Entries[3:]...)
	wantStatus(t, f.verify(Options{}), StatusBroken, 1, 3, "entry missing")
}

func TestDroppedTailIsBroken(t *testing.T) {
	f := newFixture(t, 8, 8)
	f.b.Entries = f.b.Entries[:7]
	f.b.Manifest.LastSeq = 7
	// the signed head at 8 now points past the end of the chain
	wantStatus(t, f.verify(Options{}), StatusBroken, 4, 8, "ends at 7")
}

func TestForgedPastIsCaughtBySignedHead(t *testing.T) {
	f := newFixture(t, 8, 4, 8)
	f.b.Entries[1].Record = `{"id": "x", "n": 999}` // edit seq 2 and fix every hash behind it
	rechain(t, f.b, 1)
	// the chain is internally consistent, only the signed heads disagree
	wantStatus(t, f.verify(Options{}), StatusBroken, 4, 4, "differs from the signed head")
}

func TestForgedHeadSignature(t *testing.T) {
	f := newFixture(t, 8, 4, 8)
	sig := []byte(f.b.Heads[0].Signature)
	// swap the first hex digit for a different valid one (a bit flip can leave the hex alphabet)
	if sig[0] == '0' {
		sig[0] = '1'
	} else {
		sig[0] = '0'
	}
	f.b.Heads[0].Signature = string(sig)
	wantStatus(t, f.verify(Options{}), StatusBroken, 4, 4, "signature does not verify")
}

func TestHeadForAnotherHashIsCaught(t *testing.T) {
	f := newFixture(t, 8, 8)
	f.b.Heads[0] = f.signHead(t, f.b, 8, strings.Repeat("cd", 32), time.Time{}) // validly signed, other hash
	wantStatus(t, f.verify(Options{}), StatusBroken, 4, 8, "differs from the signed head")
}

func TestWrongFingerprintIsCaught(t *testing.T) {
	f := newFixture(t, 8, 8)
	other := strings.Repeat("0", 64)
	wantStatus(t, f.verify(Options{Pins: []string{other}}), StatusBroken, 4, 8, "not trusted")
}

func TestForgedKeyringIsCaught(t *testing.T) {
	f := newFixture(t, 8, 8)
	// an attacker re-signs the whole history with a key of their own and ships its keyring
	pub, key, _ := ed25519.GenerateKey(rand.Reader)
	f.b.Keyring = []Key{KeyEntry(key, t0)}
	id := signer.KeyID(pub)
	eh, _ := hex.DecodeString(f.b.Entries[7].EntryHash)
	at := t0.Add(time.Hour)
	f.b.Heads[0] = Head{SiteID: testSite, Seq: 8, EntryHash: f.b.Entries[7].EntryHash, SignedAt: at.Format(time.RFC3339Nano), Reason: "hourly", KeyID: id,
		Signature: hexs(ed25519.Sign(key, signer.HeadPayload(testSite, 8, eh, at, id)))}
	wantStatus(t, f.verify(Options{}), StatusBroken, 4, 8, "not trusted")
}

func TestUnsigned(t *testing.T) {
	f := newFixture(t, 5)
	wantStatus(t, f.verify(Options{}), StatusUnsigned, 5, 1, "no signed head")
}

func TestStale(t *testing.T) {
	f := newFixture(t, 8, 3)
	limit := 26 * time.Hour
	wantStatus(t, f.verify(Options{MaxAge: limit}), StatusUnsignedTail, 7, 4, "UNSIGNED TAIL") // head is young: signed soon, but not signed yet
	r := f.verify(Options{MaxAge: limit, Now: t0.Add(5 * 24 * time.Hour)})
	wantStatus(t, r, StatusStale, 6, 4, "newest signed head")
	// a quiet site is never stale: the head covers the whole chain
	g := newFixture(t, 3, 3)
	wantStatus(t, g.verify(Options{MaxAge: limit, Now: t0.Add(5 * 24 * time.Hour)}), StatusValid, 0, 0, "")
}

func TestBrokenBeatsUnsignedAndStale(t *testing.T) {
	f := newFixture(t, 8, 3)
	f.b.Entries[6].RecordHash = strings.Repeat("ab", 32)
	wantStatus(t, f.verify(Options{MaxAge: time.Hour, Now: t0.Add(5 * 24 * time.Hour)}), StatusBroken, 1, 7, "entry content")
}

// checkpoint returns the bundle of entries after the head at seq, as the
// exporter makes it for a range that starts after that head.
func (f *fixture) checkpoint(seq int64) *fixture {
	c := *f
	nb := *f.b
	nb.Entries = append([]Entry(nil), f.b.Entries[seq:]...)
	nb.Heads = nil
	for _, h := range f.b.Heads {
		if h.Seq >= seq {
			nb.Heads = append(nb.Heads, h)
		}
	}
	nb.Manifest.AnchorSeq, nb.Manifest.FirstSeq = seq, seq+1
	c.b = &nb
	return &c
}

func TestCheckpointRangeEqualsFullVerify(t *testing.T) {
	// valid data: both are valid
	f := newFixture(t, 12, 4, 8, 12)
	wantStatus(t, f.checkpoint(8).verify(Options{}), StatusValid, 0, 0, "")
	// an edit after the checkpoint: same first break
	f.b.Entries[9].RecordHash = strings.Repeat("ab", 32)
	full := f.verify(Options{})
	part := f.checkpoint(8).verify(Options{})
	wantStatus(t, full, StatusBroken, 1, 10, "entry content")
	if part.Break == nil || part.Break.Seq != full.Break.Seq || part.Break.Reason != full.Break.Reason || part.Code() != full.Code() {
		t.Fatalf("checkpoint %+v, full %+v", part.Break, full.Break)
	}
	if part.Entries >= full.Entries {
		t.Fatalf("checkpoint checked %d entries, full %d", part.Entries, full.Entries)
	}
	// an edit before the checkpoint is outside what this range vouches for, and not reported
	g := newFixture(t, 12, 4, 8, 12)
	g.b.Entries[1].RecordHash = strings.Repeat("ab", 32)
	wantStatus(t, g.checkpoint(8).verify(Options{}), StatusValid, 0, 0, "")
	wantStatus(t, g.verify(Options{}), StatusBroken, 1, 2, "entry content")
}

func TestCheckpointAnchorMustBeSignedAndPresent(t *testing.T) {
	f := newFixture(t, 12, 4, 8, 12).checkpoint(8)
	sig := []byte(f.b.Heads[0].Signature)
	if sig[3] == '0' { // stay in the hex alphabet: a bit flip can leave it
		sig[3] = '1'
	} else {
		sig[3] = '0'
	}
	f.b.Heads[0].Signature = string(sig)
	wantStatus(t, f.verify(Options{}), StatusBroken, 4, 8, "signature does not verify")

	g := newFixture(t, 12, 4, 8, 12).checkpoint(8)
	g.b.Heads = g.b.Heads[1:] // anchor head removed
	r := g.verify(Options{})
	if r.Status != StatusBundle || r.Code() != 3 {
		t.Fatalf("missing anchor: %s %+v", r.Status, r.Break)
	}
}

func TestSupersededEntries(t *testing.T) {
	f := newFixture(t, 6, 6)
	// seq 1 says the same record changed again at seq 4: it must be that record
	f.b.Entries[0].RecordState, f.b.Entries[0].Record, f.b.Entries[0].NextSeq = StateSuperseded, "", 4
	wantStatus(t, f.verify(Options{}), StatusBroken, 1, 1, "superseded")
	f.b.Entries[3].Kind, f.b.Entries[3].RecordID = f.b.Entries[0].Kind, f.b.Entries[0].RecordID
	rechain(t, f.b, 3)
	f.b.Heads[0] = f.signHead(t, f.b, 6, f.b.Entries[5].EntryHash, time.Time{})
	r := f.verify(Options{})
	wantStatus(t, r, StatusValid, 0, 0, "")
	// a later entry beyond the range cannot be checked: said so, not hidden
	f.b.Entries[1].RecordState, f.b.Entries[1].Record, f.b.Entries[1].NextSeq = StateSuperseded, "", 50
	wantStatus(t, f.verify(Options{}), StatusUnverified, 8, 2, "UNVERIFIED RECORDS")
	if r := f.verify(Options{AllowUnverified: true}); r.Status != StatusUnverifiedOK || r.Code() != 12 || r.RecordsNotChecked != 1 || !hasWarning(r, "NOT checked") {
		t.Fatalf("%s, %d records not checked, %v", r.Status, r.RecordsNotChecked, r.Warnings)
	}
}

func TestUnsupportedHashVersion(t *testing.T) {
	f := newFixture(t, 3, 3)
	f.b.Entries[1].HashVersion = 2
	wantStatus(t, f.verify(Options{}), StatusBroken, 1, 2, "hash_version")
}

func TestBundleRoundTripAndFileTamper(t *testing.T) {
	f := newFixture(t, 8, 8)
	var buf bytes.Buffer
	if err := f.b.Write(&buf); err != nil {
		t.Fatal(err)
	}
	got, err := Read(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if r := Verify(got, Options{Pins: []string{f.pin}, Now: t0.Add(time.Hour), NoRangeCheck: true}); r.Status != StatusValid {
		t.Fatalf("round trip: %+v", r.Break)
	}
	if got.ManifestSHA256 != f.b.ManifestSHA256 || got.ManifestSHA256 == "" {
		t.Fatal("integrity hash differs after a round trip")
	}
	if bytes.Contains(buf.Bytes(), []byte(f.seed)) {
		t.Fatal("the private seed is in the bundle")
	}
	// a changed file no longer matches the manifest. The zip is deflated, so
	// rebuild one with an edited entries.jsonl.
	f.b.Entries[2].Record = "{}"
	var edited bytes.Buffer
	if err := f.b.Write(&edited); err != nil {
		t.Fatal(err)
	}
	tampered := swapFile(t, buf.Bytes(), edited.Bytes(), FileEntries)
	if _, err := Read(tampered); err == nil || !strings.Contains(err.Error(), "does not match the hash") {
		t.Fatalf("tampered entries.jsonl: %v", err)
	}
	if _, err := Read([]byte("not a zip")); err == nil {
		t.Fatal("garbage accepted")
	}
}
