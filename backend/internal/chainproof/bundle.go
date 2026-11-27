// Package chainproof checks the record chain and its signed heads from an
// export bundle, with no database and no network (TAQ-18). The same code runs
// in the API (online check of a site and date range) and in the offline CLI
// cmd/garda-verify, so both give the same answer for the same data.
package chainproof

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/securepatrol/backend/internal/signer"
)

// Format names the bundle layout; docs/chain-verification.md describes it.
const Format = "garda-chain-bundle/v1"

// Files of a bundle (a zip). manifest.json lists the sha256 of the other three.
const (
	FileManifest = "manifest.json"
	FileEntries  = "entries.jsonl"
	FileHeads    = "heads.json"
	FileKeyring  = "keyring.json"
	FileReadme   = "README.txt"
)

// Record states of an entry (see Entry.RecordState).
const (
	StateLatest     = "latest"     // newest entry of its record: Record holds the record text
	StateSuperseded = "superseded" // a later entry of the same record exists (NextSeq)
	StateDeleted    = "deleted"    // newest entry of its record, but the record no longer exists
)

// Manifest describes what the bundle covers. It is not signed: a bundle edited
// together with its manifest is caught by the chain and the signed heads, and
// the sha256 of manifest.json (BundleSHA256) can be noted down out of band.
type Manifest struct {
	Format       string `json:"format"`
	SiteID       string `json:"site_id"`
	SiteTimezone string `json:"site_timezone"`
	From         string `json:"from"` // requested range, UTC, from <= created_at < to
	To           string `json:"to"`
	ExportedAt   string `json:"exported_at"`
	// The entries of the bundle are FirstSeq..LastSeq without gaps (none when
	// Entries is 0). AnchorSeq is where the check starts: 0 = the site's
	// genesis value, otherwise the seq of a signed head in heads.json whose
	// entry hash is the prev_hash of FirstSeq. The bundle ends at the first
	// signed head at or after the range (or at the chain's end).
	AnchorSeq int64 `json:"anchor_seq"`
	FirstSeq  int64 `json:"first_seq"`
	LastSeq   int64 `json:"last_seq"`
	Entries   int   `json:"entries"`
	Heads     int   `json:"heads"`
	// The chain's own head when exported (record_chain_heads).
	ChainTipSeq  int64  `json:"chain_tip_seq"`
	ChainTipHash string `json:"chain_tip_hash"`
	// What came of asking the signer for a fresh head when the bundle was
	// exported (signer.Outcome status, and the reason when it did not sign).
	// Only an explanation for a reader: the verdict comes from the signed heads.
	Signing       string            `json:"signing,omitempty"`
	SigningReason string            `json:"signing_reason,omitempty"`
	Files         map[string]string `json:"files"` // name -> hex sha256
}

// Entry is one row of record_chain plus what a third party needs to recompute
// its hashes: the canonical text of the record (only the newest entry of a
// record can have it: older states are not kept).
type Entry struct {
	Seq         int64  `json:"seq"`
	Kind        string `json:"kind"`
	RecordID    string `json:"record_id"`
	RecordHash  string `json:"record_hash"`
	PrevHash    string `json:"prev_hash"`
	EntryHash   string `json:"entry_hash"`
	HashVersion int    `json:"hash_version"`
	Origin      string `json:"origin"`
	CreatedAt   string `json:"created_at"` // UTC, microseconds
	RecordState string `json:"record_state"`
	NextSeq     int64  `json:"next_seq,omitempty"` // StateSuperseded: seq of the record's next entry
	Record      string `json:"record,omitempty"`   // StateLatest: "<kind>:" + Record is what record_hash hashes
}

// Head is a signed head, in the format of `signer export`.
type Head struct {
	SiteID    string `json:"site_id"`
	Seq       int64  `json:"seq"`
	EntryHash string `json:"entry_hash"`
	SignedAt  string `json:"signed_at"`
	Reason    string `json:"reason"`
	KeyID     string `json:"key_id"`
	Signature string `json:"signature"`
}

// Key is a keyring entry (public keys and signatures only), in the format of `signer export`.
type Key struct {
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

// Bundle is the decoded export.
type Bundle struct {
	Manifest Manifest
	Entries  []Entry
	Heads    []Head
	Keyring  []Key
	// ManifestSHA256 is set by Write and Read: the integrity hash of the bundle.
	ManifestSHA256 string
}

// NoteSigning records the outcome of the sign-on-export request in the manifest.
func (b *Bundle) NoteSigning(o signer.Outcome) {
	b.Manifest.Signing, b.Manifest.SigningReason = o.Status, o.Reason
}

// ErrBundle marks a bundle that cannot be read or fails its integrity check.
var ErrBundle = errors.New("bundle")

func bundleErr(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrBundle, fmt.Sprintf(format, a...))
}

const readme = `GARDA chain export bundle (garda-chain-bundle/v1).
Check it offline with: garda-verify --pin <signer key fingerprint> <this file>
Files: manifest.json (range and file hashes), entries.jsonl (chain entries and record text),
heads.json (signed heads), keyring.json (public keys of the signer). No private key is in here.
`

// Write encodes the bundle as a zip. It fills Manifest.Files, Entries, Heads
// and ManifestSHA256.
func (b *Bundle) Write(w io.Writer) error {
	var ent bytes.Buffer
	enc := json.NewEncoder(&ent)
	for _, e := range b.Entries {
		if err := enc.Encode(e); err != nil {
			return err
		}
	}
	heads, err := marshal(nonNil(b.Heads))
	if err != nil {
		return err
	}
	keys, err := marshal(nonNil(b.Keyring))
	if err != nil {
		return err
	}
	b.Manifest.Format = Format
	b.Manifest.Entries, b.Manifest.Heads = len(b.Entries), len(b.Heads)
	b.Manifest.Files = map[string]string{FileEntries: sum(ent.Bytes()), FileHeads: sum(heads), FileKeyring: sum(keys)}
	man, err := marshal(b.Manifest)
	if err != nil {
		return err
	}
	b.ManifestSHA256 = sum(man)

	modified, _ := time.Parse(time.RFC3339Nano, b.Manifest.ExportedAt)
	zw := zip.NewWriter(w)
	for _, f := range []struct {
		name string
		data []byte
	}{{FileManifest, man}, {FileEntries, ent.Bytes()}, {FileHeads, heads}, {FileKeyring, keys}, {FileReadme, []byte(readme)}} {
		h, err := zw.CreateHeader(&zip.FileHeader{Name: f.name, Method: zip.Deflate, Modified: modified})
		if err != nil {
			return err
		}
		if _, err := h.Write(f.data); err != nil {
			return err
		}
	}
	return zw.Close()
}

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

func marshal(v any) ([]byte, error) { return json.MarshalIndent(v, "", "  ") }

func sum(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// maxFile bounds what Read inflates from one file of the zip.
const maxFile = 1 << 30

// Read decodes a bundle and checks the file hashes of its manifest. Any
// problem is an ErrBundle.
func Read(data []byte) (*Bundle, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, bundleErr("not a zip file: %v", err)
	}
	files := map[string][]byte{}
	for _, f := range zr.File {
		if f.Name != FileManifest && f.Name != FileEntries && f.Name != FileHeads && f.Name != FileKeyring {
			continue
		}
		if f.UncompressedSize64 > maxFile {
			return nil, bundleErr("%s is too large", f.Name)
		}
		rc, err := f.Open()
		if err != nil {
			return nil, bundleErr("%s: %v", f.Name, err)
		}
		raw, err := io.ReadAll(io.LimitReader(rc, maxFile+1))
		_ = rc.Close()
		if err != nil || len(raw) > maxFile {
			return nil, bundleErr("%s cannot be read", f.Name)
		}
		files[f.Name] = raw
	}
	for _, n := range []string{FileManifest, FileEntries, FileHeads, FileKeyring} {
		if _, ok := files[n]; !ok {
			return nil, bundleErr("%s is missing", n)
		}
	}
	b := &Bundle{ManifestSHA256: sum(files[FileManifest])}
	if err := strictJSON(files[FileManifest], &b.Manifest); err != nil {
		return nil, bundleErr("manifest.json: %v", err)
	}
	if b.Manifest.Format != Format {
		return nil, bundleErr("unsupported format %q (this tool reads %s)", b.Manifest.Format, Format)
	}
	for _, n := range []string{FileEntries, FileHeads, FileKeyring} {
		if want := b.Manifest.Files[n]; want == "" || want != sum(files[n]) {
			return nil, bundleErr("%s does not match the hash in the manifest (the file was changed after the export)", n)
		}
	}
	for i, line := range bytes.Split(files[FileEntries], []byte("\n")) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var e Entry
		if err := strictJSON(line, &e); err != nil {
			return nil, bundleErr("entries.jsonl line %d: %v", i+1, err)
		}
		b.Entries = append(b.Entries, e)
	}
	if err := strictJSON(files[FileHeads], &b.Heads); err != nil {
		return nil, bundleErr("heads.json: %v", err)
	}
	if err := strictJSON(files[FileKeyring], &b.Keyring); err != nil {
		return nil, bundleErr("keyring.json: %v", err)
	}
	if len(b.Entries) != b.Manifest.Entries || len(b.Heads) != b.Manifest.Heads {
		return nil, bundleErr("the manifest says %d entries and %d heads, the files hold %d and %d",
			b.Manifest.Entries, b.Manifest.Heads, len(b.Entries), len(b.Heads))
	}
	return b, nil
}

func strictJSON(raw []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

func (h Head) toSigner() (signer.SignedHead, error) {
	eh, err := hex.DecodeString(h.EntryHash)
	if err != nil || len(eh) != sha256.Size {
		return signer.SignedHead{}, errors.New("entry_hash is not a sha256 hex")
	}
	sig, err := hex.DecodeString(h.Signature)
	if err != nil {
		return signer.SignedHead{}, errors.New("signature is not hex")
	}
	at, err := time.Parse(time.RFC3339Nano, h.SignedAt)
	if err != nil {
		return signer.SignedHead{}, errors.New("signed_at is not a time")
	}
	return signer.SignedHead{SiteID: strings.ToLower(h.SiteID), Seq: h.Seq, EntryHash: eh, SignedAt: at, Reason: h.Reason, KeyID: h.KeyID, Signature: sig}, nil
}

func (k Key) toSigner() (signer.KeyringEntry, error) {
	pub, err := hex.DecodeString(k.PublicKey)
	if err != nil {
		return signer.KeyringEntry{}, errors.New("public_key is not hex")
	}
	at, err := time.Parse(time.RFC3339Nano, k.CreatedAt)
	if err != nil {
		return signer.KeyringEntry{}, errors.New("created_at is not a time")
	}
	nks, err := hex.DecodeString(k.NewKeySig)
	if err != nil {
		return signer.KeyringEntry{}, errors.New("new_key_sig is not hex")
	}
	var pks []byte
	if k.PrevKeySig != "" {
		if pks, err = hex.DecodeString(k.PrevKeySig); err != nil {
			return signer.KeyringEntry{}, errors.New("prev_key_sig is not hex")
		}
	}
	return signer.KeyringEntry{KeyID: k.KeyID, Epoch: k.Epoch, Kind: k.Kind, PublicKey: pub, PrevKeyID: k.PrevKeyID,
		Reason: k.Reason, CreatedAt: at, NewKeySig: nks, PrevKeySig: pks}, nil
}
