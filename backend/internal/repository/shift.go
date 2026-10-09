package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/securepatrol/backend/internal/model"
)

// ── Shift templates ──────────────────────────────────────────────────────────

const templateColumns = `id, site_id, guard_id,
	to_char(start_time, 'HH24:MI'), to_char(end_time, 'HH24:MI'), weekdays,
	to_char(valid_from, 'YYYY-MM-DD'), to_char(valid_until, 'YYYY-MM-DD'),
	active, created_at, updated_at`

func scanTemplate(row pgx.Row) (model.ShiftTemplate, error) {
	var t model.ShiftTemplate
	err := row.Scan(&t.ID, &t.SiteID, &t.GuardID, &t.StartTime, &t.EndTime, &t.Weekdays,
		&t.ValidFrom, &t.ValidUntil, &t.Active, &t.CreatedAt, &t.UpdatedAt)
	return t, err
}

func scanTemplateRows(rows pgx.Rows) (model.ShiftTemplate, error) { return scanTemplate(rows) }

// ListShiftTemplates returns the templates of a site.
func ListShiftTemplates(ctx context.Context, db DBTX, siteID string) ([]model.ShiftTemplate, error) {
	rows, err := db.Query(ctx,
		`SELECT `+templateColumns+` FROM shift_templates WHERE site_id = $1 ORDER BY start_time, created_at`, siteID)
	if err != nil {
		return nil, fmt.Errorf("ListShiftTemplates: %w", err)
	}
	return collect(rows, scanTemplateRows)
}

// ListActiveShiftTemplates returns every active template; used by the generator.
func ListActiveShiftTemplates(ctx context.Context, db DBTX) ([]model.ShiftTemplate, error) {
	rows, err := db.Query(ctx, `SELECT `+templateColumns+` FROM shift_templates WHERE active`)
	if err != nil {
		return nil, fmt.Errorf("ListActiveShiftTemplates: %w", err)
	}
	return collect(rows, scanTemplateRows)
}

// GetShiftTemplate retrieves a template by ID.
func GetShiftTemplate(ctx context.Context, db DBTX, id string) (*model.ShiftTemplate, error) {
	t, err := scanTemplate(db.QueryRow(ctx, `SELECT `+templateColumns+` FROM shift_templates WHERE id = $1`, id))
	if err != nil {
		return nil, mapNoRows(err)
	}
	return &t, nil
}

// CreateShiftTemplate inserts a template.
func CreateShiftTemplate(ctx context.Context, db DBTX, t model.ShiftTemplate) (*model.ShiftTemplate, error) {
	out, err := scanTemplate(db.QueryRow(ctx, `
		INSERT INTO shift_templates (site_id, guard_id, start_time, end_time, weekdays, valid_from, valid_until, active)
		VALUES ($1, $2, $3::time, $4::time, $5, $6::date, $7::date, $8)
		RETURNING `+templateColumns,
		t.SiteID, t.GuardID, t.StartTime, t.EndTime, t.Weekdays, t.ValidFrom, t.ValidUntil, t.Active))
	if err != nil {
		return nil, fmt.Errorf("CreateShiftTemplate: %w", err)
	}
	return &out, nil
}

// UpdateShiftTemplate replaces the editable fields of a template.
func UpdateShiftTemplate(ctx context.Context, db DBTX, t model.ShiftTemplate) (*model.ShiftTemplate, error) {
	out, err := scanTemplate(db.QueryRow(ctx, `
		UPDATE shift_templates
		SET guard_id = $2, start_time = $3::time, end_time = $4::time, weekdays = $5,
		    valid_from = $6::date, valid_until = $7::date, active = $8, updated_at = now()
		WHERE id = $1
		RETURNING `+templateColumns,
		t.ID, t.GuardID, t.StartTime, t.EndTime, t.Weekdays, t.ValidFrom, t.ValidUntil, t.Active))
	if err != nil {
		return nil, mapNoRows(err)
	}
	return &out, nil
}

// DeleteShiftTemplate removes a template. Generated shifts keep existing with
// template_id set to NULL; callers remove the future ones first.
func DeleteShiftTemplate(ctx context.Context, db DBTX, id string) error {
	tag, err := db.Exec(ctx, `DELETE FROM shift_templates WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("DeleteShiftTemplate: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ── Concrete shifts ──────────────────────────────────────────────────────────

const shiftColumns = `id, guard_id, site_id, template_id, start_time, end_time, created_at`

func scanShift(row pgx.Row) (model.Shift, error) {
	var s model.Shift
	err := row.Scan(&s.ID, &s.GuardID, &s.SiteID, &s.TemplateID, &s.StartTime, &s.EndTime, &s.CreatedAt)
	return s, err
}

func scanShiftRows(rows pgx.Rows) (model.Shift, error) { return scanShift(rows) }

// ListShifts returns the shifts of a site starting in [from, to), optionally
// for one guard.
func ListShifts(ctx context.Context, db DBTX, siteID, guardID string, from, to time.Time) ([]model.Shift, error) {
	rows, err := db.Query(ctx, `
		SELECT `+shiftColumns+` FROM shifts
		WHERE site_id = $1 AND start_time >= $2 AND start_time < $3
		  AND ($4 = '' OR guard_id::text = $4)
		ORDER BY start_time`, siteID, from, to, guardID)
	if err != nil {
		return nil, fmt.Errorf("ListShifts: %w", err)
	}
	return collect(rows, scanShiftRows)
}

// GetShift retrieves a shift by ID.
func GetShift(ctx context.Context, db DBTX, id string) (*model.Shift, error) {
	s, err := scanShift(db.QueryRow(ctx, `SELECT `+shiftColumns+` FROM shifts WHERE id = $1`, id))
	if err != nil {
		return nil, mapNoRows(err)
	}
	return &s, nil
}

// CreateShift inserts a one-off shift (no template).
func CreateShift(ctx context.Context, db DBTX, guardID, siteID string, start, end time.Time) (*model.Shift, error) {
	s, err := scanShift(db.QueryRow(ctx, `
		INSERT INTO shifts (guard_id, site_id, start_time, end_time)
		VALUES ($1, $2, $3, $4)
		RETURNING `+shiftColumns, guardID, siteID, start, end))
	if err != nil {
		return nil, fmt.Errorf("CreateShift: %w", err)
	}
	return &s, nil
}

// InsertGeneratedShift inserts a shift for a template occurrence; it is a
// no-op when that occurrence already exists. Returns whether a row was added.
func InsertGeneratedShift(ctx context.Context, db DBTX, templateID, guardID, siteID string, start, end time.Time) (bool, error) {
	tag, err := db.Exec(ctx, `
		INSERT INTO shifts (template_id, guard_id, site_id, start_time, end_time)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (template_id, start_time) WHERE template_id IS NOT NULL DO NOTHING`,
		templateID, guardID, siteID, start, end)
	if err != nil {
		return false, fmt.Errorf("InsertGeneratedShift: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// DeleteFutureTemplateShifts removes a template's shifts that start after
// `after` and have no attendance yet, so the template can be regenerated.
func DeleteFutureTemplateShifts(ctx context.Context, db DBTX, templateID string, after time.Time) (int64, error) {
	tag, err := db.Exec(ctx, `
		DELETE FROM shifts s
		WHERE s.template_id = $1 AND s.start_time > $2
		  AND NOT EXISTS (SELECT 1 FROM attendance_records a WHERE a.shift_id = s.id)`,
		templateID, after)
	if err != nil {
		return 0, fmt.Errorf("DeleteFutureTemplateShifts: %w", err)
	}
	return tag.RowsAffected(), nil
}

// UpdateShift changes guard and times of a single shift. The shift is
// detached from its template so regeneration will not overwrite the edit.
func UpdateShift(ctx context.Context, db DBTX, id, guardID string, start, end time.Time) (*model.Shift, error) {
	s, err := scanShift(db.QueryRow(ctx, `
		UPDATE shifts SET guard_id = $2, start_time = $3, end_time = $4, template_id = NULL
		WHERE id = $1
		RETURNING `+shiftColumns, id, guardID, start, end))
	if err != nil {
		return nil, mapNoRows(err)
	}
	return &s, nil
}

// DeleteShift removes a shift that has no attendance record.
func DeleteShift(ctx context.Context, db DBTX, id string) error {
	tag, err := db.Exec(ctx, `
		DELETE FROM shifts s WHERE s.id = $1
		  AND NOT EXISTS (SELECT 1 FROM attendance_records a WHERE a.shift_id = s.id)`, id)
	if err != nil {
		return fmt.Errorf("DeleteShift: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// FindShiftForClockIn returns the guard's shift at a site whose clock-in
// window contains t: [start - window, end). The latest-starting match wins.
func FindShiftForClockIn(ctx context.Context, db DBTX, guardID, siteID string, t time.Time, window time.Duration) (*model.Shift, error) {
	s, err := scanShift(db.QueryRow(ctx, `
		SELECT `+shiftColumns+` FROM shifts
		WHERE guard_id = $1 AND site_id = $2
		  AND start_time - $4::interval <= $3 AND $3 < end_time
		ORDER BY start_time DESC
		LIMIT 1`, guardID, siteID, t, window))
	if err != nil {
		return nil, mapNoRows(err)
	}
	return &s, nil
}

// CurrentOrNextShift returns the guard's shift at a site that has not ended
// yet (the running one, else the next one), or ErrNotFound.
func CurrentOrNextShift(ctx context.Context, db DBTX, guardID, siteID string, now time.Time) (*model.Shift, error) {
	s, err := scanShift(db.QueryRow(ctx, `
		SELECT `+shiftColumns+` FROM shifts
		WHERE guard_id = $1 AND site_id = $2 AND end_time > $3
		ORDER BY start_time
		LIMIT 1`, guardID, siteID, now))
	if err != nil {
		return nil, mapNoRows(err)
	}
	return &s, nil
}
