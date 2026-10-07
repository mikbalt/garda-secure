package service

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/securepatrol/backend/internal/model"
	"github.com/securepatrol/backend/internal/repository"
)

// StaleSessionAge is when an in-progress session is auto-abandoned (D-008).
const StaleSessionAge = 12 * time.Hour

// NormalizeRoute validates checkpoints, assigns missing IDs and renumbers
// order_index 0..n-1 following the submitted order.
func NormalizeRoute(r *model.Route) error {
	r.Name = strings.TrimSpace(r.Name)
	if r.Name == "" {
		return BadRequest("name_required", "route name is required")
	}
	if len(r.Checkpoints) == 0 {
		return BadRequest("checkpoints_required", "a route needs at least one checkpoint")
	}
	sort.SliceStable(r.Checkpoints, func(i, j int) bool {
		return r.Checkpoints[i].OrderIndex < r.Checkpoints[j].OrderIndex
	})
	ids, codes := map[string]bool{}, map[string]bool{}
	for i := range r.Checkpoints {
		cp := &r.Checkpoints[i]
		cp.Name = strings.TrimSpace(cp.Name)
		cp.QRCode = strings.TrimSpace(cp.QRCode)
		if cp.ID == "" {
			cp.ID = uuid.NewString()
		}
		if cp.QRCode == "" {
			cp.QRCode = "SP-" + strings.ToUpper(strings.ReplaceAll(cp.ID, "-", "")[:12])
		}
		if cp.Name == "" {
			return BadRequest("checkpoint_name_required", "checkpoint %d has no name", i+1)
		}
		if ids[cp.ID] {
			return BadRequest("duplicate_checkpoint", "duplicate checkpoint id %q", cp.ID)
		}
		if codes[cp.QRCode] {
			return BadRequest("duplicate_qr_code", "duplicate QR code %q", cp.QRCode)
		}
		if cp.SLAMinutes != nil && (*cp.SLAMinutes < 1 || *cp.SLAMinutes > 24*60) {
			return BadRequest("invalid_sla", "checkpoint %q: sla_minutes must be between 1 and 1440", cp.Name)
		}
		ids[cp.ID], codes[cp.QRCode] = true, true
		cp.OrderIndex = i
	}
	return nil
}

// CheckpointProgress is the state of one checkpoint within a session.
type CheckpointProgress struct {
	model.Checkpoint
	ScanID    *string    `json:"scan_id,omitempty"`
	ScannedAt *time.Time `json:"scanned_at,omitempty"`
	Method    *string    `json:"method,omitempty"`
	ScannedBy *string    `json:"scanned_by,omitempty"`
	DueAt     *time.Time `json:"due_at,omitempty"`
	// Status: on_time | late | overdue (not scanned, past due) | missed
	// (session ended without a scan) | pending.
	Status string `json:"status"`
}

// SessionProgress summarises a session against its route.
type SessionProgress struct {
	Session       model.PatrolSession  `json:"session"`
	RouteName     string               `json:"route_name"`
	Checkpoints   []CheckpointProgress `json:"checkpoints"`
	Total         int                  `json:"total"`
	Scanned       int                  `json:"scanned"`
	CompletionPct float64              `json:"completion_pct"`
	NextIndex     *int                 `json:"next_index,omitempty"`
}

// ComputeProgress evaluates a session at time now. The SLA of checkpoint i
// runs from the previous checkpoint's scan (session start for the first; the
// previous due time when the previous one was skipped).
func ComputeProgress(route model.Route, s model.PatrolSession, scans []model.CheckpointScan, now time.Time) SessionProgress {
	first := map[string]model.CheckpointScan{}
	for _, sc := range scans {
		if prev, ok := first[sc.CheckpointID]; !ok || sc.ScannedAt.Before(prev.ScannedAt) {
			first[sc.CheckpointID] = sc
		}
	}
	ended := s.Status != "in_progress"
	p := SessionProgress{Session: s, RouteName: route.Name, Total: len(route.Checkpoints)}
	anchor := s.StartedAt
	for i, cp := range route.Checkpoints {
		cpp := CheckpointProgress{Checkpoint: cp}
		var due *time.Time
		if cp.SLAMinutes != nil {
			d := anchor.Add(time.Duration(*cp.SLAMinutes) * time.Minute)
			due = &d
			cpp.DueAt = due
		}
		if sc, ok := first[cp.ID]; ok {
			id, at, method, by := sc.ID, sc.ScannedAt, sc.Method, sc.GuardID
			cpp.ScanID, cpp.ScannedAt, cpp.Method, cpp.ScannedBy = &id, &at, &method, &by
			cpp.Status = "on_time"
			if due != nil && at.After(*due) {
				cpp.Status = "late"
			}
			p.Scanned++
			anchor = at
		} else {
			switch {
			case ended:
				cpp.Status = "missed"
			case due != nil && now.After(*due):
				cpp.Status = "overdue"
			default:
				cpp.Status = "pending"
			}
			if p.NextIndex == nil && !ended {
				idx := i
				p.NextIndex = &idx
			}
			if due != nil {
				anchor = *due
			}
		}
		p.Checkpoints = append(p.Checkpoints, cpp)
	}
	if p.Total > 0 {
		p.CompletionPct = float64(int(float64(p.Scanned)/float64(p.Total)*1000+0.5)) / 10
	}
	if p.Checkpoints == nil {
		p.Checkpoints = []CheckpointProgress{}
	}
	return p
}

// LoadProgress fetches a session's route and scans and computes progress.
func LoadProgress(ctx context.Context, db repository.DBTX, s model.PatrolSession) (*SessionProgress, error) {
	route, err := repository.GetRoute(ctx, db, s.RouteID)
	if err != nil {
		return nil, err
	}
	scans, err := repository.ListSessionScans(ctx, db, s.ID)
	if err != nil {
		return nil, err
	}
	p := ComputeProgress(*route, s, scans, Now())
	return &p, nil
}

// StartSessionInput starts a patrol from a tablet.
type StartSessionInput struct {
	ID       *string
	RouteID  string
	At       *time.Time
	Guard    *model.Guard
	Site     *model.Site
	DeviceID *string
}

// StartSession validates the route and creates the session (idempotent on ID).
func StartSession(ctx context.Context, pool *pgxpool.Pool, in StartSessionInput) (*model.PatrolSession, bool, error) {
	at, err := ClientTime(in.At)
	if err != nil {
		return nil, false, err
	}
	route, err := repository.GetRoute(ctx, pool, in.RouteID)
	if errors.Is(err, repository.ErrNotFound) || repository.IsInvalidInput(err) {
		return nil, false, NotFound("route_not_found", "route not found")
	}
	if err != nil {
		return nil, false, err
	}
	if route.SiteID != in.Site.ID {
		return nil, false, Forbidden("route_other_site", "route belongs to another site")
	}
	if !route.Active {
		return nil, false, Conflict("route_inactive", "route is not active")
	}
	s, created, err := repository.CreateSession(ctx, pool, model.PatrolSession{
		RouteID: route.ID, SiteID: in.Site.ID, GuardID: in.Guard.ID, DeviceID: in.DeviceID, StartedAt: at,
	}, in.ID)
	if err != nil {
		return nil, false, err
	}
	if !created && s.SiteID != in.Site.ID {
		return nil, false, Conflict("id_conflict", "session id already used")
	}
	return s, created, nil
}

// ScanInput records a checkpoint scan from a tablet.
type ScanInput struct {
	ID             *string
	SessionID      string
	CheckpointID   string // either the checkpoint ID …
	QRCode         string // … or the scanned QR payload
	Method         string // qr | manual
	PhotoObjectKey *string
	At             *time.Time
	GPSLat, GPSLng *float64
	Guard          *model.Guard
	Site           *model.Site
	DeviceID       *string
}

// RecordScan validates a scan against the session's route (FR-PAT-02/03).
func RecordScan(ctx context.Context, pool *pgxpool.Pool, in ScanInput) (*model.CheckpointScan, bool, error) {
	at, err := ClientTime(in.At)
	if err != nil {
		return nil, false, err
	}
	if in.Method == "" {
		in.Method = "qr"
	}
	if in.Method != "qr" && in.Method != "manual" {
		return nil, false, BadRequest("invalid_method", "method must be qr or manual")
	}
	if in.Method == "manual" && (in.PhotoObjectKey == nil || *in.PhotoObjectKey == "") {
		return nil, false, BadRequest("photo_required", "manual check-in requires a photo")
	}
	if err := CheckObjectKey(in.PhotoObjectKey, in.Site.ID); err != nil {
		return nil, false, err
	}

	s, err := repository.GetSession(ctx, pool, in.SessionID)
	if errors.Is(err, repository.ErrNotFound) || repository.IsInvalidInput(err) {
		return nil, false, NotFound("session_not_found", "patrol session not found")
	}
	if err != nil {
		return nil, false, err
	}
	if s.SiteID != in.Site.ID {
		return nil, false, Forbidden("session_other_site", "session belongs to another site")
	}
	if at.Before(s.StartedAt) {
		return nil, false, BadRequest("before_session_start", "scan time is before the session started")
	}
	if s.EndedAt != nil && at.After(*s.EndedAt) {
		return nil, false, Conflict("session_ended", "session already ended")
	}
	route, err := repository.GetRoute(ctx, pool, s.RouteID)
	if err != nil {
		return nil, false, err
	}
	var cp *model.Checkpoint
	for i := range route.Checkpoints {
		c := &route.Checkpoints[i]
		if (in.CheckpointID != "" && c.ID == in.CheckpointID) || (in.QRCode != "" && c.QRCode == in.QRCode) {
			cp = c
			break
		}
	}
	if cp == nil {
		return nil, false, BadRequest("unknown_checkpoint", "checkpoint is not part of this route")
	}

	sessionID := s.ID
	return repository.InsertScan(ctx, pool, model.CheckpointScan{
		SessionID: &sessionID, RouteID: route.ID, CheckpointID: cp.ID, GuardID: in.Guard.ID,
		DeviceID: in.DeviceID, ScannedAt: at, Method: in.Method, PhotoObjectKey: in.PhotoObjectKey,
		GPSLat: in.GPSLat, GPSLng: in.GPSLng,
	}, in.ID)
}

// EndSession completes a session (or marks it abandoned).
func EndSession(ctx context.Context, pool *pgxpool.Pool, sessionID string, site *model.Site, at *time.Time, abandoned bool) (*model.PatrolSession, error) {
	ts, err := ClientTime(at)
	if err != nil {
		return nil, err
	}
	s, err := repository.GetSession(ctx, pool, sessionID)
	if errors.Is(err, repository.ErrNotFound) || repository.IsInvalidInput(err) {
		return nil, NotFound("session_not_found", "patrol session not found")
	}
	if err != nil {
		return nil, err
	}
	if s.SiteID != site.ID {
		return nil, Forbidden("session_other_site", "session belongs to another site")
	}
	if s.Status != "in_progress" {
		return s, nil // idempotent
	}
	if ts.Before(s.StartedAt) {
		ts = s.StartedAt
	}
	status := "completed"
	if abandoned {
		status = "abandoned"
	}
	out, err := repository.EndSession(ctx, pool, s.ID, ts, status)
	if errors.Is(err, repository.ErrNotFound) {
		return repository.GetSession(ctx, pool, s.ID) // ended concurrently
	}
	return out, err
}
