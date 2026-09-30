package model

import (
	"encoding/json"
	"time"
)

// Guard represents a security guard user.
type Guard struct {
	ID              string     `json:"id"`
	KeycloakUserID  *string    `json:"keycloak_user_id,omitempty"`
	Name            string     `json:"name"`
	EmployeeID      *string    `json:"employee_id,omitempty"`
	Role            string     `json:"role"`
	PinHash         *string    `json:"-"` // never serialised to JSON
	Status          string     `json:"status"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
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

// Shift represents a scheduled work period for a guard at a site.
type Shift struct {
	ID         string     `json:"id"`
	GuardID    string     `json:"guard_id"`
	SiteID     string     `json:"site_id"`
	StartTime  time.Time  `json:"start_time"`
	EndTime    time.Time  `json:"end_time"`
	RepeatRule *string    `json:"repeat_rule,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

// AttendanceRecord stores a guard's clock-in/out for a shift.
type AttendanceRecord struct {
	ID               string     `json:"id"`
	ShiftID          *string    `json:"shift_id,omitempty"`
	GuardID          string     `json:"guard_id"`
	SiteID           string     `json:"site_id"`
	ClockInAt        *time.Time `json:"clock_in_at,omitempty"`
	ClockOutAt       *time.Time `json:"clock_out_at,omitempty"`
	Status           string     `json:"status"` // pending | present | absent | late | early_out
	SelfieObjectKey  *string    `json:"selfie_object_key,omitempty"`
	GPSLat           *float64   `json:"gps_lat,omitempty"`
	GPSLng           *float64   `json:"gps_lng,omitempty"`
	OverrideReason   *string    `json:"override_reason,omitempty"`
	OverriddenBy     *string    `json:"overridden_by,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
}

// Checkpoint is an individual stop on a patrol route (stored as JSONB inside Route).
type Checkpoint struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	QRCode     string  `json:"qr_code"`
	SLAMinutes *int    `json:"sla_minutes,omitempty"` // nil = no SLA enforced
	OrderIndex int     `json:"order_index"`
}

// Route defines an ordered set of checkpoints for a patrol at a site.
type Route struct {
	ID          string          `json:"id"`
	SiteID      string          `json:"site_id"`
	Name        string          `json:"name"`
	Checkpoints json.RawMessage `json:"checkpoints"` // []Checkpoint serialised as JSONB
	Active      bool            `json:"active"`
	CreatedAt   time.Time       `json:"created_at"`
}

// CheckpointScan records a guard scanning a checkpoint during a patrol.
type CheckpointScan struct {
	ID              string    `json:"id"`
	RouteID         string    `json:"route_id"`
	CheckpointID    string    `json:"checkpoint_id"`
	GuardID         string    `json:"guard_id"`
	ScannedAt       time.Time `json:"scanned_at"`
	Method          string    `json:"method"` // qr | manual
	PhotoObjectKey  *string   `json:"photo_object_key,omitempty"`
	GPSLat          *float64  `json:"gps_lat,omitempty"`
	GPSLng          *float64  `json:"gps_lng,omitempty"`
}

// Incident records a security event filed by a guard.
type Incident struct {
	ID              string          `json:"id"`
	SiteID          string          `json:"site_id"`
	GuardID         string          `json:"guard_id"`
	Type            string          `json:"type"`
	Severity        string          `json:"severity"` // low | medium | high | critical
	Description     *string         `json:"description,omitempty"`
	PhotoObjectKeys json.RawMessage `json:"photo_object_keys"` // []string
	Location        *string         `json:"location,omitempty"`
	Status          string          `json:"status"` // open | in_progress | resolved
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

// DefectReport records a facility or equipment issue filed by a guard.
type DefectReport struct {
	ID             string    `json:"id"`
	SiteID         string    `json:"site_id"`
	GuardID        string    `json:"guard_id"`
	DefectType     *string   `json:"defect_type,omitempty"`
	Location       *string   `json:"location,omitempty"`
	Description    *string   `json:"description,omitempty"`
	PhotoObjectKey *string   `json:"photo_object_key,omitempty"`
	Status         string    `json:"status"` // open | in_progress | resolved
	CreatedAt      time.Time `json:"created_at"`
}

// VisitorLog records a visitor or vehicle entering a site.
type VisitorLog struct {
	ID             string     `json:"id"`
	SiteID         string     `json:"site_id"`
	GuardID        string     `json:"guard_id"`
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
}

// SosAlert records a panic/SOS event triggered by a guard.
type SosAlert struct {
	ID               string     `json:"id"`
	GuardID          string     `json:"guard_id"`
	SiteID           string     `json:"site_id"`
	FiredAt          time.Time  `json:"fired_at"`
	GPSLat           *float64   `json:"gps_lat,omitempty"`
	GPSLng           *float64   `json:"gps_lng,omitempty"`
	AcknowledgedBy   *string    `json:"acknowledged_by,omitempty"`
	AcknowledgedAt   *time.Time `json:"acknowledged_at,omitempty"`
}

// AuditLog records admin/mutating actions for compliance.
type AuditLog struct {
	ID         string    `json:"id"`
	UserID     string    `json:"user_id"`
	Action     string    `json:"action"`
	ResourceID *string   `json:"resource_id,omitempty"`
	IP         string    `json:"ip"`
	UserAgent  string    `json:"user_agent"`
	StatusCode int       `json:"status_code"`
	CreatedAt  time.Time `json:"created_at"`
}
