package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/securepatrol/backend/internal/chainproof"
	"github.com/securepatrol/backend/internal/signer"
)

const site = "11111111-2222-4333-8444-555555555555"

var now = time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)

// bundleFile writes a signed 4-entry bundle and returns its path and the key.
func bundleFile(t *testing.T, mutate func(*chainproof.Bundle), headSeq int64, headAge time.Duration) (string, ed25519.PrivateKey, string) {
	t.Helper()
	pub, key, _ := ed25519.GenerateKey(rand.Reader)
	pin := signer.KeyID(pub)
	b := &chainproof.Bundle{Manifest: chainproof.Manifest{SiteID: site, SiteTimezone: "UTC", From: "2026-11-01T00:00:00Z", To: "2026-12-01T00:00:00Z", ExportedAt: now.Format(time.RFC3339Nano), FirstSeq: 1, LastSeq: 4, ChainTipSeq: 4}}
	prev := chainproof.Genesis(site)
	for i := 1; i <= 4; i++ {
		rec := `{"n": ` + string(rune('0'+i)) + `}`
		rh := sha256.Sum256([]byte("scan:" + rec))
		e := chainproof.Entry{Seq: int64(i), Kind: "scan", RecordID: "00000000-0000-4000-8000-00000000000" + string(rune('0'+i)), RecordHash: hex.EncodeToString(rh[:]),
			PrevHash: hex.EncodeToString(prev), HashVersion: 1, Origin: "live", CreatedAt: "2026-11-01T09:00:00.000000Z", RecordState: chainproof.StateLatest, Record: rec}
		eh, _ := chainproof.EntryHash(site, e)
		e.EntryHash, prev = hex.EncodeToString(eh), eh
		b.Entries = append(b.Entries, e)
	}
	b.Manifest.ChainTipHash = hex.EncodeToString(prev)
	at := now.Add(-time.Hour).Truncate(time.Microsecond)
	nks := ed25519.Sign(key, signer.KeyPayload(signer.KindGenesis, 1, pin, pub, "", at))
	b.Keyring = []chainproof.Key{{KeyID: pin, Epoch: 1, Kind: signer.KindGenesis, PublicKey: hex.EncodeToString(pub), CreatedAt: at.Format(time.RFC3339Nano), NewKeySig: hex.EncodeToString(nks)}}
	if headSeq > 0 {
		hat := now.Add(-headAge).Truncate(time.Microsecond)
		eh := b.Entries[headSeq-1].EntryHash
		raw, _ := hex.DecodeString(eh)
		sig := ed25519.Sign(key, signer.HeadPayload(site, headSeq, raw, hat, pin))
		b.Heads = []chainproof.Head{{SiteID: site, Seq: headSeq, EntryHash: eh, SignedAt: hat.Format(time.RFC3339Nano), Reason: "hourly", KeyID: pin, Signature: hex.EncodeToString(sig)}}
	}
	if mutate != nil {
		mutate(b)
	}
	var buf bytes.Buffer
	if err := b.Write(&buf); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "bundle.zip")
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return path, key, pin
}

func runCLI(args ...string) (int, string, string) {
	var out, errb bytes.Buffer
	code := run(args, &out, &errb, now)
	return code, out.String(), errb.String()
}

func TestExitCodes(t *testing.T) {
	good, key, pin := bundleFile(t, nil, 4, time.Hour)
	unsigned, _, upin := bundleFile(t, nil, 0, 0)
	stale, _, spin := bundleFile(t, nil, 2, 72*time.Hour)
	empty, _, epin2 := bundleFile(t, func(b *chainproof.Bundle) {
		b.Entries, b.Heads = nil, nil
		b.Manifest.FirstSeq, b.Manifest.LastSeq, b.Manifest.ChainTipSeq, b.Manifest.ChainTipHash = 0, 0, 0, ""
	}, 4, time.Hour)
	truncated, _, tpin := bundleFile(t, func(b *chainproof.Bundle) {
		b.Entries = b.Entries[:3]
		b.Manifest.LastSeq, b.Manifest.ChainTipSeq, b.Manifest.ChainTipHash = 3, 3, b.Entries[2].EntryHash
	}, 2, time.Hour)
	escape, _, spin2 := bundleFile(t, func(b *chainproof.Bundle) {
		b.Entries[1].Record, b.Entries[1].RecordState, b.Entries[1].NextSeq = "", chainproof.StateSuperseded, 5
	}, 4, time.Hour)
	edited, _, epin := bundleFile(t, func(b *chainproof.Bundle) { b.Entries[2].Record = `{"n": 99}` }, 4, time.Hour)

	for _, c := range []struct {
		name string
		args []string
		code int
		want string
	}{
		{"valid, range not checked", []string{"--pin", pin, "--no-range-check", good}, 0, "RESULT: VALID (range not checked)"},
		{"no flags is not exit 0", []string{"--pin", pin, good}, 11, "RANGE NOT AUTHENTICATED"},
		{"flags exclude each other", []string{"--pin", pin, "--no-range-check", "--expect-from", "2026-11-01", "--expect-to", "2026-11-30", good}, 2, ""},
		{"expect-to alone", []string{"--pin", pin, "--expect-to", "2026-11-30", good}, 2, ""},
		{"valid json", []string{"--pin", pin, "--no-range-check", "--json", good}, 0, `"status": "valid"`},
		{"trust first use", []string{"--trust-first-use", "--no-range-check", good}, 0, "FIRST USE"},
		{"no pin", []string{good}, 2, ""},
		{"bad pin", []string{"--pin", "abc", good}, 2, ""},
		{"no file", []string{"--pin", pin}, 2, ""},
		{"missing file", []string{"--pin", pin, "/nonexistent.zip"}, 3, ""},
		{"wrong fingerprint", []string{"--pin", strings.Repeat("0", 64), good}, 4, "RESULT: BROKEN"},
		{"unsigned", []string{"--pin", upin, unsigned}, 5, "RESULT: UNSIGNED"},
		{"stale", []string{"--pin", spin, stale}, 6, "RESULT: STALE"},
		{"stale check off is still an unsigned tail", []string{"--pin", spin, "--max-age", "0", stale}, 7, "UNSIGNED TAIL"},
		{"valid says what it does not prove", []string{"--pin", pin, "--no-range-check", good}, 0, "does NOT prove"},
		{"valid says the range is not authenticated", []string{"--pin", pin, "--no-range-check", good}, 0, "NOT AUTHENTICATED"},
		{"valid shows freshness", []string{"--pin", pin, "--no-range-check", good}, 0, "Exported     2026-11-02T00:00:00Z"},
		{"empty bundle", []string{"--pin", epin2, empty}, 9, "NO ENTRIES (NOT VALID, NOT PROVEN)"},
		{"truncated tail", []string{"--pin", tpin, truncated}, 7, "UNSIGNED TAIL"},
		{"superseded escape", []string{"--pin", spin2, "--no-range-check", escape}, 8, "UNVERIFIED RECORDS"},
		{"superseded escape, opt-out is still not exit 0", []string{"--pin", spin2, "--allow-unverified-records", "--no-range-check", escape}, 12, "1 of 4 entries (25%) were NOT checked"},
		{"superseded escape, opt-out alone", []string{"--pin", spin2, "--allow-unverified-records", escape}, 12, "UNVERIFIED RECORDS ACCEPTED"},
		{"range expected, head too old", []string{"--pin", pin, "--expect-from", "2026-11-01", "--expect-to", "2026-11-30", good}, 10, "RANGE MISMATCH"},
		{"only one expected date", []string{"--pin", pin, "--expect-from", "2026-11-01", good}, 2, ""},
		{"edited record", []string{"--pin", epin, edited}, 1, "First problem at seq 3, scan 00000000-0000-4000-8000-000000000003"},
	} {
		code, out, errs := runCLI(c.args...)
		if code != c.code || !strings.Contains(out, c.want) {
			t.Errorf("%s: exit %d (want %d)\nstdout: %s\nstderr: %s", c.name, code, c.code, out, errs)
		}
		if strings.Contains(out+errs, hex.EncodeToString(key.Seed())) {
			t.Errorf("%s: key material in the output", c.name)
		}
	}
	// A garbled file is exit 3, never a panic.
	bad := filepath.Join(t.TempDir(), "x.zip")
	_ = os.WriteFile(bad, []byte("PK garbage"), 0o600)
	if code, _, errs := runCLI("--pin", pin, bad); code != 3 || !strings.Contains(errs, "bundle") {
		t.Errorf("garbled file: exit %d %s", code, errs)
	}
}
