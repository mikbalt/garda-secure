package service

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/securepatrol/backend/internal/model"
	"github.com/securepatrol/backend/internal/repository"
)

// GenerationHorizonDays is how far ahead concrete shifts are generated (D-003).
const GenerationHorizonDays = 14

// Occurrence is one concrete shift of a template.
type Occurrence struct {
	Start, End time.Time
}

// ValidateTemplate normalises and checks a template's fields.
func ValidateTemplate(t *model.ShiftTemplate) error {
	if _, err := parseClock(t.StartTime); err != nil {
		return BadRequest("invalid_start_time", "start_time must be HH:MM")
	}
	if _, err := parseClock(t.EndTime); err != nil {
		return BadRequest("invalid_end_time", "end_time must be HH:MM")
	}
	if t.StartTime == t.EndTime {
		return BadRequest("invalid_times", "start_time and end_time must differ")
	}
	if len(t.Weekdays) == 0 {
		return BadRequest("invalid_weekdays", "at least one weekday is required")
	}
	days := slices.Clone(t.Weekdays)
	slices.Sort(days)
	days = slices.Compact(days)
	for _, d := range days {
		if d < 1 || d > 7 {
			return BadRequest("invalid_weekdays", "weekdays must be 1 (Monday) … 7 (Sunday)")
		}
	}
	t.Weekdays = days
	from, err := time.Parse("2006-01-02", t.ValidFrom)
	if err != nil {
		return BadRequest("invalid_valid_from", "valid_from must be YYYY-MM-DD")
	}
	if t.ValidUntil != nil && *t.ValidUntil != "" {
		until, err := time.Parse("2006-01-02", *t.ValidUntil)
		if err != nil {
			return BadRequest("invalid_valid_until", "valid_until must be YYYY-MM-DD")
		}
		if until.Before(from) {
			return BadRequest("invalid_valid_until", "valid_until must not be before valid_from")
		}
	} else {
		t.ValidUntil = nil
	}
	return nil
}

// Occurrences expands a template into concrete shifts whose local start date
// lies in [fromDate, toDate] (inclusive, site-local calendar dates).
func Occurrences(t model.ShiftTemplate, loc *time.Location, fromDate, toDate time.Time) ([]Occurrence, error) {
	startClock, err := parseClock(t.StartTime)
	if err != nil {
		return nil, err
	}
	endClock, err := parseClock(t.EndTime)
	if err != nil {
		return nil, err
	}
	validFrom, err := time.ParseInLocation("2006-01-02", t.ValidFrom, loc)
	if err != nil {
		return nil, err
	}
	var validUntil *time.Time
	if t.ValidUntil != nil {
		u, err := time.ParseInLocation("2006-01-02", *t.ValidUntil, loc)
		if err != nil {
			return nil, err
		}
		validUntil = &u
	}

	var out []Occurrence
	day := time.Date(fromDate.Year(), fromDate.Month(), fromDate.Day(), 0, 0, 0, 0, loc)
	last := time.Date(toDate.Year(), toDate.Month(), toDate.Day(), 0, 0, 0, 0, loc)
	for ; !day.After(last); day = day.AddDate(0, 0, 1) {
		if day.Before(validFrom) || (validUntil != nil && day.After(*validUntil)) {
			continue
		}
		if !slices.Contains(t.Weekdays, isoWeekday(day)) {
			continue
		}
		start := time.Date(day.Year(), day.Month(), day.Day(), startClock.h, startClock.m, 0, 0, loc)
		endDay := day
		if endClock.minutes() <= startClock.minutes() {
			endDay = day.AddDate(0, 0, 1)
		}
		end := time.Date(endDay.Year(), endDay.Month(), endDay.Day(), endClock.h, endClock.m, 0, 0, loc)
		out = append(out, Occurrence{Start: start.UTC(), End: end.UTC()})
	}
	return out, nil
}

// GenerateTemplateShifts inserts the template's future shifts up to the
// horizon. Occurrences that already started are skipped so a new template
// never creates shifts that would be flagged absent immediately.
func GenerateTemplateShifts(ctx context.Context, db repository.DBTX, t model.ShiftTemplate, loc *time.Location) (int, error) {
	if !t.Active {
		return 0, nil
	}
	now := Now()
	today := now.In(loc)
	occ, err := Occurrences(t, loc, today, today.AddDate(0, 0, GenerationHorizonDays))
	if err != nil {
		return 0, fmt.Errorf("expand template %s: %w", t.ID, err)
	}
	added := 0
	for _, o := range occ {
		if !o.Start.After(now) {
			continue
		}
		ok, err := repository.InsertGeneratedShift(ctx, db, t.ID, t.GuardID, t.SiteID, o.Start, o.End)
		if err != nil {
			return added, err
		}
		if ok {
			added++
		}
	}
	return added, nil
}

// RegenerateTemplateShifts replaces a template's future shifts (those without
// attendance) after the template was edited.
func RegenerateTemplateShifts(ctx context.Context, db repository.DBTX, t model.ShiftTemplate, loc *time.Location) (int, error) {
	if _, err := repository.DeleteFutureTemplateShifts(ctx, db, t.ID, Now()); err != nil {
		return 0, err
	}
	return GenerateTemplateShifts(ctx, db, t, loc)
}

// GenerateAllShifts runs the generator for every active template.
func GenerateAllShifts(ctx context.Context, db repository.DBTX) (int, error) {
	templates, err := repository.ListActiveShiftTemplates(ctx, db)
	if err != nil {
		return 0, err
	}
	sites := map[string]*time.Location{}
	total := 0
	for _, t := range templates {
		loc, ok := sites[t.SiteID]
		if !ok {
			site, err := repository.GetSite(ctx, db, t.SiteID)
			if err != nil {
				return total, err
			}
			loc = site.Location()
			sites[t.SiteID] = loc
		}
		n, err := GenerateTemplateShifts(ctx, db, t, loc)
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}

type clock struct{ h, m int }

func (c clock) minutes() int { return c.h*60 + c.m }

func parseClock(s string) (clock, error) {
	t, err := time.Parse("15:04", s)
	if err != nil {
		return clock{}, err
	}
	return clock{t.Hour(), t.Minute()}, nil
}

// isoWeekday returns 1 (Monday) … 7 (Sunday).
func isoWeekday(t time.Time) int16 {
	return [...]int16{7, 1, 2, 3, 4, 5, 6}[t.Weekday()]
}
