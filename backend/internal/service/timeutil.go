package service

import (
	"time"
)

// Accepted skew for client-supplied event timestamps (offline sync, D-006).
const (
	MaxClientPast   = 7 * 24 * time.Hour
	MaxClientFuture = 2 * time.Minute
)

// Now is the clock used by the service layer; tests replace it.
var Now = func() time.Time { return time.Now().UTC() }

// DayRange returns [start, end) in UTC of a calendar date ("YYYY-MM-DD") in
// loc. An empty date means today in loc.
func DayRange(date string, loc *time.Location) (time.Time, time.Time, error) {
	var d time.Time
	if date == "" {
		n := Now().In(loc)
		d = time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, loc)
	} else {
		var err error
		d, err = time.ParseInLocation("2006-01-02", date, loc)
		if err != nil {
			return time.Time{}, time.Time{}, BadRequest("invalid_date", "date must be YYYY-MM-DD")
		}
	}
	return d.UTC(), d.AddDate(0, 0, 1).UTC(), nil
}

// DateRange returns [start of from, end of to) in UTC for two inclusive
// calendar dates in loc; empty values default to today. Ranges longer than
// maxDays are rejected.
func DateRange(from, to string, loc *time.Location, maxDays int) (time.Time, time.Time, error) {
	start, _, err := DayRange(from, loc)
	if err != nil {
		return start, start, err
	}
	_, end, err := DayRange(to, loc)
	if err != nil {
		return start, end, err
	}
	if !end.After(start) {
		return start, end, BadRequest("invalid_range", "'to' must not be before 'from'")
	}
	if end.Sub(start) > time.Duration(maxDays)*24*time.Hour+time.Hour { // +1h tolerates DST
		return start, end, BadRequest("range_too_long", "date range is limited to %d days", maxDays)
	}
	return start, end, nil
}

// MonthRange returns [start, end) in UTC of a month ("YYYY-MM") in loc. An
// empty month means the current month in loc.
func MonthRange(month string, loc *time.Location) (time.Time, time.Time, error) {
	var m time.Time
	if month == "" {
		n := Now().In(loc)
		m = time.Date(n.Year(), n.Month(), 1, 0, 0, 0, 0, loc)
	} else {
		var err error
		m, err = time.ParseInLocation("2006-01", month, loc)
		if err != nil {
			return time.Time{}, time.Time{}, BadRequest("invalid_month", "month must be YYYY-MM")
		}
	}
	return m.UTC(), m.AddDate(0, 1, 0).UTC(), nil
}

// ClientTime validates an optional client-supplied event time; nil means now.
func ClientTime(t *time.Time) (time.Time, error) {
	now := Now()
	if t == nil {
		return now, nil
	}
	ts := t.UTC()
	if ts.Before(now.Add(-MaxClientPast)) || ts.After(now.Add(MaxClientFuture)) {
		return now, BadRequest("timestamp_out_of_range", "event time %s is outside the accepted sync window", ts.Format(time.RFC3339))
	}
	return ts, nil
}

// minutesBetween returns whole minutes from a to b, or 0 when b is not after a.
func minutesBetween(a, b time.Time) int {
	if !b.After(a) {
		return 0
	}
	return int(b.Sub(a) / time.Minute)
}
