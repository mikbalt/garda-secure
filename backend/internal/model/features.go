package model

import "time"

// AttendanceOverride is one admin correction of an attendance record with
// the values before and after it (F6). Rows are written in the same
// transaction as the override and never change.
type AttendanceOverride struct {
	ID               string     `json:"id"`
	AttendanceID     string     `json:"attendance_id"`
	ShiftID          *string    `json:"shift_id,omitempty"`
	SiteID           string     `json:"site_id"`
	BeforeStatus     *string    `json:"before_status,omitempty"` // nil: the shift had no record
	BeforeClockInAt  *time.Time `json:"before_clock_in_at,omitempty"`
	BeforeClockOutAt *time.Time `json:"before_clock_out_at,omitempty"`
	BeforeReason     *string    `json:"before_reason,omitempty"`
	AfterStatus      string     `json:"after_status"`
	AfterClockInAt   *time.Time `json:"after_clock_in_at,omitempty"`
	AfterClockOutAt  *time.Time `json:"after_clock_out_at,omitempty"`
	Reason           string     `json:"reason"`
	Actor            string     `json:"actor"`
	CreatedAt        time.Time  `json:"created_at"`
}

// Floorplan is an uploaded site floor plan image on which checkpoints are
// placed (F3, D-035: on-prem live map without map tiles).
type Floorplan struct {
	ID          string    `json:"id"`
	SiteID      string    `json:"site_id"`
	Name        string    `json:"name"`
	ObjectKey   string    `json:"object_key"`
	ContentType string    `json:"content_type"`
	Width       int       `json:"width"`  // pixels, as displayed
	Height      int       `json:"height"` // pixels, as displayed
	SizeBytes   int64     `json:"size_bytes"`
	CreatedBy   *string   `json:"created_by,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	URL         string    `json:"url,omitempty"` // pre-signed, filled by the handler
}
