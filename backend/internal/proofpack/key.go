package proofpack

import "fmt"

// KeyFinal is where the scheduled job keeps the pack of a month once a
// signature made after the month covers it (written once, never replaced).
func KeyFinal(siteID, month string) string {
	return fmt.Sprintf("proof-packs/%s/%s.pdf", siteID, month)
}

// KeyOpen is the copy kept until then, whatever its status (unsigned_tail,
// failed, not_proven, no_activity). The status is stored with the object and
// served with it. The job replaces it with the latest pack in the first ten
// minutes of each hour until the final copy exists, so it is never final.
func KeyOpen(siteID, month string) string {
	return fmt.Sprintf("proof-packs/%s/%s.open.pdf", siteID, month)
}
