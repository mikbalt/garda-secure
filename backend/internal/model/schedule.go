package model

import "time"

// RouteSchedule is one window in which a route must be patrolled (D-035).
// EveryMinutes nil = one patrol at StartTime; otherwise patrols are due every
// EveryMinutes from StartTime until (excluding) EndTime. EndTime <= StartTime
// crosses midnight (equal = 24 h). Weekdays refer to the day the window
// starts, in the site's time zone.
type RouteSchedule struct {
	ID           string    `json:"id"`
	RouteID      string    `json:"route_id"`
	Weekdays     []int16   `json:"weekdays"`           // ISO 1 = Monday … 7 = Sunday
	StartTime    string    `json:"start_time"`         // HH:MM, site local time
	EndTime      *string   `json:"end_time,omitempty"` // HH:MM; required with every_minutes
	EveryMinutes *int      `json:"every_minutes"`      // nil = once at start_time
	GraceMinutes int       `json:"grace_minutes"`      // a patrol may start this early/late
	Active       bool      `json:"active"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// PatrolExpectation is one scheduled patrol and whether it was started.
type PatrolExpectation struct {
	ID         string     `json:"id"`
	RouteID    string     `json:"route_id"`
	RouteName  string     `json:"route_name,omitempty"`
	SiteID     string     `json:"site_id"`
	ScheduleID *string    `json:"schedule_id,omitempty"`
	DueAt      time.Time  `json:"due_at"`
	GraceUntil time.Time  `json:"grace_until"`
	Status     string     `json:"status"` // pending | started | missed | cancelled
	SessionID  *string    `json:"session_id,omitempty"`
	LateStart  bool       `json:"late_start"` // started after grace_until
	NotifiedAt *time.Time `json:"notified_at,omitempty"`
	MatchedAt  *time.Time `json:"matched_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

// Grace is the expectation's grace period (window [due - grace, due + grace]).
func (e *PatrolExpectation) Grace() time.Duration { return e.GraceUntil.Sub(e.DueAt) }
