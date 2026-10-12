package pii

import (
	"errors"
	"strings"
	"testing"
)

func TestEncryptDecryptMask(t *testing.T) {
	c, err := For("test-secret")
	if err != nil {
		t.Fatal(err)
	}
	enc, err := c.Encrypt("3201234567890001")
	if err != nil || !IsEncrypted(enc) || strings.Contains(enc, "3201") {
		t.Fatalf("encrypt = %q, %v", enc, err)
	}
	enc2, _ := c.Encrypt("3201234567890001")
	if enc == enc2 {
		t.Fatal("nonce must differ per value")
	}
	if again, _ := c.Encrypt(enc); again != enc {
		t.Fatal("double encryption")
	}
	if p, err := c.Decrypt(enc); err != nil || p != "3201234567890001" {
		t.Fatalf("decrypt = %q, %v", p, err)
	}
	if p, _ := c.Decrypt("legacy-plain"); p != "legacy-plain" {
		t.Fatal("legacy plaintext must pass through")
	}
	other, _ := New("other-secret")
	if _, err := other.Decrypt(enc); !errors.Is(err, ErrDecrypt) {
		t.Fatalf("wrong key: %v", err)
	}
	if _, err := c.Decrypt(enc[:len(enc)-3]); !errors.Is(err, ErrDecrypt) {
		t.Fatal("truncated value accepted")
	}
	for in, want := range map[string]string{
		"3201234567890001": "3201********0001", "B1234XYZ": "******YZ", "AB": "**", "": "",
	} {
		if got := Mask(in); got != want {
			t.Errorf("Mask(%q) = %q, want %q", in, got, want)
		}
	}
}
