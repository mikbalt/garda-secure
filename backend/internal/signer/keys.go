package signer

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
)

// KeyID is the fingerprint of a public key: hex sha256 of the 32 key bytes.
// Clients pin it out of band.
func KeyID(pub ed25519.PublicKey) string {
	sum := sha256.Sum256(pub)
	return hex.EncodeToString(sum[:])
}

func pubOf(k ed25519.PrivateKey) ed25519.PublicKey { return k.Public().(ed25519.PublicKey) }

// GenerateKeyFile creates a new Ed25519 key at path (hex seed, mode 0400) and
// returns its public key. It never overwrites an existing file. Run it at
// install time, on the signer's own volume; the private key is never printed.
func GenerateKeyFile(path string) (ed25519.PublicKey, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o400) // #nosec G304 -- path is operator-supplied config
	if err != nil {
		return nil, fmt.Errorf("create key file: %w", err)
	}
	if _, err := f.WriteString(hex.EncodeToString(priv.Seed()) + "\n"); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return nil, err
	}
	if err := f.Close(); err != nil {
		return nil, err
	}
	return pub, nil
}

// LoadKeyFile reads a key written by GenerateKeyFile. It refuses a file that
// group or others can access.
func LoadKeyFile(path string) (ed25519.PrivateKey, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("key file: %w", err)
	}
	if !st.Mode().IsRegular() || st.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("key file %s must be a regular file with mode 0400 (is %v)", path, st.Mode())
	}
	raw, err := os.ReadFile(path) // #nosec G304 -- path is operator-supplied config
	if err != nil {
		return nil, fmt.Errorf("key file: %w", err)
	}
	seed, err := hex.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil || len(seed) != ed25519.SeedSize {
		return nil, errors.New("key file: not a hex-encoded Ed25519 seed")
	}
	return ed25519.NewKeyFromSeed(seed), nil
}
