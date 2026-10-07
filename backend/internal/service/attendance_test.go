package service

import (
	"testing"
	"time"
)

// TestIsLate verifies the 10-minute grace window logic.
func TestIsLate_BeforeGrace(t *testing.T) {
	// Shift starts 5 minutes ago — should NOT be late.
	shiftStart := time.Now().UTC().Add(-5 * time.Minute)
	if isLate(shiftStart) {
		t.Error("expected not late within 10-minute grace window")
	}
}

func TestIsLate_AfterGrace(t *testing.T) {
	// Shift started 15 minutes ago — should be late.
	shiftStart := time.Now().UTC().Add(-15 * time.Minute)
	if !isLate(shiftStart) {
		t.Error("expected late after 10-minute grace window")
	}
}

func TestIsLate_ExactlyAtGrace(t *testing.T) {
	// Shift started exactly 10 minutes ago — edge case, should NOT be late.
	shiftStart := time.Now().UTC().Add(-10 * time.Minute)
	// time.After is strictly after, so exactly 10 min is not late.
	if isLate(shiftStart) {
		t.Error("expected not late at exactly 10-minute mark")
	}
}

// TestIsEarlyOut verifies early-departure detection.
func TestIsEarlyOut_WellBeforeEnd(t *testing.T) {
	// Shift ends in 60 minutes — clocking out now is early.
	shiftEnd := time.Now().UTC().Add(60 * time.Minute)
	if !isEarlyOut(shiftEnd) {
		t.Error("expected early_out when 60 min remain on shift")
	}
}

func TestIsEarlyOut_AfterEnd(t *testing.T) {
	// Shift ended 5 minutes ago — not early out.
	shiftEnd := time.Now().UTC().Add(-5 * time.Minute)
	if isEarlyOut(shiftEnd) {
		t.Error("expected not early_out when shift already ended")
	}
}

func TestIsEarlyOut_WithinGrace(t *testing.T) {
	// Shift ends in 5 minutes — within 10-minute grace, not early out.
	shiftEnd := time.Now().UTC().Add(5 * time.Minute)
	if isEarlyOut(shiftEnd) {
		t.Error("expected not early_out within 10-minute end grace window")
	}
}

// TestTodayStr verifies date formatting is YYYY-MM-DD.
func TestTodayStr(t *testing.T) {
	result := todayStr()
	if len(result) != 10 {
		t.Errorf("expected 10-char date, got %q (len %d)", result, len(result))
	}
	if result[4] != '-' || result[7] != '-' {
		t.Errorf("expected YYYY-MM-DD format, got %q", result)
	}
}
