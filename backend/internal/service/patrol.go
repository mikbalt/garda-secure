package service

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/securepatrol/backend/internal/fieldauth"
	"github.com/securepatrol/backend/internal/model"
	"github.com/securepatrol/backend/internal/repository"
)

// MaxCheckpoints caps the checkpoints of one route.
const MaxCheckpoints = 500

// StaleSessionAge is when an in-progress session is auto-abandoned (D-008).
const StaleSessionAge = 12 * time.Hour

// NormalizeRoute validates checkpoints, assigns missing IDs and renumbers
// order_index 0..n-1 following the submitted order. prev is the stored route
// on an update (nil on create).
//
// QR codes (D-035), with qr != nil: a new checkpoint without a custom code
// gets a signed payload (version 1); a checkpoint that was signed stays
// signed with its version (the submitted qr_code is ignored); a legacy
// checkpoint keeps its custom code, or becomes signed when the code is
// cleared. With qr == nil plain "SP-…" codes are generated (legacy).
func NormalizeRoute(r *model.Route, prev *model.Route, qr *fieldauth.QRSigner) error {
	r.Name = strings.TrimSpace(r.Name)
	if r.Name == "" {
		return BadRequest("name_required", "route name is required")
	}
	if err := CheckLengths(S("name", r.Name, MaxName)); err != nil {
		return err
	}
	if len(r.Checkpoints) == 0 {
		return BadRequest("checkpoints_required", "a route needs at least one checkpoint")
	}
	if len(r.Checkpoints) > MaxCheckpoints {
		return BadRequest("too_many_checkpoints", "a route can have at most %d checkpoints", MaxCheckpoints)
	}
	sort.SliceStable(r.Checkpoints, func(i, j int) bool {
		return r.Checkpoints[i].OrderIndex < r.Checkpoints[j].OrderIndex
	})
	stored := map[string]model.Checkpoint{}
	if prev != nil {
		for _, cp := range prev.Checkpoints {
			stored[cp.ID] = cp
		}
	}
	ids, codes := map[string]bool{}, map[string]bool{}
	for i := range r.Checkpoints {
		cp := &r.Checkpoints[i]
		cp.Name = strings.TrimSpace(cp.Name)
		cp.QRCode = strings.TrimSpace(cp.QRCode)
		old, existed := stored[cp.ID]
		if cp.ID == "" {
			cp.ID = uuid.NewString()
		}
		assignQR(cp, old, existed, qr)
		// GPS positions are learned from scans (LearnCheckpointPositions, D-059);
		// submitted values are ignored and a stored position is kept.
		cp.Lat, cp.Lng = nil, nil
		if existed {
			cp.Lat, cp.Lng = old.Lat, old.Lng
		}
		if cp.Name == "" {
			return BadRequest("checkpoint_name_required", "checkpoint %d has no name", i+1)
		}
		if err := CheckLengths(S("checkpoints.name", cp.Name, MaxName), S("checkpoints.qr_code", cp.QRCode, MaxName),
			S("checkpoints.id", cp.ID, MaxIdentifier)); err != nil {
			return err
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
	ScanFlags []string   `json:"scan_flags,omitempty"` // plausibility flags of the scan
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
			if len(sc.Flags) > 0 {
				cpp.ScanFlags = sc.Flags
			}
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

// LoadProgress fetches a session's route (its snapshot from the session
// start, F9) and scans and computes progress.
func LoadProgress(ctx context.Context, db repository.DBTX, s model.PatrolSession) (*SessionProgress, error) {
	route, err := repository.GetSessionRoute(ctx, db, s.ID)
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

// LoadProgressBatch computes the progress of many sessions with two queries
// (routes incl. snapshots, and scans) instead of two per session.
func LoadProgressBatch(ctx context.Context, db repository.DBTX, sessions []model.PatrolSession) ([]SessionProgress, error) {
	out := make([]SessionProgress, 0, len(sessions))
	if len(sessions) == 0 {
		return out, nil
	}
	ids := make([]string, len(sessions))
	for i, s := range sessions {
		ids[i] = s.ID
	}
	routes, err := repository.ListSessionRoutes(ctx, db, ids)
	if err != nil {
		return nil, err
	}
	scans, err := repository.ListScansForSessions(ctx, db, ids)
	if err != nil {
		return nil, err
	}
	now := Now()
	for _, s := range sessions {
		route, ok := routes[s.ID]
		if !ok {
			continue // deleted concurrently
		}
		out = append(out, ComputeProgress(route, s, scans[s.ID], now))
	}
	return out, nil
}

// StartSessionInput starts a patrol from a tablet.
type StartSessionInput struct {
	ID       *string
	RouteID  string
	At       *time.Time
	Guard    *model.Guard
	Auth     *fieldauth.GuardAuth // guard token proof; nil = unverified
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
		Flags: GuardFlags(in.Auth, at),
	}, in.ID)
	if err != nil {
		return nil, false, err
	}
	if err := CheckReplay(s, created, in.Site.ID, in.Guard.ID); err != nil {
		return nil, false, err
	}
	return s, created, nil
}

// ScanInput records a checkpoint scan from a tablet.
type ScanInput struct {
	ID             *string
	SessionID      string
	CheckpointID   string // either the checkpoint ID …
	QRCode         string // … or a legacy plain QR code …
	QRPayload      string // … or the raw scanned text (signed SP1:<id>:<sig> or plain)
	Method         string // qr | manual
	PhotoObjectKey *string
	At             *time.Time
	GPSLat, GPSLng *float64
	Guard          *model.Guard
	Auth           *fieldauth.GuardAuth // guard token proof; nil = unverified
	Site           *model.Site
	DeviceID       *string
	QR             *fieldauth.QRSigner
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
	// Scans match the CURRENT route (not the F9 snapshot): QR regeneration
	// must invalidate old prints immediately, also in running sessions.
	route, err := repository.GetRoute(ctx, pool, s.RouteID)
	if err != nil {
		return nil, false, err
	}
	cp, qrFlag := resolveCheckpoint(route, in)
	if cp == nil {
		return nil, false, BadRequest("unknown_checkpoint", "checkpoint is not part of this route")
	}

	flags, err := scanFlags(ctx, pool, s.ID, route, *cp, at, in)
	if err != nil {
		return nil, false, err
	}
	if qrFlag != "" {
		flags = append(flags, qrFlag)
	}
	flags = append(flags, GuardFlags(in.Auth, at)...)
	sessionID := s.ID
	scan, created, err := repository.InsertScan(ctx, pool, model.CheckpointScan{
		SessionID: &sessionID, RouteID: route.ID, CheckpointID: cp.ID, GuardID: in.Guard.ID,
		DeviceID: in.DeviceID, ScannedAt: at, Method: in.Method, PhotoObjectKey: in.PhotoObjectKey,
		GPSLat: in.GPSLat, GPSLng: in.GPSLng, Flags: flags,
	}, in.ID)
	if err != nil {
		return nil, false, err
	}
	if err := CheckReplay(scan, created, s.ID, in.Guard.ID); err != nil {
		return nil, false, err
	}
	return scan, created, nil
}

// resolveCheckpoint finds the scanned checkpoint of the route and the QR
// verification flag of a QR scan (D-035):
//   - a signed payload (qr_payload, or an SP1: text in qr_code) names the
//     checkpoint; a signature that does not match its current version is
//     accepted but flagged qr_invalid;
//   - a plain code matches a legacy checkpoint's qr_code: qr_unsigned;
//   - a checkpoint_id alone proves no scan: qr_unsigned.
//
// Manual check-ins (photo required) get no QR flag.
func resolveCheckpoint(route *model.Route, in ScanInput) (*model.Checkpoint, string) {
	byID := func(id string) *model.Checkpoint {
		for i := range route.Checkpoints {
			if route.Checkpoints[i].ID == id {
				return &route.Checkpoints[i]
			}
		}
		return nil
	}
	byLegacyCode := func(code string) *model.Checkpoint {
		for i := range route.Checkpoints {
			if c := &route.Checkpoints[i]; !c.QRSigned && c.QRCode == code {
				return c
			}
		}
		return nil
	}
	qrFlag := func(f string) string {
		if in.Method == "manual" {
			return ""
		}
		return f
	}
	payload := strings.TrimSpace(in.QRPayload)
	if payload == "" && strings.HasPrefix(in.QRCode, fieldauth.QRPrefix) {
		payload = in.QRCode
	}
	switch {
	case payload != "":
		id, sig, ok := fieldauth.ParseQR(payload)
		if !ok { // raw text of a legacy plain code
			if cp := byLegacyCode(payload); cp != nil {
				return cp, qrFlag(FlagQRUnsigned)
			}
			return nil, ""
		}
		cp := byID(id)
		if cp == nil {
			return nil, ""
		}
		if cp.QRSigned && in.QR != nil && in.QR.Valid(cp.ID, cp.QRVersion, sig) {
			return cp, ""
		}
		return cp, qrFlag(FlagQRInvalid)
	case in.QRCode != "":
		if cp := byLegacyCode(in.QRCode); cp != nil {
			return cp, qrFlag(FlagQRUnsigned)
		}
		return nil, ""
	case in.CheckpointID != "":
		if cp := byID(in.CheckpointID); cp != nil {
			return cp, qrFlag(FlagQRUnsigned)
		}
	}
	return nil, ""
}

// scanFlags runs the time, GPS and speed plausibility checks of a scan
// against the previous scan of the session.
func scanFlags(ctx context.Context, db repository.DBTX, sessionID string, route *model.Route,
	cp model.Checkpoint, at time.Time, in ScanInput) ([]string, error) {
	sc := ScanContext{RadiusMeters: in.Site.Config().ScanRadiusMeters, Checkpoint: cp, GPSLat: in.GPSLat, GPSLng: in.GPSLng}
	scans, err := repository.ListSessionScans(ctx, db, sessionID)
	if err != nil {
		return nil, err
	}
	for i := len(scans) - 1; i >= 0; i-- { // ordered by scanned_at
		if !scans[i].ScannedAt.After(at) {
			for k := range route.Checkpoints {
				if route.Checkpoints[k].ID == scans[i].CheckpointID {
					sc.Previous, sc.PreviousAt = &route.Checkpoints[k], scans[i].ScannedAt
				}
			}
			break
		}
	}
	return append(TimeFlags(at, Now()), ScanFlags(at, sc)...), nil
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

// assignQR decides a checkpoint's QR code on save (see NormalizeRoute).
func assignQR(cp *model.Checkpoint, old model.Checkpoint, existed bool, qr *fieldauth.QRSigner) {
	custom := cp.QRCode != "" && !strings.HasPrefix(cp.QRCode, fieldauth.QRPrefix)
	cp.QRSigned, cp.QRVersion = false, 0
	if existed {
		cp.QRVersion = old.QRVersion
	}
	switch {
	case qr == nil:
		if !custom {
			cp.QRCode = "SP-" + strings.ToUpper(strings.ReplaceAll(cp.ID, "-", "")[:12])
		}
		return
	case existed && old.QRSigned:
		cp.QRSigned = true
	case custom:
		return // legacy custom code (kept or newly typed)
	default:
		cp.QRSigned = true
		cp.QRVersion = old.QRVersion + 1 // new checkpoint: 1; cleared legacy code: next version
	}
	cp.QRCode = qr.Payload(cp.ID, cp.QRVersion)
}

// RegenerateQR switches every checkpoint of a route to a signed code with a
// new version, so all previously printed codes of the route stop verifying.
func RegenerateQR(r *model.Route, qr *fieldauth.QRSigner) {
	for i := range r.Checkpoints {
		cp := &r.Checkpoints[i]
		cp.QRSigned = true
		cp.QRVersion++
		cp.QRCode = qr.Payload(cp.ID, cp.QRVersion)
	}
}

// DecorateQR fills in the current signed payload of every signed checkpoint
// (web responses; the stored text may predate a key change).
func DecorateQR(r *model.Route, qr *fieldauth.QRSigner) {
	for i := range r.Checkpoints {
		if cp := &r.Checkpoints[i]; cp.QRSigned && qr != nil {
			cp.QRCode = qr.Payload(cp.ID, cp.QRVersion)
		}
	}
}

// HideSignedQR blanks the payload of signed checkpoints in a session's
// progress before it goes to a tablet: tablets must never learn valid
// payloads (they would let a guard "scan" without being there).
func HideSignedQR(p *SessionProgress) {
	if p == nil {
		return
	}
	for i := range p.Checkpoints {
		if p.Checkpoints[i].QRSigned {
			p.Checkpoints[i].QRCode = ""
		}
	}
}
