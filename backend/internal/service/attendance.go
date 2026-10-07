package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/securepatrol/backend/internal/model"
	"github.com/securepatrol/backend/internal/repository"
)

// ClockInRequest holds validated input for a clock-in action.
type ClockInRequest struct {
	GuardID         string
	SiteID          string
	GPSLat          *float64
	GPSLng          *float64
	SelfieObjectKey *string
}

// ClockOutRequest holds validated input for a clock-out action.
type ClockOutRequest struct {
	AttendanceID string
	GPSLat       *float64
	GPSLng       *float64
}

// ClockIn validates and records a guard clocking in.
// Sets status to "present" or "late" based on shift start time.
func ClockIn(ctx context.Context, pool *pgxpool.Pool, req ClockInRequest) (*model.AttendanceRecord, error) {
	if req.GuardID == "" || req.SiteID == "" {
		return nil, errors.New("guard_id and site_id are required")
	}

	guard, err := repository.GetGuardByID(ctx, pool, req.GuardID)
	if err != nil {
		return nil, fmt.Errorf("guard not found: %w", err)
	}
	if guard.Status != "active" {
		return nil, fmt.Errorf("guard %s is not active (status=%s)", guard.Name, guard.Status)
	}

	today := todayStr()
	existing, err := repository.GetTodayClockIn(ctx, pool, req.GuardID, req.SiteID, today)
	if err == nil && existing != nil {
		return nil, errors.New("guard already clocked in today")
	}

	var shiftID *string
	status := "present"
	shift, err := repository.GetShiftForGuardOnDate(ctx, pool, req.GuardID, req.SiteID, today)
	if err == nil && shift != nil {
		shiftID = &shift.ID
		if time.Now().UTC().After(shift.StartTime.UTC().Add(10 * time.Minute)) {
			status = "late"
		}
	}

	return repository.ClockIn(ctx, pool, repository.ClockInRequest{
		GuardID:         req.GuardID,
		SiteID:          req.SiteID,
		ShiftID:         shiftID,
		GPSLat:          req.GPSLat,
		GPSLng:          req.GPSLng,
		SelfieObjectKey: req.SelfieObjectKey,
		Status:          status,
	})
}

// ClockOut validates and records a guard clocking out.
// Sets status to "early_out" if guard leaves >10 min before shift end.
func ClockOut(ctx context.Context, pool *pgxpool.Pool, req ClockOutRequest) (*model.AttendanceRecord, error) {
	if req.AttendanceID == "" {
		return nil, errors.New("attendance_id is required")
	}

	record, err := repository.GetAttendanceByID(ctx, pool, req.AttendanceID)
	if err != nil {
		return nil, fmt.Errorf("attendance record not found: %w", err)
	}
	if record.ClockOutAt != nil {
		return nil, errors.New("guard already clocked out")
	}

	newStatus := "present"
	if record.ShiftID != nil {
		shift, err := repository.GetShiftByID(ctx, pool, *record.ShiftID)
		if err == nil && time.Now().UTC().Before(shift.EndTime.UTC().Add(-10*time.Minute)) {
			newStatus = "early_out"
		}
	}

	return repository.ClockOut(ctx, pool, req.AttendanceID, newStatus, req.GPSLat, req.GPSLng)
}

// AutoFlagAbsent marks guards with shifts today but no attendance record as absent.
func AutoFlagAbsent(ctx context.Context, pool *pgxpool.Pool, siteID, date string) (int, error) {
	shifts, err := repository.GetShiftsForSiteOnDate(ctx, pool, siteID, date)
	if err != nil {
		return 0, fmt.Errorf("GetShiftsForSiteOnDate: %w", err)
	}

	count := 0
	for _, shift := range shifts {
		existing, err := repository.GetTodayClockIn(ctx, pool, shift.GuardID, siteID, date)
		if err != nil || existing == nil {
			if err := repository.InsertAbsent(ctx, pool, shift.GuardID, siteID, shift.ID, date); err == nil {
				count++
			}
		}
	}
	return count, nil
}
