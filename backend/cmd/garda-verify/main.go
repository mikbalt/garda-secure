// Command garda-verify checks a GARDA chain export bundle offline: no
// database, no network, no account (TAQ-18, D-073). It recomputes the record
// chain, checks the signed heads against the signer key fingerprint you pin,
// and reports "valid" or the first broken record.
//
//	garda-verify --pin <fingerprint> [--pin ...] [--max-age 26h] [--json] bundle.zip
//
// Exit codes: 0 valid; 1 an entry or record was changed or deleted; 2 bad
// arguments; 3 the bundle is unreadable or fails its integrity check; 4 a
// signed head, key or signature does not hold (also a wrong fingerprint);
// 5 UNSIGNED (entries but no signed head); 6 STALE (entries after the newest
// signed head, which is older than --max-age).
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
	asJSON := fs.Bool("json", false, "print the result as JSON")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "usage: garda-verify --pin <fingerprint> [--max-age 26h] [--json] bundle.zip")
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
	res := chainproof.Verify(b, chainproof.Options{Pins: pin, MaxAge: *maxAge, Now: now})

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
	fmt.Fprintf(w, "Range        %s to %s (UTC, end not included)\n", m.From, m.To)
	switch {
	case len(b.Entries) == 0:
		fmt.Fprintln(w, "Entries      none in the range")
	case m.AnchorSeq == 0:
		fmt.Fprintf(w, "Entries      seq %d to %d, %d checked, from the start of the chain\n", m.FirstSeq, m.LastSeq, res.Entries)
	default:
		fmt.Fprintf(w, "Entries      seq %d to %d, %d checked, starting from the signed head at seq %d\n", m.FirstSeq, m.LastSeq, res.Entries, m.AnchorSeq)
	}
	fmt.Fprintf(w, "Signed heads %d checked, newest at seq %d\n", res.Heads, res.NewestHeadSeq)
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
		fmt.Fprintln(w, "RESULT: VALID")
		fmt.Fprintln(w, "Every entry links to the one before, every record we hold matches its entry, and every signed head is genuine and matches the chain.")
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
