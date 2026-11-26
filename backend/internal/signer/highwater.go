package signer

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// HighWater is the signer's own record of what it has signed: an append-only
// file on the signer's volume, outside the database. The database owner can
// delete signed_heads; it cannot make the signer forget what is in this file.
// Each head is appended and fsynced BEFORE it is stored in signed_heads, and
// the signer refuses to go below or contradict anything in it. The file also
// feeds the export for offline anchoring (TAQ-22).
type HighWater struct {
	path  string
	mu    sync.Mutex
	heads []SignedHead
}

// jsonHead is the line format of the file and of the export.
type jsonHead struct {
	SiteID    string `json:"site_id"`
	Seq       int64  `json:"seq"`
	EntryHash string `json:"entry_hash"`
	SignedAt  string `json:"signed_at"`
	Reason    string `json:"reason"`
	KeyID     string `json:"key_id"`
	Signature string `json:"signature"`
}

func toJSON(h SignedHead) jsonHead {
	return jsonHead{h.SiteID, h.Seq, hex.EncodeToString(h.EntryHash), h.SignedAt.UTC().Format(time.RFC3339Nano),
		h.Reason, h.KeyID, hex.EncodeToString(h.Signature)}
}

func fromJSON(j jsonHead) (SignedHead, error) {
	hash, err1 := hex.DecodeString(j.EntryHash)
	sig, err2 := hex.DecodeString(j.Signature)
	at, err3 := time.Parse(time.RFC3339Nano, j.SignedAt)
	if err1 != nil || err2 != nil || err3 != nil {
		return SignedHead{}, fmt.Errorf("malformed head record for seq %d", j.Seq)
	}
	return SignedHead{SiteID: j.SiteID, Seq: j.Seq, EntryHash: hash, SignedAt: at, Reason: j.Reason, KeyID: j.KeyID, Signature: sig}, nil
}

// LoadHighWater reads the file; a missing file is an empty record. A line that
// cannot be read is an error: the signer must not start on a damaged record.
func LoadHighWater(path string) (*HighWater, error) {
	hw := &HighWater{path: path}
	raw, err := os.ReadFile(path) // #nosec G304 -- path is operator-supplied config
	if os.IsNotExist(err) {
		return hw, nil
	}
	if err != nil {
		return nil, fmt.Errorf("high-water file: %w", err)
	}
	// A crash in the middle of an append leaves a last line without its newline.
	// That head was never stored (the insert comes after the sync) and nothing
	// was signed from it, so it is dropped; any other damage is an error.
	if i := bytes.LastIndexByte(raw, '\n'); len(raw) > 0 && raw[len(raw)-1] != '\n' {
		raw = raw[:i+1]
		if err := os.Truncate(path, int64(len(raw))); err != nil {
			return nil, fmt.Errorf("high-water file: %w", err)
		}
	}
	sc := bufio.NewScanner(bytes.NewReader(raw))
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for n := 1; sc.Scan(); n++ {
		if len(bytes.TrimSpace(sc.Bytes())) == 0 {
			continue
		}
		var j jsonHead
		if err := json.Unmarshal(sc.Bytes(), &j); err != nil {
			return nil, fmt.Errorf("high-water file line %d: %w", n, err)
		}
		h, err := fromJSON(j)
		if err != nil {
			return nil, fmt.Errorf("high-water file line %d: %w", n, err)
		}
		hw.heads = append(hw.heads, h)
	}
	return hw, sc.Err()
}

// Heads returns every head in the file, oldest first.
func (hw *HighWater) Heads() []SignedHead {
	hw.mu.Lock()
	defer hw.mu.Unlock()
	return append([]SignedHead(nil), hw.heads...)
}

// ForSite returns the heads of one site, oldest first.
func (hw *HighWater) ForSite(site string) []SignedHead {
	var out []SignedHead
	for _, h := range hw.Heads() {
		if h.SiteID == site {
			out = append(out, h)
		}
	}
	return out
}

// Append writes a head to the file and syncs it to disk before returning.
func (hw *HighWater) Append(h SignedHead) error {
	line, err := json.Marshal(toJSON(h))
	if err != nil {
		return err
	}
	hw.mu.Lock()
	defer hw.mu.Unlock()
	f, err := os.OpenFile(hw.path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600) // #nosec G304 -- path is operator-supplied config
	if err != nil {
		return fmt.Errorf("high-water file: %w", err)
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		_ = f.Close()
		return fmt.Errorf("high-water file: %w", err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return fmt.Errorf("high-water file sync: %w", err)
	}
	if err := f.Close(); err != nil {
		return err
	}
	hw.heads = append(hw.heads, h)
	return nil
}
