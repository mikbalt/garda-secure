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
	// Not broken, but not proven either: never to be shown as plain VALID.
	StatusNoEntries    = "no_entries"                  // the bundle holds no entry: it proves nothing about the range
	StatusUnsignedTail = "unsigned_tail"               // entries after the newest signed head
	StatusUnverified   = "unverified_records"          // records whose later change is not in the bundle
	StatusRangeNotAuth = "range_not_authenticated"     // everything holds, but the range is only the exporter's claim
	StatusUnverifiedOK = "unverified_records_accepted" // as StatusUnverified, accepted with a flag: still not plain valid
	StatusRange        = "range_mismatch"              // the bundle does not cover the range the caller expects
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
	// Things a valid result does not tell, and the freshness of the signature.
	Warnings           []string `json:"warnings"`
	NewestHeadSignedAt string   `json:"newest_head_signed_at,omitempty"`
	ExportedAt         string   `json:"exported_at,omitempty"`
	RangeAuthenticated bool     `json:"range_authenticated"`
	RangeNotChecked    bool     `json:"range_not_checked,omitempty"` // valid, but only because the caller opted out of checking the range
}

// Exit codes of garda-verify, one per failure class.
const (
	CodeValid        = 0
	CodeChain        = 1 // an entry or a record was changed, deleted or reordered
	CodeUsage        = 2 // not a verification result: bad arguments
	CodeBundle       = 3 // the bundle is unreadable or fails its integrity check
	CodeHead         = 4 // a signed head, a key or a signature does not hold (also: wrong fingerprint)
	CodeUnsigned     = 5
	CodeStale        = 6
	CodeUnsignedTail = 7
	CodeUnverified   = 8
	CodeNoEntries    = 9
	CodeRange        = 10
	CodeRangeNotAuth = 11
	CodeUnverifiedOK = 12
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
	case StatusUnsignedTail:
		return CodeUnsignedTail
	case StatusUnverified:
		return CodeUnverified
	case StatusNoEntries:
		return CodeNoEntries
	case StatusRange:
		return CodeRange
	case StatusRangeNotAuth:
		return CodeRangeNotAuth
	case StatusUnverifiedOK:
		return CodeUnverifiedOK
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
	// ExpectFrom and ExpectTo (YYYY-MM-DD, site time zone, to inclusive): the
	// range the caller asked for. The manifest's range is only the exporter's claim.
	ExpectFrom, ExpectTo string
	// AllowUnverified accepts records whose later change is outside the bundle
	// (still reported as a warning, never as a plain valid).
	AllowUnverified bool
	// NoRangeCheck accepts the range as the exporter states it (no expected
	// range given): the result is a valid with RangeNotChecked set.
	NoRangeCheck bool
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
	var anchorSigned time.Time
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
		prev, anchorSigned = anchor.EntryHash, anchor.SignedAt
	}

	var chain *Break
	var firstUnverified *Entry
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
					if firstUnverified == nil {
						e := e
						firstUnverified = &e
					}
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

	if newest != nil {
		res.NewestHeadSignedAt = newest.SignedAt.UTC().Format(time.RFC3339)
	}
	res.ExportedAt = m.ExportedAt
	notProven := func(status string, e *Entry, class, format string, a ...any) Result {
		res.Status = status
		res.Break = &Break{Class: class, Reason: fmt.Sprintf(format, a...)}
		if e != nil {
			res.Break.Seq, res.Break.Kind, res.Break.ID = e.Seq, e.Kind, e.RecordID
		}
		return res
	}

	// Nothing to prove: a bundle without entries says nothing about the
	// range, whoever built it (the manifest and the heads are not signed
	// for "this site had no entries").
	if len(b.Entries) == 0 {
		return notProven(StatusNoEntries, nil, ClassBundle, "the bundle holds no entry, so it proves nothing about this range (the signed heads cannot show that a site had no entries)")
	}

	// The range in the manifest must at least fit the entries.
	mFrom, err1 := time.Parse(time.RFC3339, m.From)
	mTo, err2 := time.Parse(time.RFC3339, m.To)
	if err1 != nil || err2 != nil || !mTo.After(mFrom) {
		return bundleBad("the range in the manifest (from, to) is not valid")
	}
	inRange := false
	for _, e := range b.Entries {
		if at, err := time.Parse(time.RFC3339Nano, e.CreatedAt); err == nil && !at.Before(mFrom) && at.Before(mTo) {
			inRange = true
			break
		}
	}
	if !inRange {
		return bundleBad("none of the entries was made in the range the manifest names (%s to %s): the range does not match the entries", m.From, m.To)
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
			Reason: fmt.Sprintf("the bundle has %d entries and no signed head%s", len(b.Entries), signingNote(m))}
		return res
	case newest.Seq < m.LastSeq && o.MaxAge > 0 && now.Sub(newest.SignedAt) > o.MaxAge:
		next, _ := entryAt(b.Entries, newest.Seq+1)
		res.Status = StatusStale
		res.Break = &Break{Seq: newest.Seq + 1, Kind: next.Kind, ID: next.RecordID, Class: ClassHead,
			Reason: fmt.Sprintf("newest signed head (seq %d) is %s old, limit %s, and the chain is at %d",
				newest.Seq, now.Sub(newest.SignedAt).Round(time.Second), o.MaxAge, m.LastSeq)}
		return res
	case newest.Seq < m.LastSeq:
		next, _ := entryAt(b.Entries, newest.Seq+1)
		return notProven(StatusUnsignedTail, &next, ClassHead,
			"UNSIGNED TAIL: %d entries after the newest signed head (seq %d, signed %s): no signature vouches for them yet, and a bundle cut short looks the same. Export again after the next signed head.%s",
			m.LastSeq-newest.Seq, newest.Seq, newest.SignedAt.UTC().Format(time.RFC3339), signingNote(m))
	}

	// The range the caller expects, against what the bundle can show.
	rangeProblem, authenticated, err := o.checkExpectedRange(m, anchorSigned, newest.SignedAt)
	if err != nil {
		return bundleBad("%v", err)
	}
	res.RangeAuthenticated = authenticated
	if rangeProblem != "" {
		return notProven(StatusRange, nil, ClassBundle, "%s", rangeProblem)
	}
	if !authenticated {
		res.Warnings = append(res.Warnings, fmt.Sprintf("the range %s to %s is the exporter's claim: the range is not authenticated. Give --expect-from and --expect-to to check it", m.From, m.To))
	}
	if res.RecordsNotChecked > 0 && !o.AllowUnverified {
		return notProven(StatusUnverified, firstUnverified, ClassRecord,
			"UNVERIFIED RECORDS: %d records are said to have changed again after this bundle, and the later entry is not in it, so their text cannot be checked. Export a range that includes the change, or accept this knowingly with --allow-unverified-records (the exit code stays non-zero)",
			res.RecordsNotChecked)
	}
	if at, err := time.Parse(time.RFC3339Nano, m.ExportedAt); err != nil {
		res.Warnings = append(res.Warnings, "the bundle has no valid exported_at time: its age is unknown")
	} else if o.MaxAge > 0 && now.Sub(at) > o.MaxAge {
		res.Warnings = append(res.Warnings, fmt.Sprintf("the bundle was exported %s ago (limit %s): anything recorded since is not in it. Export again to be sure it is current",
			now.Sub(at).Round(time.Minute), o.MaxAge))
	}

	// Never a silent exit 0: records that were not checked, or a range that
	// is only the exporter's claim, each have their own code. The only way
	// to exit 0 without --expect-* is the explicit --no-range-check.
	if res.RecordsNotChecked > 0 {
		res.Warnings = append(res.Warnings, fmt.Sprintf("%d of %d entries (%d%%) were NOT checked against their record text: the record changed again after the bundle; only their place in the chain was checked",
			res.RecordsNotChecked, len(b.Entries), res.RecordsNotChecked*100/len(b.Entries)))
		return notProven(StatusUnverifiedOK, firstUnverified, ClassRecord,
			"%d of %d entries (%d%%) were NOT checked against their record text (accepted with --allow-unverified-records)",
			res.RecordsNotChecked, len(b.Entries), res.RecordsNotChecked*100/len(b.Entries))
	}
	if !authenticated {
		if !o.NoRangeCheck {
			return notProven(StatusRangeNotAuth, nil, ClassBundle,
				"RANGE NOT AUTHENTICATED: everything in the bundle holds, but the range %s to %s is only what the exporter says: a bundle cut at an older signed head, started late, or an old genuine one looks the same. Give --expect-from and --expect-to, or accept this knowingly with --no-range-check", m.From, m.To)
		}
		res.RangeNotChecked = true
	}
	res.Status = StatusValid
	return res
}

// signingNote says why the export did not end on a signed head, when the
// exporting server asked the signer and it did not sign. It explains; it never
// changes a verdict.
func signingNote(m Manifest) string {
	switch m.Signing {
	case signer.OutcomeUnreachable, signer.OutcomeRefused, signer.OutcomeNotSetup:
		r := m.SigningReason
		if r == "" {
			r = m.Signing
		}
		return fmt.Sprintf(" (when exporting, the server asked for a fresh signed head: %s: %s)", m.Signing, r)
	case signer.OutcomeReused:
		return " (when exporting, a head signed in the last few seconds was reused: entries made since are not signed yet)"
	}
	return ""
}

// Online states of a check (Summarize): what a dashboard shows. Only
// StateBroken is a failure; the others say what is proven and what is not.
const (
	StateValidPinned     = "valid_pinned"       // holds, and the caller's pinned key vouches for it
	StateUnverifiedTrust = "unverified_trust"   // holds, but the key came from the same server: proves nothing to the caller
	StateUnsignedTail    = "unsigned_tail"      // holds so far; the newest entries are not signed yet
	StateUnverifiedRecs  = "unverified_records" // holds; some records could not be checked against their text
	StateNoEntries       = "no_entries"         // nothing in the range to check
	StateBroken          = "broken"             // something does not hold
)

// Summary is the state of an online check and how to show it.
type Summary struct {
	State string `json:"state"`
	// Tone: "ok", "notice" (not proven yet, nothing is wrong) or "broken".
	Tone string `json:"tone"`
	// Message is plain English for logs and API users; a UI translates State.
	Message string `json:"message"`
}

// Summarize maps a verification of the server's own export to the state a
// client shows. Nothing but StateBroken has the "broken" tone.
func Summarize(r Result, pinned bool) Summary {
	switch r.Status {
	case StatusValid:
		if pinned {
			return Summary{StateValidPinned, "ok", "The chain and its signed heads hold, and the signer key you pinned vouches for them."}
		}
		return Summary{StateUnverifiedTrust, "notice", "The chain and its signed heads hold, but the signer key came from this server: give the key fingerprint you were handed (pin) for a real answer."}
	case StatusUnsignedTail, StatusUnsigned, StatusStale:
		return Summary{StateUnsignedTail, "notice", "Nothing is wrong so far, but the newest entries are not covered by a signed head yet. Check again after the next signed head."}
	case StatusUnverified, StatusUnverifiedOK:
		return Summary{StateUnverifiedRecs, "notice", "The chain holds, but some records changed again after the range and could not be checked against their text."}
	case StatusNoEntries:
		return Summary{StateNoEntries, "notice", "There are no chain entries in this range, so there is nothing to check."}
	}
	return Summary{StateBroken, "broken", "Something does not hold: see first_break."}
}

// checkExpectedRange compares the range the caller expects (dates in the
// site's zone, to inclusive) with what the signatures show: the anchor head
// must have been signed before the range starts, and the newest signed head
// after it ends. problem is "" when it holds (or nothing was expected).
func (o Options) checkExpectedRange(m Manifest, anchorSigned, newestSigned time.Time) (problem string, authenticated bool, err error) {
	if o.ExpectFrom == "" && o.ExpectTo == "" {
		return "", false, nil
	}
	if o.ExpectFrom == "" || o.ExpectTo == "" {
		return "", false, errors.New("give both an expected start and an expected end of the range")
	}
	loc, lerr := time.LoadLocation(m.SiteTimezone)
	if lerr != nil {
		return "", false, fmt.Errorf("site time zone %q is unknown", m.SiteTimezone)
	}
	from, e1 := time.ParseInLocation("2006-01-02", o.ExpectFrom, loc)
	toDay, e2 := time.ParseInLocation("2006-01-02", o.ExpectTo, loc)
	if e1 != nil || e2 != nil {
		return "", false, errors.New("the expected range must be dates, YYYY-MM-DD")
	}
	to := toDay.AddDate(0, 0, 1)
	if !to.After(from) {
		return "", false, errors.New("the expected end is before the expected start")
	}
	mFrom, _ := time.Parse(time.RFC3339, m.From)
	mTo, _ := time.Parse(time.RFC3339, m.To)
	switch {
	case mFrom.After(from) || mTo.Before(to):
		return fmt.Sprintf("the bundle was made for %s to %s, which does not cover the range you expect (%s to %s, site time)", m.From, m.To, o.ExpectFrom, o.ExpectTo), false, nil
	case m.AnchorSeq > 0 && anchorSigned.After(from):
		return fmt.Sprintf("the bundle starts from a signed head (seq %d) signed at %s, after the range you expect begins (%s): earlier entries were skipped", m.AnchorSeq, anchorSigned.UTC().Format(time.RFC3339), o.ExpectFrom), false, nil
	case newestSigned.Before(to):
		return fmt.Sprintf("the newest signed head is from %s, before the end of the range you expect (%s): entries between are not vouched for. Export again once a later head exists", newestSigned.UTC().Format(time.RFC3339), o.ExpectTo), false, nil
	}
	return "", true, nil
}

func entryAt(entries []Entry, seq int64) (Entry, bool) {
	for _, e := range entries {
		if e.Seq == seq {
			return e, true
		}
	}
	return Entry{}, false
}
