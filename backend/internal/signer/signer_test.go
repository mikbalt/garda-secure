package signer

import (
	"crypto/ed25519"
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestKeyFileIs0400AndNeverOverwritten(t *testing.T) {
	path := filepath.Join(t.TempDir(), "signer.key")
	pub, err := GenerateKeyFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if st, _ := os.Stat(path); st.Mode().Perm() != 0o400 {
		t.Fatalf("key mode = %v, want 0400", st.Mode().Perm())
	}
	if _, err := GenerateKeyFile(path); err == nil {
		t.Fatal("an existing key was overwritten")
	}
	k, err := LoadKeyFile(path)
	if err != nil || !pub.Equal(pubOf(k)) {
		t.Fatalf("load: %v", err)
	}
	if err := os.Chmod(path, 0o440); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadKeyFile(path); err == nil {
		t.Fatal("a group-readable key file was accepted")
	}
}

func TestDueReason(t *testing.T) {
	jkt := time.FixedZone("WIB", 7*3600)
	at := func(h, m int) time.Time { return time.Date(2026, 11, 26, h, m, 0, 0, jkt) }
	seq := func(n int64) *int64 { return &n }
	for _, c := range []struct {
		name string
		now  time.Time
		head int64
		last *int64
		when time.Time
		want string
	}{
		{"never signed", at(10, 0), 5, nil, time.Time{}, ReasonHourly},
		{"head did not move", at(23, 59), 5, seq(5), at(1, 0), ""},
		{"moved, signed 30 min ago", at(10, 30), 6, seq(5), at(10, 0), ""},
		{"moved, signed an hour ago", at(11, 0), 6, seq(5), at(10, 0), ReasonHourly},
		{"moved, signed before local midnight", at(0, 1), 6, seq(5), at(23, 30).AddDate(0, 0, -1), ReasonDayEnd},
	} {
		if got := dueReason(c.now, jkt, c.head, c.last, c.when); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}

func key(t *testing.T) ed25519.PrivateKey {
	t.Helper()
	_, k, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func TestKeyringTrust(t *testing.T) {
	now := time.Now()
	a, b, evil := key(t), key(t), key(t)
	idA, idB := KeyID(pubOf(a)), KeyID(pubOf(b))
	genesis := newEntry(KindGenesis, 1, a, "", "", now)
	rot := newEntry(KindRotation, 1, b, idA, "", now)
	rot.PrevKeySig = ed25519.Sign(a, rot.payload())

	trust := verifyKeyring([]KeyringEntry{genesis, rot}, nil)
	if !trust[idA].trusted || !trust[idB].trusted {
		t.Fatalf("honest handover not trusted: %+v %+v", trust[idA], trust[idB])
	}
	if trust[idA].retiredAt.IsZero() {
		t.Fatal("rotated-out key is not retired")
	}

	// The new key alone cannot take over: the handover signature must come from the old key.
	forged := newEntry(KindRotation, 1, evil, idA, "", now)
	forged.PrevKeySig = ed25519.Sign(evil, forged.payload())
	if tr := verifyKeyring([]KeyringEntry{genesis, forged}, nil); tr[KeyID(pubOf(evil))].trusted {
		t.Fatal("rotation without the old key's signature was trusted")
	}
	// A new epoch needs a pin; a pinned genesis alone is not enough.
	epoch := newEntry(KindNewEpoch, 2, evil, idA, "leaked", now)
	idE := KeyID(pubOf(evil))
	if verifyKeyring([]KeyringEntry{genesis, epoch}, []string{idA})[idE].trusted {
		t.Fatal("new epoch trusted without a pin")
	}
	if !verifyKeyring([]KeyringEntry{genesis, epoch}, []string{idA, idE})[idE].trusted {
		t.Fatal("pinned new epoch not trusted")
	}
	// A swapped public key under a known key id is rejected.
	tampered := genesis
	tampered.PublicKey = pubOf(evil)
	if verifyKeyring([]KeyringEntry{tampered}, nil)[idA].trusted {
		t.Fatal("tampered keyring entry trusted")
	}
}

func TestHighWaterSurvivesARestartAndRefusesDamage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hw.jsonl")
	hw, err := LoadHighWater(path)
	if err != nil {
		t.Fatal(err)
	}
	h := SignedHead{SiteID: "6f1c0a52-0a8c-4a39-8f0e-0e8f2f3f0001", Seq: 7, EntryHash: make([]byte, 32), SignedAt: time.Now().UTC().Truncate(time.Microsecond), Reason: ReasonHourly, KeyID: "k", Signature: make([]byte, 64)}
	if err := hw.Append(h); err != nil {
		t.Fatal(err)
	}
	again, err := LoadHighWater(path)
	if err != nil || len(again.Heads()) != 1 || again.Heads()[0].Seq != 7 || !again.Heads()[0].SignedAt.Equal(h.SignedAt) {
		t.Fatalf("reload: %+v %v", again, err)
	}
	if st, _ := os.Stat(path); st.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v, want 0600", st.Mode().Perm())
	}
	// A torn last line (crash in the middle of an append) is dropped; the file is cut back to the last complete line.
	f, _ := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o600)
	_, _ = f.WriteString(`{"site_id":"x","seq":8,"entry_ha`)
	_ = f.Close()
	torn, err := LoadHighWater(path)
	if err != nil || len(torn.Heads()) != 1 {
		t.Fatalf("torn last line: %v %v, want the first head only", torn, err)
	}
	h.Seq = 9
	if err := torn.Append(h); err != nil {
		t.Fatal(err)
	}
	if reloaded, err := LoadHighWater(path); err != nil || len(reloaded.Heads()) != 2 {
		t.Fatalf("after appending to a torn file: %v %v, want 2 heads", reloaded, err)
	}
	if err := os.WriteFile(path, []byte("{not json}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadHighWater(path); err == nil {
		t.Fatal("a damaged record was accepted")
	}
}

func TestHeadPayloadIsCanonicalForTheSiteID(t *testing.T) {
	a := HeadPayload("6F1C0A52-0A8C-4A39-8F0E-0E8F2F3F0001", 1, []byte{1}, time.Unix(0, 0), "k")
	b := HeadPayload("6f1c0a52-0a8c-4a39-8f0e-0e8f2f3f0001", 1, []byte{1}, time.Unix(0, 0), "k")
	if string(a) != string(b) {
		t.Fatal("payload depends on the case of the site id")
	}
	if _, err := canonSite("6F1C0A52-0A8C-4A39-8F0E-0E8F2F3F0001"); err != nil {
		t.Fatal(err)
	}
	if _, err := canonSite("6f1c0a520a8c4a398f0e0e8f2f3f0001"); err == nil {
		t.Fatal("non-hyphenated uuid accepted")
	}
}

func TestKeyringForkAndPinnedReachability(t *testing.T) {
	now := time.Now()
	a, b, c := key(t), key(t), key(t)
	idA, idB, idC := KeyID(pubOf(a)), KeyID(pubOf(b)), KeyID(pubOf(c))
	genesis := newEntry(KindGenesis, 1, a, "", "", now)
	rb := newEntry(KindRotation, 1, b, idA, "", now)
	rb.PrevKeySig = ed25519.Sign(a, rb.payload())
	rc := newEntry(KindRotation, 1, c, idA, "", now)
	rc.PrevKeySig = ed25519.Sign(a, rc.payload())

	tr := verifyKeyring([]KeyringEntry{genesis, rb, rc}, []string{idA})
	if tr[idB].trusted || tr[idC].trusted {
		t.Fatal("a forked handover was trusted")
	}
	if !tr[idA].trusted {
		t.Fatal("the pinned root is not trusted")
	}
	// A client that pinned one successor explicitly keeps trusting it; the other stays out.
	tr = verifyKeyring([]KeyringEntry{genesis, rb, rc}, []string{idA, idB})
	if !tr[idB].trusted || tr[idC].trusted {
		t.Fatalf("explicit pin of one branch: B=%v C=%v", tr[idB].trusted, tr[idC].trusted)
	}
	// Nothing pinned is reachable: even a self-consistent chain is refused.
	tr = verifyKeyring([]KeyringEntry{genesis, rb}, []string{idC})
	if tr[idA].trusted || tr[idB].trusted {
		t.Fatal("keys not reachable from a pinned key were trusted")
	}
}
