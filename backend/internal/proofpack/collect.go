// Package proofpack builds the signed monthly proof pack of a site (TAQ-20,
// D-074): a PDF with the month's patrol, attendance, incident and flag
// summaries, the chain head, the newest signed head and the signer's key
// fingerprint, and on its last page the steps to check it with garda-verify.
//
// It only reads. The chain and the signed heads come from chainproof.Export
// (the bundle the export endpoint serves) and are judged by chainproof.Verify,
// the same code garda-verify runs: the pack never signs, hashes or decides
// anything about the chain itself.
package proofpack

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/securepatrol/backend/internal/chainproof"
	"github.com/securepatrol/backend/internal/model"
	"github.com/securepatrol/backend/internal/repository"
	"github.com/securepatrol/backend/internal/service"
)

// Status is what the pack may say about the chain of its month.
type Status string

const (
	// StatusSigned: the server's own check found no break and a signed head
	// made after the month ended covers the last entry. The reader still checks it.
	StatusSigned Status = "signed"
	// StatusUnsignedTail: no signature made after the month ended covers the
	// last records (or there is none at all). Never shown as verified.
	StatusUnsignedTail Status = "unsigned_tail"
	// StatusFailed: the chain, a record or a signed head does not hold.
	StatusFailed Status = "failed"
	// StatusNotProven: anything else the verifier does not call valid.
	StatusNotProven Status = "not_proven"
	// StatusNoActivity: the site has no chain entry in the month.
	StatusNoActivity Status = "no_activity"
)

// maxFlags bounds the flag rows read for the summary.
const maxFlags = 20_000

// Day is one day of the month, in the site's time zone.
type Day struct {
	Date                           string
	Present, Late, Absent, Pending int // shifts by outcome
	PatrolSessions, Incidents      int
}

// GuardRow is one guard's month. No employee id, no contact data: what a
// client sees in the monthly recap.
type GuardRow struct {
	Name                                 string
	ScheduledDays, DaysPresent, DaysLate int
	DaysAbsent                           int
	Hours                                float64
	PatrolSessions                       int
	PatrolPct                            float64
	Incidents                            int
}

// FlagCount is a plausibility flag (TAQ-21) and how many were reviewed.
type FlagCount struct {
	Flag            string
	Count, Reviewed int
}

// Head is a signed head as the signer stored it.
type Head struct {
	Seq                                   int64
	EntryHash, SignedAt, KeyID, Signature string
}

// Chain is the proof section of the pack.
type Chain struct {
	Status            Status
	VerifyStatus      string // chainproof status of the server's own check
	Detail            string // plain-words reason when not signed
	RecordsNotChecked int    // records changed again after the month: not checked against their text
	Entries           int    // chain entries made in the month
	TipSeq            int64
	TipHash           string // the chain's head when the pack was made
	Head              *Head  // newest signed head in the checked range
	Fingerprint       string // key id (sha256 of the public key) of that head
	BundleName        string // file name the export gets
	ExpectFrom        string // first and last day of the month, site time
	ExpectTo          string
}

// Data is everything the pack says.
type Data struct {
	SiteID, SiteName, Timezone, Month string
	FirstDay, LastDay                 string
	Generated                         time.Time
	MonthOpen                         bool // the month is not over yet
	Days                              []Day
	Guards                            []GuardRow
	Totals                            repository.MonthlyRecapTotals

	PatrolSessions      int
	PatrolAvgPct        float64
	SLABreaches         int
	FlaggedScans        int
	IncidentsTotal      int
	IncidentsBySeverity map[string]int
	Flags               []FlagCount
	FlagsTruncated      bool
	Chain               Chain
}

// BundleName is the file name of the chain export of [from, to] (to inclusive).
func BundleName(siteID, from, to string) string {
	return fmt.Sprintf("garda-chain-%s-%s_%s.zip", strings.SplitN(siteID, "-", 2)[0], from, to)
}

// Collect reads the month of a site. b is the chain bundle for the month
// (chainproof.Export of [first day, first day of next month)), or nil when it
// could not be read: the pack then says the chain was not checked.
func Collect(ctx context.Context, db repository.DBTX, site *model.Site, month string, b *chainproof.Bundle, now time.Time) (*Data, error) {
	loc := site.Location()
	from, to, err := service.MonthRange(month, loc)
	if err != nil {
		return nil, err
	}
	first, last := from.In(loc).Format("2006-01-02"), to.In(loc).AddDate(0, 0, -1).Format("2006-01-02")
	d := &Data{SiteID: site.ID, SiteName: site.Name, Timezone: site.Timezone, Month: month, FirstDay: first, LastDay: last,
		Generated: now.In(loc), MonthOpen: now.Before(to), IncidentsBySeverity: map[string]int{}}

	rows, err := repository.MonthlyRecap(ctx, db, site.ID, loc, from, to)
	if err != nil {
		return nil, err
	}
	d.Totals = repository.RecapTotals(rows)
	shifts, err := repository.RecapShifts(ctx, db, site.ID, from, to)
	if err != nil {
		return nil, err
	}
	sessions, err := repository.SessionSummaries(ctx, db, site.ID, from, to)
	if err != nil {
		return nil, err
	}
	incidents, err := repository.IncidentCounts(ctx, db, site.ID, site.Timezone, from, to)
	if err != nil {
		return nil, err
	}

	days := map[string]*Day{}
	for t := from.In(loc); t.Before(to) && !t.After(now); t = t.AddDate(0, 0, 1) {
		k := t.Format("2006-01-02")
		days[k] = &Day{Date: k}
	}
	day := func(k string) *Day {
		if days[k] == nil {
			days[k] = &Day{Date: k}
		}
		return days[k]
	}
	for _, s := range shifts {
		x := day(s.Start.In(loc).Format("2006-01-02"))
		switch s.Status {
		case "present":
			x.Present++
		case "late":
			x.Late++
		case "absent":
			x.Absent++
		default:
			x.Pending++
		}
	}
	var pct float64
	for _, s := range sessions {
		day(s.StartedAt.In(loc).Format("2006-01-02")).PatrolSessions++
		d.PatrolSessions++
		pct += s.CompletionPct
		d.SLABreaches += s.SLABreaches
		d.FlaggedScans += s.FlaggedScans
	}
	if d.PatrolSessions > 0 {
		d.PatrolAvgPct = round1(pct / float64(d.PatrolSessions))
	}
	perGuard := map[string]int{}
	for _, c := range incidents {
		day(c.Day).Incidents += c.N
		d.IncidentsTotal += c.N
		d.IncidentsBySeverity[c.Severity] += c.N
		perGuard[c.GuardID] += c.N
	}
	for _, x := range days {
		d.Days = append(d.Days, *x)
	}
	sort.Slice(d.Days, func(i, j int) bool { return d.Days[i].Date < d.Days[j].Date })

	seen := map[string]bool{}
	for _, r := range rows {
		seen[r.GuardID] = true
		d.Guards = append(d.Guards, GuardRow{Name: r.GuardName, ScheduledDays: r.ScheduledDays, DaysPresent: r.DaysPresent,
			DaysLate: r.DaysLate, DaysAbsent: r.DaysAbsent, Hours: r.TotalHours, PatrolSessions: r.PatrolSessions,
			PatrolPct: r.PatrolCompletePct, Incidents: perGuard[r.GuardID]})
	}

	// A guard whose only record is an incident is not in the recap: add the row so the table sums to the total.
	var extra []string
	for g := range perGuard {
		if !seen[g] {
			extra = append(extra, g)
		}
	}
	if len(extra) > 0 {
		names, err := repository.GuardNames(ctx, db, extra)
		if err != nil {
			return nil, err
		}
		for _, g := range extra {
			d.Guards = append(d.Guards, GuardRow{Name: names[g], Incidents: perGuard[g]})
		}
		sort.SliceStable(d.Guards, func(i, j int) bool { return d.Guards[i].Name < d.Guards[j].Name })
	}

	flags, err := repository.ListFlags(ctx, db, repository.FlagFilter{SiteIDs: []string{site.ID}, SiteID: &site.ID, From: &from, To: &to, Limit: maxFlags + 1})
	if err != nil {
		return nil, err
	}
	if len(flags) > maxFlags {
		flags, d.FlagsTruncated = flags[:maxFlags], true
	}
	byFlag := map[string]*FlagCount{}
	for _, f := range flags {
		c := byFlag[f.Flag]
		if c == nil {
			c = &FlagCount{Flag: f.Flag}
			byFlag[f.Flag] = c
		}
		c.Count++
		if f.ReviewedAt != nil {
			c.Reviewed++
		}
	}
	for _, c := range byFlag {
		d.Flags = append(d.Flags, *c)
	}
	sort.Slice(d.Flags, func(i, j int) bool {
		if d.Flags[i].Count != d.Flags[j].Count {
			return d.Flags[i].Count > d.Flags[j].Count
		}
		return d.Flags[i].Flag < d.Flags[j].Flag
	})

	d.Chain = chainSection(b, site, from, to, first, last, now)
	return d, nil
}

func round1(v float64) float64 { return float64(int(v*10+0.5)) / 10 }

// chainSection judges the bundle with the same verifier garda-verify runs,
// for the range the reader is told to expect (the whole month, site time).
func chainSection(b *chainproof.Bundle, site *model.Site, from, to time.Time, first, last string, now time.Time) Chain {
	c := Chain{ExpectFrom: first, ExpectTo: last, BundleName: BundleName(site.ID, first, last)}
	if b == nil {
		c.Status, c.VerifyStatus, c.Detail = StatusNotProven, "not_checked", "the chain was not checked when this pack was made (too large to read here, or unreadable): check the export offline with garda-verify"
		return c
	}
	c.TipSeq, c.TipHash = b.Manifest.ChainTipSeq, b.Manifest.ChainTipHash
	for _, e := range b.Entries {
		if at, err := time.Parse(time.RFC3339Nano, e.CreatedAt); err == nil && !at.Before(from) && at.Before(to) {
			c.Entries++
		}
	}
	res := chainproof.Verify(b, chainproof.Options{MaxAge: 26 * time.Hour, Now: now, ExpectFrom: first, ExpectTo: last})
	c.VerifyStatus = res.Status
	c.RecordsNotChecked = res.RecordsNotChecked
	if res.Break != nil {
		c.Detail = res.Break.Reason
	}
	var newest *chainproof.Head
	for i := range b.Heads {
		if b.Heads[i].Seq == res.NewestHeadSeq && res.NewestHeadSeq > 0 {
			newest = &b.Heads[i]
		}
	}
	if newest != nil {
		c.Head = &Head{Seq: newest.Seq, EntryHash: newest.EntryHash, SignedAt: newest.SignedAt, KeyID: newest.KeyID, Signature: newest.Signature}
		c.Fingerprint = newest.KeyID
	} else if len(b.Keyring) > 0 {
		c.Fingerprint = b.Keyring[0].KeyID
	}

	switch res.Status {
	case chainproof.StatusValid:
		c.Status = StatusSigned
	case chainproof.StatusNoEntries:
		c.Status = StatusNoActivity
	case chainproof.StatusUnsigned, chainproof.StatusUnsignedTail, chainproof.StatusStale:
		c.Status = StatusUnsignedTail
	case chainproof.StatusRange:
		// Everything holds, but no head was signed after the month ended.
		c.Status = StatusNotProven
		if at, err := time.Parse(time.RFC3339Nano, newestSignedAt(c)); err == nil && at.Before(to) {
			c.Status = StatusUnsignedTail
		}
	case chainproof.StatusBroken, chainproof.StatusBundle:
		c.Status = StatusFailed
	default:
		c.Status = StatusNotProven
	}
	return c
}

func newestSignedAt(c Chain) string {
	if c.Head == nil {
		return ""
	}
	return c.Head.SignedAt
}
