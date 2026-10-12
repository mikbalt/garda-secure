package model

import (
	"encoding/json"
	"time"
)

// Guard represents a security guard (or a supervisor who works from the tablet).
type Guard struct {
	ID             string    `json:"id"`
	KeycloakUserID *string   `json:"keycloak_user_id,omitempty"`
	Name           string    `json:"name"`
	EmployeeID     *string   `json:"employee_id,omitempty"`
	SiteID         *string   `json:"site_id,omitempty"`
	Role           string    `json:"role"` // guard | supervisor
	PinHash        *string   `json:"-"`    // only exposed to devices via RosterGuard
	HasPin         bool      `json:"has_pin"`
	Status         string    `json:"status"` // active | inactive
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	// PinLockedUntil is set while server PIN checks are locked after too
	// many wrong PINs (guard sessions, D-035).
	PinLockedUntil    *time.Time `json:"pin_locked_until,omitempty"`
	PinFailedAttempts int        `json:"-"`
}

// RosterGuard is the guard view a tablet caches for offline PIN unlock.
type RosterGuard struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	EmployeeID *string `json:"employee_id,omitempty"`
	Role       string  `json:"role"`
	PinHash    *string `json:"pin_hash"`
}

// Site represents a client location managed by the system.
type Site struct {
	ID            string          `json:"id"`
	Name          string          `json:"name"`
	ClientID      *string         `json:"client_id,omitempty"`
	KeycloakRealm *string         `json:"keycloak_realm,omitempty"`
	Address       *string         `json:"address,omitempty"`
	Timezone      string          `json:"timezone"`
	ConfigJSON    json.RawMessage `json:"config_json,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
}

// SiteConfig holds the per-site operational settings stored in sites.config_json.
type SiteConfig struct {
	ClockWindowMinutes   int    `json:"clock_window_minutes"`    // clock in/out allowed ± this around shift start/end
	LateGraceMinutes     int    `json:"late_grace_minutes"`      // clock-in after start + grace is late
	EarlyOutGraceMinutes int    `json:"early_out_grace_minutes"` // clock-out before end - grace is early-out
	AbsentAfterMinutes   int    `json:"absent_after_minutes"`    // no clock-in by start + this → absent
	IdleLockMinutes      int    `json:"idle_lock_minutes"`       // tablet auto-lock
	Language             string `json:"language"`                // notification language: id | en
	ScanRadiusMeters     int    `json:"scan_radius_m"`           // scans with GPS farther from the checkpoint are flagged gps_far; 0 = off
}

// DefaultSiteConfig returns the configuration applied when a site has no overrides.
func DefaultSiteConfig() SiteConfig {
	return SiteConfig{
		ClockWindowMinutes:   15,
		LateGraceMinutes:     5,
		EarlyOutGraceMinutes: 5,
		AbsentAfterMinutes:   30,
		IdleLockMinutes:      5,
		Language:             "id",
		ScanRadiusMeters:     150,
	}
}

// Config parses the site's config_json on top of the defaults.
func (s *Site) Config() SiteConfig {
	cfg := DefaultSiteConfig()
	if len(s.ConfigJSON) > 0 {
		_ = json.Unmarshal(s.ConfigJSON, &cfg)
	}
	return cfg
}

// Location returns the site's time zone, falling back to UTC when invalid.
func (s *Site) Location() *time.Location {
	loc, err := time.LoadLocation(s.Timezone)
	if err != nil {
		return time.UTC
	}
	return loc
}

// Device is a shared tablet bound to one site.
type Device struct {
	ID             string     `json:"id"`
	KeycloakRealm  string     `json:"keycloak_realm"`
	KeycloakUserID string     `json:"keycloak_user_id"`
	Name           string     `json:"name"`
	SiteID         *string    `json:"site_id,omitempty"`
	Status         string     `json:"status"` // pending | active | revoked
	ApprovedBy     *string    `json:"approved_by,omitempty"`
	ApprovedAt     *time.Time `json:"approved_at,omitempty"`
	LastSeenAt     *time.Time `json:"last_seen_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// ShiftTemplate is a recurring schedule from which concrete shifts are generated.
type ShiftTemplate struct {
	ID         string    `json:"id"`
	SiteID     string    `json:"site_id"`
	GuardID    string    `json:"guard_id"`
	StartTime  string    `json:"start_time"` // HH:MM, site local time
	EndTime    string    `json:"end_time"`   // HH:MM; <= start_time means next day
	Weekdays   []int16   `json:"weekdays"`   // ISO 1 = Monday … 7 = Sunday
	ValidFrom  string    `json:"valid_from"` // YYYY-MM-DD
	ValidUntil *string   `json:"valid_until,omitempty"`
	Active     bool      `json:"active"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// Shift represents a concrete scheduled work period for a guard at a site.
type Shift struct {
	ID         string    `json:"id"`
	GuardID    string    `json:"guard_id"`
	SiteID     string    `json:"site_id"`
	TemplateID *string   `json:"template_id,omitempty"`
	StartTime  time.Time `json:"start_time"`
	EndTime    time.Time `json:"end_time"`
	CreatedAt  time.Time `json:"created_at"`
}

// AttendanceRecord stores a guard's clock-in/out for a shift.
type AttendanceRecord struct {
	ID              string     `json:"id"`
	ShiftID         *string    `json:"shift_id,omitempty"`
	GuardID         string     `json:"guard_id"`
	GuardName       string     `json:"guard_name,omitempty"`
	SiteID          string     `json:"site_id"`
	ClockInAt       *time.Time `json:"clock_in_at,omitempty"`
	ClockOutAt      *time.Time `json:"clock_out_at,omitempty"`
	Status          string     `json:"status"` // present | late | absent
	LateMinutes     int        `json:"late_minutes"`
	EarlyOutMinutes int        `json:"early_out_minutes"`
	SelfieObjectKey *string    `json:"selfie_object_key,omitempty"`
	GPSLat          *float64   `json:"gps_lat,omitempty"`
	GPSLng          *float64   `json:"gps_lng,omitempty"`
	ClockOutGPSLat  *float64   `json:"clock_out_gps_lat,omitempty"`
	ClockOutGPSLng  *float64   `json:"clock_out_gps_lng,omitempty"`
	DeviceID        *string    `json:"device_id,omitempty"`
	OverrideReason  *string    `json:"override_reason,omitempty"`
	OverriddenBy    *string    `json:"overridden_by,omitempty"`
	OverriddenAt    *time.Time `json:"overridden_at,omitempty"`
	NeedsReview     bool       `json:"needs_review"`
	ReviewNote      *string    `json:"review_note,omitempty"`
	ReviewedBy      *string    `json:"reviewed_by,omitempty"`
	ReviewedAt      *time.Time `json:"reviewed_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	// ReceivedAt is when the clock-in reached the server; Flags are
	// plausibility findings (service.Flag*).
	ReceivedAt *time.Time `json:"received_at,omitempty"`
	Flags      []string   `json:"flags"`
}

// Checkpoint is an individual stop on a patrol route (stored as JSONB inside Route).
type Checkpoint struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	QRCode string `json:"qr_code"`
	// QRSigned: qr_code is the signed payload SP1:<id>:<sig> of QRVersion
	// (server-generated); false = legacy plain code (D-035).
	QRSigned   bool     `json:"qr_signed"`
	QRVersion  int      `json:"qr_version"`
	SLAMinutes *int     `json:"sla_minutes,omitempty"` // nil = no SLA enforced
	OrderIndex int      `json:"order_index"`
	Lat        *float64 `json:"lat,omitempty"`
	Lng        *float64 `json:"lng,omitempty"`
	// Position on a site floor plan (F3): relative coordinates 0..1 from the
	// top-left corner of the displayed image.
	FloorplanID *string  `json:"floorplan_id,omitempty"`
	PlanX       *float64 `json:"plan_x,omitempty"`
	PlanY       *float64 `json:"plan_y,omitempty"`
}

// Route defines an ordered set of checkpoints for a patrol at a site.
type Route struct {
	ID          string       `json:"id"`
	SiteID      string       `json:"site_id"`
	Name        string       `json:"name"`
	Checkpoints []Checkpoint `json:"checkpoints"`
	Active      bool         `json:"active"`
	CreatedAt   time.Time    `json:"created_at"`
}

// PatrolSession is one walk of a route by a guard.
type PatrolSession struct {
	ID        string     `json:"id"`
	RouteID   string     `json:"route_id"`
	SiteID    string     `json:"site_id"`
	GuardID   string     `json:"guard_id"`
	DeviceID  *string    `json:"device_id,omitempty"`
	StartedAt time.Time  `json:"started_at"`
	EndedAt   *time.Time `json:"ended_at,omitempty"`
	Status    string     `json:"status"` // in_progress | completed | abandoned
	CreatedAt time.Time  `json:"created_at"`
	Flags     []string   `json:"flags"` // e.g. guard_unverified
}

// CheckpointScan records a guard scanning a checkpoint during a patrol.
type CheckpointScan struct {
	ID             string    `json:"id"`
	SessionID      *string   `json:"session_id,omitempty"`
	RouteID        string    `json:"route_id"`
	CheckpointID   string    `json:"checkpoint_id"`
	GuardID        string    `json:"guard_id"`
	DeviceID       *string   `json:"device_id,omitempty"`
	ScannedAt      time.Time `json:"scanned_at"`
	ReceivedAt     time.Time `json:"received_at"`
	Method         string    `json:"method"` // qr | manual
	PhotoObjectKey *string   `json:"photo_object_key,omitempty"`
	GPSLat         *float64  `json:"gps_lat,omitempty"`
	GPSLng         *float64  `json:"gps_lng,omitempty"`
	Flags          []string  `json:"flags"` // plausibility findings, e.g. gps_far, too_fast
}

// Incident records a security event filed by a guard. The original is
// immutable; corrections are stored as IncidentAmendment rows.
type Incident struct {
	ID              string    `json:"id"`
	SiteID          string    `json:"site_id"`
	GuardID         string    `json:"guard_id"`
	GuardName       string    `json:"guard_name,omitempty"`
	DeviceID        *string   `json:"device_id,omitempty"`
	Type            string    `json:"type"`     // category: theft, trespassing, medical, …
	Severity        string    `json:"severity"` // low | medium | high | critical
	Description     *string   `json:"description,omitempty"`
	PhotoObjectKeys []string  `json:"photo_object_keys"`
	Location        *string   `json:"location,omitempty"`
	Status          string    `json:"status"` // open | in_progress | resolved
	OccurredAt      time.Time `json:"occurred_at"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
	ReceivedAt      time.Time `json:"received_at"` // server receipt time
	Flags           []string  `json:"flags"`
	// Type, Severity, Description and Location are the effective values:
	// the latest amendment of each field, else the original (F7). Amended
	// is true once an amendment exists; Original* hold the filed values of
	// fields whose effective value differs.
	Amended             bool    `json:"amended"`
	OriginalType        *string `json:"original_type,omitempty"`
	OriginalSeverity    *string `json:"original_severity,omitempty"`
	OriginalDescription *string `json:"original_description,omitempty"`
	OriginalLocation    *string `json:"original_location,omitempty"`
}

// IncidentAmendment is a supervisor-approved correction to an incident.
type IncidentAmendment struct {
	ID         string          `json:"id"`
	IncidentID string          `json:"incident_id"`
	Changes    json.RawMessage `json:"changes"`
	Reason     string          `json:"reason"`
	AmendedBy  string          `json:"amended_by"`
	CreatedAt  time.Time       `json:"created_at"`
}

// DefectReport records a facility or equipment issue filed by a guard.
type DefectReport struct {
	ID             string    `json:"id"`
	SiteID         string    `json:"site_id"`
	GuardID        string    `json:"guard_id"`
	GuardName      string    `json:"guard_name,omitempty"`
	DeviceID       *string   `json:"device_id,omitempty"`
	DefectType     *string   `json:"defect_type,omitempty"`
	Location       *string   `json:"location,omitempty"`
	Description    *string   `json:"description,omitempty"`
	PhotoObjectKey *string   `json:"photo_object_key,omitempty"`
	Status         string    `json:"status"` // open | in_progress | resolved
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	Flags          []string  `json:"flags"` // e.g. guard_unverified
}

// VisitorLog records a visitor or vehicle entering a site.
type VisitorLog struct {
	ID             string     `json:"id"`
	SiteID         string     `json:"site_id"`
	GuardID        string     `json:"guard_id"`
	GuardName      string     `json:"guard_name,omitempty"`
	DeviceID       *string    `json:"device_id,omitempty"`
	VisitorName    string     `json:"visitor_name"`
	IDType         *string    `json:"id_type,omitempty"`
	IDNumber       *string    `json:"id_number,omitempty"`
	Plate          *string    `json:"plate,omitempty"`
	HostName       *string    `json:"host_name,omitempty"`
	EntryAt        *time.Time `json:"entry_at,omitempty"`
	ExitAt         *time.Time `json:"exit_at,omitempty"`
	Purpose        *string    `json:"purpose,omitempty"`
	PhotoObjectKey *string    `json:"photo_object_key,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	Flags          []string   `json:"flags"` // e.g. guard_unverified
}

// SosAlert records a panic/SOS event triggered by a guard.
type SosAlert struct {
	ID             string     `json:"id"`
	GuardID        string     `json:"guard_id"`
	GuardName      string     `json:"guard_name,omitempty"`
	SiteID         string     `json:"site_id"`
	DeviceID       *string    `json:"device_id,omitempty"`
	FiredAt        time.Time  `json:"fired_at"`
	ReceivedAt     time.Time  `json:"received_at"`
	GPSLat         *float64   `json:"gps_lat,omitempty"`
	GPSLng         *float64   `json:"gps_lng,omitempty"`
	AcknowledgedBy *string    `json:"acknowledged_by,omitempty"`
	AcknowledgedAt *time.Time `json:"acknowledged_at,omitempty"`
	Flags          []string   `json:"flags"`
}

// NotificationRule routes an event type at a site to a channel recipient.
type NotificationRule struct {
	ID        string    `json:"id"`
	SiteID    string    `json:"site_id"`
	EventType string    `json:"event_type"`
	Channel   string    `json:"channel"` // email | whatsapp
	Recipient string    `json:"recipient"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
}

// NotificationLog is one delivery attempt record (one per recipient).
type NotificationLog struct {
	ID        string     `json:"id"`
	SiteID    *string    `json:"site_id,omitempty"`
	EventType string     `json:"event_type"`
	Channel   string     `json:"channel"`
	Recipient string     `json:"recipient"`
	Subject   string     `json:"subject"`
	Body      string     `json:"body"`
	Status    string     `json:"status"` // queued | sent | failed
	Attempts  int        `json:"attempts"`
	LastError *string    `json:"last_error,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	SentAt    *time.Time `json:"sent_at,omitempty"`
}

// Backup is one pg_dump run stored in MinIO.
type Backup struct {
	ID          string     `json:"id"`
	ObjectKey   string     `json:"object_key"`
	SizeBytes   *int64     `json:"size_bytes,omitempty"`
	Status      string     `json:"status"`  // pending | running | succeeded | failed
	Trigger     string     `json:"trigger"` // scheduled | manual
	Error       *string    `json:"error,omitempty"`
	RequestedBy *string    `json:"requested_by,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	FinishedAt  *time.Time `json:"finished_at,omitempty"`
	// Keycloak database dump of the same run (when configured).
	KeycloakObjectKey *string `json:"keycloak_object_key,omitempty"`
	KeycloakSizeBytes *int64  `json:"keycloak_size_bytes,omitempty"`
}

// ── Ownership of records created by idempotent tablet sync (BE-S2) ──────────

// Owner returns the scope (site) and guard that created the record.
func (r *AttendanceRecord) Owner() (string, string) { return r.SiteID, r.GuardID }

// Owner returns the site and guard of the session.
func (s *PatrolSession) Owner() (string, string) { return s.SiteID, s.GuardID }

// Owner returns the session (scans have no site column) and guard.
func (s *CheckpointScan) Owner() (string, string) {
	if s.SessionID == nil {
		return "", s.GuardID
	}
	return *s.SessionID, s.GuardID
}

// Owner returns the site and guard of the incident.
func (i *Incident) Owner() (string, string) { return i.SiteID, i.GuardID }

// Owner returns the site and guard of the defect report.
func (d *DefectReport) Owner() (string, string) { return d.SiteID, d.GuardID }

// Owner returns the site and guard of the visitor log.
func (v *VisitorLog) Owner() (string, string) { return v.SiteID, v.GuardID }

// Owner returns the site and guard of the SOS alert.
func (a *SosAlert) Owner() (string, string) { return a.SiteID, a.GuardID }

// AuditLog records a mutating API request for compliance (NFR-05). Rows
// are append-only.
type AuditLog struct {
	ID           string          `json:"id"`
	UserID       string          `json:"user_id"` // "realm/actor[ guard:<id>]" (legacy display)
	Realm        *string         `json:"realm,omitempty"`
	Actor        *string         `json:"actor,omitempty"` // username, or subject
	GuardID      *string         `json:"guard_id,omitempty"`
	Roles        []string        `json:"roles"`
	Action       string          `json:"action"` // "METHOD /route/:pattern"
	Method       *string         `json:"method,omitempty"`
	Route        *string         `json:"route,omitempty"`
	ResourceType *string         `json:"resource_type,omitempty"` // e.g. guards, patrol.routes
	ResourceID   *string         `json:"resource_id,omitempty"`
	Params       json.RawMessage `json:"params,omitempty"` // route params, e.g. {"shiftId": "…"}
	RequestID    *string         `json:"request_id,omitempty"`
	Summary      json.RawMessage `json:"summary,omitempty"` // redacted request body (web writes)
	Before       json.RawMessage `json:"before,omitempty"`
	After        json.RawMessage `json:"after,omitempty"`
	IP           string          `json:"ip"`
	UserAgent    string          `json:"user_agent"`
	StatusCode   int             `json:"status_code"`
	CreatedAt    time.Time       `json:"created_at"`
}
