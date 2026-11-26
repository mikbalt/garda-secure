package chainproof

import (
	"archive/zip"
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"io"
	"testing"
	"time"

	"github.com/securepatrol/backend/internal/signer"
)

// swapFile returns zip a with file name taken from zip b.
func swapFile(t *testing.T, a, b []byte, name string) []byte {
	t.Helper()
	za, err := zip.NewReader(bytes.NewReader(a), int64(len(a)))
	if err != nil {
		t.Fatal(err)
	}
	zb, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	for _, f := range za.File {
		src := f
		if f.Name == name {
			for _, g := range zb.File {
				if g.Name == name {
					src = g
				}
			}
		}
		rc, err := src.Open()
		if err != nil {
			t.Fatal(err)
		}
		w, _ := zw.Create(f.Name)
		if _, err := io.Copy(w, rc); err != nil {
			t.Fatal(err)
		}
		_ = rc.Close()
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

// KeyEntry is a genesis keyring entry of key, as the signer writes it.
func KeyEntry(key ed25519.PrivateKey, at time.Time) Key {
	pub := key.Public().(ed25519.PublicKey)
	id := signer.KeyID(pub)
	at = at.UTC().Truncate(time.Microsecond)
	sig := ed25519.Sign(key, signer.KeyPayload(signer.KindGenesis, 1, id, pub, "", at))
	return Key{KeyID: id, Epoch: 1, Kind: signer.KindGenesis, PublicKey: hex.EncodeToString(pub), CreatedAt: at.Format(time.RFC3339Nano), NewKeySig: hex.EncodeToString(sig)}
}
