// Command garda-verify checks a GARDA chain export bundle offline: no
// database, no network, no account (TAQ-18, D-073). It recomputes the record
// chain, checks the signed heads against the signer key fingerprint you pin,
// and reports "valid" or the first broken record.
//
//	garda-verify --pin <fingerprint> --expect-from 2026-11-01 --expect-to 2026-11-30 [--max-age 26h] [--json] bundle.zip
//
// Exit codes: 0 valid; 1 an entry or record was changed or deleted; 2 bad
// arguments; 3 the bundle is unreadable or fails its integrity check; 4 a
// signed head, key or signature does not hold (also a wrong fingerprint);
// 5 UNSIGNED (entries but no signed head); 6 STALE (entries after the newest
// signed head, which is older than --max-age); 7 UNSIGNED TAIL (entries after
// the newest signed head); 8 UNVERIFIED RECORDS (records changed again outside
// the bundle); 9 NO ENTRIES (nothing to check); 10 RANGE (the bundle does not
// cover the range you expect); 11 RANGE NOT AUTHENTICATED (everything holds, but
// you gave no --expect-from/--expect-to, so the range is only the exporter's
// claim: --no-range-check accepts that knowingly and exits 0 as "VALID (range
// not checked)"); 12 records NOT checked (--allow-unverified-records accepted
// them: still not plain valid).
//
// VALID does not prove that nothing is missing, see docs/chain-verification.md.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/securepatrol/backend/internal/chainproof"
)

var pinRE = regexp.MustCompile(`^[0-9a-f]{64}$`)

type pins []string

func (p *pins) String() string { return strings.Join(*p, ",") }
func (p *pins) Set(v string) error {
	v = strings.ToLower(strings.TrimSpace(v))
	if !pinRE.MatchString(v) {
		return errors.New("a fingerprint is 64 hex characters (sha256 of the public key)")
	}
	*p = append(*p, v)
	return nil
}

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, time.Now())) }

func run(args []string, stdout, stderr io.Writer, now time.Time) int {
	fs := flag.NewFlagSet("garda-verify", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var pin pins
	fs.Var(&pin, "pin", "signer key `fingerprint` you hold out of band (repeatable)")
	tofu := fs.Bool("trust-first-use", false, "no pinned fingerprint: trust the first key in the bundle (weaker: the bundle vouches for itself)")
	maxAge := fs.Duration("max-age", 26*time.Hour, "report STALE when entries follow the newest signed head and it is older than this (0 = off)")
	expFrom := fs.String("expect-from", "", "the first day you expect the bundle to cover (YYYY-MM-DD, site time). Without it the range is only the exporter's claim")
	expTo := fs.String("expect-to", "", "the last day you expect the bundle to cover (YYYY-MM-DD, site time, included)")
	allowUnv := fs.Bool("allow-unverified-records", false, "accept records that changed again outside the bundle: they stay unchecked and the exit code stays non-zero (12)")
	noRange := fs.Bool("no-range-check", false, "no --expect-from/--expect-to: accept the range as the exporter states it (exit 0, printed as \"VALID (range not checked)\")")
	asJSON := fs.Bool("json", false, "print the result as JSON")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "usage: garda-verify --pin <fingerprint> --expect-from YYYY-MM-DD --expect-to YYYY-MM-DD [--max-age 26h] [--json] bundle.zip\n       (without --expect-*: --no-range-check, exit 11 otherwise)")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return chainproof.CodeUsage
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return chainproof.CodeUsage
	}
	if len(pin) == 0 && !*tofu {
		fmt.Fprintln(stderr, "garda-verify: give the signer key fingerprint with --pin (you got it when the system was installed), or --trust-first-use to accept the bundle's own first key")
		return chainproof.CodeUsage
	}
	if *maxAge < 0 {
		fmt.Fprintln(stderr, "garda-verify: --max-age must not be negative")
		return chainproof.CodeUsage
	}

	raw, err := os.ReadFile(fs.Arg(0))
	if err != nil {
		fmt.Fprintln(stderr, "garda-verify:", err)
		return chainproof.CodeBundle
	}
	b, err := chainproof.Read(raw)
	if err != nil {
		fmt.Fprintln(stderr, "garda-verify:", err)
		return chainproof.CodeBundle
	}
	if *noRange && *expFrom != "" {
		fmt.Fprintln(stderr, "garda-verify: --no-range-check and --expect-from/--expect-to exclude each other")
		return chainproof.CodeUsage
	}
	if (*expFrom == "") != (*expTo == "") {
		fmt.Fprintln(stderr, "garda-verify: give --expect-from and --expect-to together")
		return chainproof.CodeUsage
	}
	for _, d := range []string{*expFrom, *expTo} {
		if _, err := time.Parse("2006-01-02", d); d != "" && err != nil {
			fmt.Fprintln(stderr, "garda-verify: --expect-from and --expect-to are dates, YYYY-MM-DD")
			return chainproof.CodeUsage
		}
	}
	res := chainproof.Verify(b, chainproof.Options{Pins: pin, MaxAge: *maxAge, Now: now, ExpectFrom: *expFrom, ExpectTo: *expTo, AllowUnverified: *allowUnv, NoRangeCheck: *noRange})

	if *asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(map[string]any{"result": res, "exit_code": res.Code(), "bundle_sha256": b.ManifestSHA256, "trust": trustMode(pin)})
		return res.Code()
	}
	report(stdout, b, res, pin)
	return res.Code()
}

func trustMode(p pins) string {
	if len(p) == 0 {
		return "first_use"
	}
	return "pinned"
}

func report(w io.Writer, b *chainproof.Bundle, res chainproof.Result, p pins) {
	m := b.Manifest
	fmt.Fprintf(w, "Site         %s (%s)\n", m.SiteID, m.SiteTimezone)
	if res.RangeAuthenticated {
		fmt.Fprintf(w, "Range        %s to %s (UTC, end not included), checked against the range you expect and the signing times\n", m.From, m.To)
	} else {
		fmt.Fprintf(w, "Range        %s to %s (UTC, end not included): NOT AUTHENTICATED, this is only what the exporter says\n", m.From, m.To)
	}
	switch {
	case len(b.Entries) == 0:
		fmt.Fprintln(w, "Entries      none in the range")
	case m.AnchorSeq == 0:
		fmt.Fprintf(w, "Entries      seq %d to %d, %d checked, from the start of the chain\n", m.FirstSeq, m.LastSeq, res.Entries)
	default:
		fmt.Fprintf(w, "Entries      seq %d to %d, %d checked, starting from the signed head at seq %d\n", m.FirstSeq, m.LastSeq, res.Entries, m.AnchorSeq)
	}
	fmt.Fprintf(w, "Signed heads %d checked, newest at seq %d, signed %s\n", res.Heads, res.NewestHeadSeq, orNone(res.NewestHeadSignedAt))
	fmt.Fprintf(w, "Exported     %s (by the exporter's clock)\n", orNone(res.ExportedAt))
	if res.RecordsNotChecked > 0 {
		fmt.Fprintf(w, "Note         %d records changed again after this range: their old states are not kept, so they are checked as links of the chain only\n", res.RecordsNotChecked)
	}
	if len(p) == 0 {
		fmt.Fprintln(w, "Trust        FIRST USE: no fingerprint was pinned, the bundle's first key is trusted without proof")
	} else {
		fmt.Fprintf(w, "Trust        pinned %s\n", strings.Join(p, ", "))
	}
	fmt.Fprintf(w, "Bundle       sha256 %s\n\n", b.ManifestSHA256)
	switch res.Status {
	case chainproof.StatusValid:
		if res.RangeNotChecked {
			fmt.Fprintln(w, "RESULT: VALID (range not checked)")
		} else {
			fmt.Fprintln(w, "RESULT: VALID")
		}
		fmt.Fprintln(w, "The entries in this bundle link to each other, the records it carries match their entries, and its signed heads are genuine and match the chain.")
		fmt.Fprintln(w, "This does NOT prove nothing is missing, that the bundle is current, or what the host or the signer could have done: see docs/chain-verification.md, \"What VALID does not prove\".")
		for _, wn := range res.Warnings {
			fmt.Fprintf(w, "WARNING: %s\n", wn)
		}
	case chainproof.StatusUnsignedTail, chainproof.StatusUnverified, chainproof.StatusUnverifiedOK, chainproof.StatusNoEntries, chainproof.StatusRange, chainproof.StatusRangeNotAuth:
		fmt.Fprintf(w, "RESULT: %s (NOT VALID, NOT PROVEN)\n", strings.ToUpper(strings.ReplaceAll(res.Status, "_", " ")))
		if br := res.Break; br != nil {
			if br.Seq > 0 {
				fmt.Fprintf(w, "At seq %d, %s %s\n", br.Seq, br.Kind, br.ID)
			}
			fmt.Fprintf(w, "Reason: %s\n", br.Reason)
		}
	default:
		fmt.Fprintf(w, "RESULT: %s\n", strings.ToUpper(res.Status))
		if br := res.Break; br != nil {
			if br.Seq > 0 {
				fmt.Fprintf(w, "First problem at seq %d", br.Seq)
				if br.Kind != "" {
					fmt.Fprintf(w, ", %s %s", br.Kind, br.ID)
				}
				fmt.Fprintln(w)
			}
			fmt.Fprintf(w, "Reason: %s\n", br.Reason)
		}
	}
}

func orNone(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
}
