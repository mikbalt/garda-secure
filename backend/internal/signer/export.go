package signer

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"io"
	"time"
)

type jsonKey struct {
	KeyID      string `json:"key_id"`
	Epoch      int    `json:"epoch"`
	Kind       string `json:"kind"`
	PublicKey  string `json:"public_key"`
	PrevKeyID  string `json:"prev_key_id,omitempty"`
	Reason     string `json:"reason,omitempty"`
	CreatedAt  string `json:"created_at"`
	NewKeySig  string `json:"new_key_sig"`
	PrevKeySig string `json:"prev_key_sig,omitempty"`
}

// Export writes the signed heads (from the signer's own record on its volume,
// not from the database) and the keyring as JSON, for offline anchoring: copy
// it off the box (TAQ-22) and a later rewrite of the database cannot reach it.
// site "" = all sites. The payload to verify a head is HeadPayload.
func (s *Signer) Export(ctx context.Context, w io.Writer, site string) error {
	heads := []jsonHead{}
	for _, h := range s.State.Heads() {
		if site == "" || h.SiteID == site {
			heads = append(heads, toJSON(h))
		}
	}
	ring, err := loadKeyring(ctx, s.DB)
	if err != nil {
		return err
	}
	keys := []jsonKey{}
	for _, e := range ring {
		k := jsonKey{KeyID: e.KeyID, Epoch: e.Epoch, Kind: e.Kind, PublicKey: hex.EncodeToString(e.PublicKey), PrevKeyID: e.PrevKeyID,
			Reason: e.Reason, CreatedAt: e.CreatedAt.UTC().Format(time.RFC3339Nano), NewKeySig: hex.EncodeToString(e.NewKeySig)}
		if e.PrevKeySig != nil {
			k.PrevKeySig = hex.EncodeToString(e.PrevKeySig)
		}
		keys = append(keys, k)
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(map[string]any{"format": "garda-signed-heads/v1", "exported_at": s.Now().UTC().Format(time.RFC3339Nano), "heads": heads, "keyring": keys})
}
