package signer

import (
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// Heads and keyring entries are signed over fixed text, so a verifier in any
// language can rebuild it. Times are UTC with microseconds (what Postgres keeps).
const tsLayout = "2006-01-02T15:04:05.000000Z"

func ts(t time.Time) string { return t.UTC().Format(tsLayout) }

// HeadPayload is the signed text of a head:
// garda-head:v1|site|seq|entry_hash hex|signed_at|key_id.
func HeadPayload(siteID string, seq int64, entryHash []byte, signedAt time.Time, keyID string) []byte {
	return []byte(fmt.Sprintf("garda-head:v1|%s|%d|%s|%s|%s", strings.ToLower(siteID), seq, hex.EncodeToString(entryHash), ts(signedAt), keyID))
}

// KeyPayload is the signed text of a keyring entry:
// garda-key:v1|kind|epoch|key_id|public key hex|prev_key_id|created_at.
// A rotation is signed by the new key and by the previous key over this text.
func KeyPayload(kind string, epoch int, keyID string, pub []byte, prevKeyID string, createdAt time.Time) []byte {
	return []byte(fmt.Sprintf("garda-key:v1|%s|%d|%s|%s|%s|%s", kind, epoch, keyID, hex.EncodeToString(pub), prevKeyID, ts(createdAt)))
}

func verify(pub ed25519.PublicKey, msg, sig []byte) bool {
	return len(pub) == ed25519.PublicKeySize && ed25519.Verify(pub, msg, sig)
}
