package service

import "time"

// todayStr returns today's date as YYYY-MM-DD in UTC.
func todayStr() string {
	return time.Now().UTC().Format("2006-01-02")
}

// isLate returns true if now is more than 10 minutes past shiftStart.
func isLate(shiftStart time.Time) bool {
	return time.Now().UTC().After(shiftStart.UTC().Add(10 * time.Minute))
}

// isEarlyOut returns true if now is more than 10 minutes before shiftEnd.
func isEarlyOut(shiftEnd time.Time) bool {
	return time.Now().UTC().Before(shiftEnd.UTC().Add(-10 * time.Minute))
}
