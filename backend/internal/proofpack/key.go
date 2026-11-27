package proofpack

import "fmt"

// Key is where the scheduled job keeps a site's pack of a month
// (D-074): "<month>.pdf" when a signature made after the month covers it,
// "<month>.unsigned.pdf" while it carries the UNSIGNED TAIL notice.
func Key(siteID, month string, signed bool) string {
	if signed {
		return fmt.Sprintf("proof-packs/%s/%s.pdf", siteID, month)
	}
	return fmt.Sprintf("proof-packs/%s/%s.unsigned.pdf", siteID, month)
}
