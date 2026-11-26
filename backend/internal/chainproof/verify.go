package chainproof

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/securepatrol/backend/internal/signer"
)

// What a verification concluded.
const (
	StatusValid    = "valid"
	StatusBroken   = "broken"         // an entry, a record or a signed head does not hold
	StatusBundle   = "bundle_invalid" // the bundle itself is unusable
	StatusUnsigned = "unsigned"       // entries, but no signed head at all
	StatusStale    = "stale"          // entries after the newest signed head, which is older than the limit
)

// Where a break is (Break.Class). The CLI exit code follows the class.
const (
	ClassChain  = "chain"  // links, gaps, entry content
	ClassRecord = "record" // the record no longer matches its entry
	ClassHead   = "head"   // a signed head, its key or its signature
	ClassBundle = "bundle"
)

// tsLayout: UTC, microseconds, as the signer writes times.
const tsLayout = "2006-01-02T15:04:05.000000Z"

// Break is the first record that does not hold, by seq.
type Break struct {
	Seq    int64  `json:"seq"`
	Kind   string `json:"kind"`   // attendance, scan, incident, incident_amendment, or signed_head
	ID     string `json:"id"`     // the record id (signed_head: the key id)
	Reason string `json:"reason"` // plain words
	Class  string `json:"class"`
}

// Result is the outcome of Verify.
type Result struct {
	Status string `json:"status"`
	Break  *Break `json:"first_break,omitempty"`

	SiteID    string `json:"site_id"`
	Entries   int    `json:"entries_checked"`
	Heads     int    `json:"heads_checked"`
	AnchorSeq int64  `json:"anchor_seq"` // 0 = checked from the site's first entry
	FirstSeq  int64  `json:"first_seq"`
	LastSeq   int64  `json:"last_seq"`
	// NewestHeadSeq is the newest signed head of the bundle (0: none).
	NewestHeadSeq int64 `json:"newest_head_seq"`
	// RecordsNotChecked counts entries whose record changed again after the
	// range: they are checked as links of the chain, not against record text.
	RecordsNotChecked int `json:"records_not_checked"`
}

// Exit codes of garda-verify, one per failure class.
const (
	CodeValid    = 0
	CodeChain    = 1 // an entry or a record was changed, deleted or reordered
	CodeUsage    = 2 // not a verification result: bad arguments
	CodeBundle   = 3 // the bundle is unreadable or fails its integrity check
	CodeHead     = 4 // a signed head, a key or a signature does not hold (also: wrong fingerprint)
	CodeUnsigned = 5
	CodeStale    = 6
)

// Code is the process exit code for the result.
func (r Result) Code() int {
	switch r.Status {
	case StatusValid:
		return CodeValid
	case StatusUnsigned:
		return CodeUnsigned
	case StatusStale:
		return CodeStale
	}
	if r.Break == nil {
		return CodeChain
	}
	switch r.Break.Class {
	case ClassHead:
		return CodeHead
	case ClassBundle:
		return CodeBundle
	}
	return CodeChain
}

// Options of Verify.
type Options struct {
	// Pins are the signer key fingerprints the client holds out of band. With
	// none, only the first genesis key of the keyring is trusted (trust on first use).
	Pins []string
	// MaxAge > 0: entries after the newest signed head fail as stale when that head is older.
	MaxAge time.Duration
	Now    time.Time // zero = time.Now()
}

// Genesis is the value the first entry of a site links to.
func Genesis(siteID string) []byte {
	s := sha256.Sum256([]byte("garda-record-chain:v1:" + strings.ToLower(siteID)))
	return s[:]
}

// EntryHash recomputes the hash of an entry (hash_version 1, migration 011):
// sha256 of version|site|seq|kind|record id|record_hash|prev_hash|origin|created_at.
func EntryHash(siteID string, e Entry) ([]byte, error) {
	if e.HashVersion != 1 {
		return nil, fmt.Errorf("hash_version %d is not supported by this tool (it checks version 1)", e.HashVersion)
	}
	rh, err1 := hex.DecodeString(e.RecordHash)
	ph, err2 := hex.DecodeString(e.PrevHash)
	if err1 != nil || err2 != nil || len(rh) != sha256.Size || len(ph) != sha256.Size {
		return nil, errors.New("record_hash or prev_hash is not a sha256 hex")
	}
	at, err := time.Parse(time.RFC3339Nano, e.CreatedAt)
	if err != nil {
		return nil, errors.New("created_at is not a time")
	}
	text := fmt.Sprintf("%d|%s|%d|%s|%s|%s|%s|%s|%s", e.HashVersion, strings.ToLower(siteID), e.Seq, e.Kind, strings.ToLower(e.RecordID),
		hex.EncodeToString(rh), hex.EncodeToString(ph), e.Origin, at.UTC().Format("2006-01-02T15:04:05.000000"))
	s := sha256.Sum256([]byte(text))
	return s[:], nil
}

func recordHash(e Entry) string {
	s := sha256.Sum256([]byte(e.Kind + ":" + e.Record))
	return hex.EncodeToString(s[:])
}

func chainBreak(e Entry, class, format string, a ...any) *Break {
	return &Break{Seq: e.Seq, Kind: e.Kind, ID: e.RecordID, Reason: fmt.Sprintf(format, a...), Class: class}
}

// Verify checks a bundle: the chain from its anchor, every record that still
// has its text, and the signed heads against the keys the pins vouch for. It
// returns the first problem by seq. A broken chain or head outranks UNSIGNED
// and STALE, which are only reported for a chain that is otherwise intact.
func Verify(b *Bundle, o Options) Result {
	m := b.Manifest
	res := Result{Status: StatusBroken, SiteID: m.SiteID, AnchorSeq: m.AnchorSeq}
	bundleBad := func(format string, a ...any) Result {
		res.Status = StatusBundle
		res.Break = &Break{Reason: fmt.Sprintf(format, a...), Class: ClassBundle}
		return res
	}
	site, err := uuid.Parse(m.SiteID)
	if err != nil || len(m.SiteID) != 36 || site.String() != m.SiteID {
		return bundleBad("site_id is not a canonical (lowercase, hyphenated) uuid")
	}
	if m.AnchorSeq < 0 {
		return bundleBad("anchor_seq is negative")
	}
	now := o.Now
	if now.IsZero() {
		now = time.Now()
	}

	ring := make([]signer.KeyringEntry, 0, len(b.Keyring))
	for _, k := range b.Keyring {
		e, err := k.toSigner()
		if err != nil {
			return bundleBad("keyring entry %s: %v", k.KeyID, err)
		}
		ring = append(ring, e)
	}
	trust := signer.NewTrust(ring, o.Pins)

	// Heads: lowest seq with a problem, and the entry hash each one claims.
	type head struct {
		signer.SignedHead
		problem string
	}
	var heads []head
	for _, h := range b.Heads {
		sh, err := h.toSigner()
		if err != nil {
			return bundleBad("signed head at seq %d: %v", h.Seq, err)
		}
		hd := head{SignedHead: sh}
		if sh.SiteID != m.SiteID {
			hd.problem = "head is for another site"
		} else {
			hd.problem = trust.HeadProblem(sh)
		}
		if sh.Seq >= m.AnchorSeq {
			heads = append(heads, hd)
		}
	}
	var headBreak *Break
	setHead := func(seq int64, keyID, reason string) {
		if headBreak == nil || seq < headBreak.Seq {
			headBreak = &Break{Seq: seq, Kind: "signed_head", ID: keyID, Reason: reason, Class: ClassHead}
		}
	}

	// The anchor: genesis, or a signed head the entries continue from.
	prev := Genesis(m.SiteID)
	if m.AnchorSeq > 0 {
		var anchor *head
		for i := range heads {
			if heads[i].Seq == m.AnchorSeq {
				anchor = &heads[i]
				break
			}
		}
		if anchor == nil {
			return bundleBad("the signed head at the start of the range (seq %d) is not in the bundle", m.AnchorSeq)
		}
		prev = anchor.EntryHash
	}

	var chain *Break
	bySeq := map[int64]Entry{}
	expect := m.AnchorSeq + 1
	for _, e := range b.Entries {
		bySeq[e.Seq] = e
		if chain != nil {
			continue
		}
		if e.Seq != expect {
			chain = chainBreak(e, ClassChain, "entry missing")
			chain.Seq, chain.Kind, chain.ID = expect, "", ""
			continue
		}
		ph, _ := hex.DecodeString(e.PrevHash)
		eh, err := EntryHash(m.SiteID, e)
		switch {
		case e.HashVersion != 1:
			chain = chainBreak(e, ClassChain, "hash_version %d is not supported by this tool (it checks version 1)", e.HashVersion)
		case string(ph) != string(prev):
			chain = chainBreak(e, ClassChain, "prev_hash does not link to the previous entry")
		case err != nil || hex.EncodeToString(eh) != strings.ToLower(e.EntryHash):
			chain = chainBreak(e, ClassChain, "entry content does not match its hash")
		default:
			switch e.RecordState {
			case StateLatest:
				if e.Record == "" || recordHash(e) != strings.ToLower(e.RecordHash) {
					chain = chainBreak(e, ClassRecord, "record modified")
				}
			case StateDeleted:
				chain = chainBreak(e, ClassRecord, "record deleted")
			case StateSuperseded:
				if e.NextSeq <= e.Seq {
					chain = chainBreak(e, ClassChain, "entry says it was superseded by seq %d, which is not later", e.NextSeq)
				} else if e.NextSeq > m.LastSeq {
					res.RecordsNotChecked++
				} else if n, ok := entryAt(b.Entries, e.NextSeq); !ok || n.Kind != e.Kind || !strings.EqualFold(n.RecordID, e.RecordID) {
					chain = chainBreak(e, ClassChain, "entry says it was superseded by seq %d, which is not an entry of the same record", e.NextSeq)
				}
			default:
				chain = chainBreak(e, ClassChain, "unknown record_state %q", e.RecordState)
			}
		}
		if chain == nil {
			prev = eh
			expect++
			res.Entries++
		}
	}
	last := expect - 1 // newest entry that held
	if chain == nil && last < m.LastSeq {
		chain = &Break{Seq: last + 1, Reason: "entry missing", Class: ClassChain}
	}

	// Signed heads: valid, and the chain entry at that seq has the signed hash.
	var newest *head
	for i := range heads {
		h := heads[i]
		res.Heads++
		if h.problem != "" {
			setHead(h.Seq, h.KeyID, h.problem)
			continue
		}
		if h.Seq > m.AnchorSeq {
			e, ok := bySeq[h.Seq]
			switch {
			case h.Seq > m.LastSeq || (!ok && chain == nil):
				setHead(m.LastSeq+1, h.KeyID, fmt.Sprintf("chain ends at %d, before the signed head %d", m.LastSeq, h.Seq))
			case ok && strings.ToLower(e.EntryHash) != hex.EncodeToString(h.EntryHash):
				setHead(h.Seq, h.KeyID, "chain entry differs from the signed head (past rewritten)")
			}
		}
		if newest == nil || h.Seq >= newest.Seq {
			newest = &heads[i]
		}
	}
	if newest != nil {
		res.NewestHeadSeq = newest.Seq
	}
	res.FirstSeq, res.LastSeq = m.AnchorSeq+1, m.LastSeq

	switch {
	case chain != nil && (headBreak == nil || chain.Seq <= headBreak.Seq):
		res.Break = chain
		return res
	case headBreak != nil:
		res.Break = headBreak
		return res
	}

	if len(b.Entries) == 0 {
		res.Status = StatusValid
		return res
	}
	// The end of the bundle must be the chain's end, or a signed head.
	lastEntry := b.Entries[len(b.Entries)-1]
	switch {
	case m.ChainTipSeq == m.LastSeq && !strings.EqualFold(m.ChainTipHash, lastEntry.EntryHash):
		res.Break = chainBreak(lastEntry, ClassChain, "chain head does not match the last entry")
		return res
	case m.ChainTipSeq < m.LastSeq:
		return bundleBad("the manifest's chain head (seq %d) is behind the last entry (%d)", m.ChainTipSeq, m.LastSeq)
	case m.ChainTipSeq > m.LastSeq && (newest == nil || newest.Seq != m.LastSeq):
		return bundleBad("the bundle ends at seq %d, before the chain's end (%d), and not at a signed head", m.LastSeq, m.ChainTipSeq)
	}
	switch {
	case newest == nil:
		res.Status = StatusUnsigned
		res.Break = &Break{Seq: b.Entries[0].Seq, Kind: b.Entries[0].Kind, ID: b.Entries[0].RecordID, Class: ClassHead,
			Reason: fmt.Sprintf("the bundle has %d entries and no signed head", len(b.Entries))}
		return res
	case newest.Seq < m.LastSeq && o.MaxAge > 0 && now.Sub(newest.SignedAt) > o.MaxAge:
		next, _ := entryAt(b.Entries, newest.Seq+1)
		res.Status = StatusStale
		res.Break = &Break{Seq: newest.Seq + 1, Kind: next.Kind, ID: next.RecordID, Class: ClassHead,
			Reason: fmt.Sprintf("newest signed head (seq %d) is %s old, limit %s, and the chain is at %d",
				newest.Seq, now.Sub(newest.SignedAt).Round(time.Second), o.MaxAge, m.LastSeq)}
		return res
	}
	res.Status = StatusValid
	return res
}

func entryAt(entries []Entry, seq int64) (Entry, bool) {
	for _, e := range entries {
		if e.Seq == seq {
			return e, true
		}
	}
	return Entry{}, false
}
