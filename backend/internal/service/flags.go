package service

import (
	"math"
	"time"

	"github.com/securepatrol/backend/internal/fieldauth"
	"github.com/securepatrol/backend/internal/model"
)

// Plausibility flags on field records (BE-S1, D-035). They do not reject
// anything: they make records that may have been forged visible to
// supervisors.
const (
	// FlagLateSync: the record reached the server more than LateSyncAfter
	// after its event time (offline queue or a backdated client clock).
	FlagLateSync = "late_sync"
	// FlagClockAhead: the event time is ahead of the server clock by more
	// than ClockAheadAfter (still inside the accepted +2 min window).
	FlagClockAhead = "clock_ahead"
	// FlagBackdatedOnTime: an on-time clock-in arrived more than
	// LateSyncAfter late although the tablet talked to the server after the
	// claimed time — not explained by being offline. Sets needs_review.
	FlagBackdatedOnTime = "backdated_on_time"
	// FlagGPSFar: the scan's GPS fix is farther from the checkpoint than the
	// site's scan_radius_m.
	FlagGPSFar = "gps_far"
	// FlagTooFast: the guard would have moved faster than MaxWalkSpeed from
	// the previous checkpoint scanned in the session.
	FlagTooFast = "too_fast"
	// FlagGuardUnverified: the record was made without a valid guard token
	// (PIN only checked on the tablet, e.g. offline). On a clock-in it also
	// sets needs_review.
	FlagGuardUnverified = "guard_unverified"
	// FlagQRInvalid: a signed checkpoint QR payload failed verification
	// (forged, or printed before the codes were regenerated).
	FlagQRInvalid = "qr_invalid"
	// FlagQRUnsigned: a QR scan carried no signed payload (legacy plain
	// code, or only a checkpoint ID).
	FlagQRUnsigned = "qr_unsigned"
)

// GuardFlags returns [guard_unverified] unless auth verifies the acting
// guard for a record with the given event time.
func GuardFlags(auth *fieldauth.GuardAuth, event time.Time) []string {
	if auth.VerifiedAt(event, Now()) {
		return nil
	}
	return []string{FlagGuardUnverified}
}

// Thresholds of the plausibility checks.
const (
	LateSyncAfter    = 10 * time.Minute
	ClockAheadAfter  = 30 * time.Second
	MaxWalkSpeed     = 3.0  // m/s (brisk walk / light jog)
	minSpeedDistance = 50.0 // m; shorter hops are within GPS/placement noise
)

// TimeFlags compares a client event time with the server receipt time.
func TimeFlags(event, received time.Time) []string {
	flags := []string{}
	switch {
	case received.Sub(event) > LateSyncAfter:
		flags = append(flags, FlagLateSync)
	case event.Sub(received) > ClockAheadAfter:
		flags = append(flags, FlagClockAhead)
	}
	return flags
}

// ClockInFlags adds the clock-in specific check to TimeFlags. deviceLastSeen
// is when the tablet was last seen before this request. needsReview is true
// when a supervisor should confirm the record.
func ClockInFlags(event, received time.Time, status string, deviceLastSeen *time.Time) (flags []string, needsReview bool) {
	flags = TimeFlags(event, received)
	if status == "present" && received.Sub(event) > LateSyncAfter &&
		deviceLastSeen != nil && deviceLastSeen.After(event.Add(2*time.Minute)) {
		flags = append(flags, FlagBackdatedOnTime)
		needsReview = true
	}
	return flags, needsReview
}

// ScanContext is what the scan checks need besides the scan itself.
type ScanContext struct {
	RadiusMeters   int               // 0 disables the GPS check
	Checkpoint     model.Checkpoint  // scanned checkpoint
	Previous       *model.Checkpoint // checkpoint of the previous scan in the session
	PreviousAt     time.Time
	GPSLat, GPSLng *float64
}

// ScanFlags checks a scan's GPS fix against the checkpoint and the speed
// from the previous checkpoint.
func ScanFlags(at time.Time, sc ScanContext) []string {
	flags := []string{}
	cp := sc.Checkpoint
	if sc.RadiusMeters > 0 && sc.GPSLat != nil && sc.GPSLng != nil && cp.Lat != nil && cp.Lng != nil &&
		DistanceMeters(*sc.GPSLat, *sc.GPSLng, *cp.Lat, *cp.Lng) > float64(sc.RadiusMeters) {
		flags = append(flags, FlagGPSFar)
	}
	if p := sc.Previous; p != nil && p.ID != cp.ID && p.Lat != nil && p.Lng != nil && cp.Lat != nil && cp.Lng != nil {
		d := DistanceMeters(*p.Lat, *p.Lng, *cp.Lat, *cp.Lng)
		secs := at.Sub(sc.PreviousAt).Seconds()
		if d > minSpeedDistance && (secs <= 0 || d/secs > MaxWalkSpeed) {
			flags = append(flags, FlagTooFast)
		}
	}
	return flags
}

// DistanceMeters is the great-circle distance between two coordinates.
func DistanceMeters(lat1, lng1, lat2, lng2 float64) float64 {
	const earthRadius = 6371000.0
	rad := math.Pi / 180
	dLat, dLng := (lat2-lat1)*rad, (lng2-lng1)*rad
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Sin(dLng/2)*math.Sin(dLng/2)
	return 2 * earthRadius * math.Asin(math.Min(1, math.Sqrt(a)))
}
