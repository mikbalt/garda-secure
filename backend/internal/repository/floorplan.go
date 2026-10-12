package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/securepatrol/backend/internal/model"
)

const floorplanColumns = `id, site_id, name, object_key, content_type, width, height, size_bytes, created_by, created_at`

func scanFloorplan(r pgx.Row) (model.Floorplan, error) {
	var f model.Floorplan
	err := r.Scan(&f.ID, &f.SiteID, &f.Name, &f.ObjectKey, &f.ContentType, &f.Width, &f.Height,
		&f.SizeBytes, &f.CreatedBy, &f.CreatedAt)
	return f, err
}

// CreateFloorplan stores floor plan metadata (the image is already stored).
func CreateFloorplan(ctx context.Context, db DBTX, f model.Floorplan) (*model.Floorplan, error) {
	out, err := scanFloorplan(db.QueryRow(ctx, `
		INSERT INTO site_floorplans (id, site_id, name, object_key, content_type, width, height, size_bytes, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING `+floorplanColumns,
		f.ID, f.SiteID, f.Name, f.ObjectKey, f.ContentType, f.Width, f.Height, f.SizeBytes, f.CreatedBy))
	if err != nil {
		return nil, fmt.Errorf("CreateFloorplan: %w", err)
	}
	return &out, nil
}

// ListFloorplans returns the floor plans of a site, oldest first.
func ListFloorplans(ctx context.Context, db DBTX, siteID string) ([]model.Floorplan, error) {
	rows, err := db.Query(ctx, `SELECT `+floorplanColumns+` FROM site_floorplans WHERE site_id = $1 ORDER BY created_at, id`, siteID)
	if err != nil {
		return nil, fmt.Errorf("ListFloorplans: %w", err)
	}
	return collect(rows, func(r pgx.Rows) (model.Floorplan, error) { return scanFloorplan(r) })
}

// GetFloorplan retrieves a floor plan by ID.
func GetFloorplan(ctx context.Context, db DBTX, id string) (*model.Floorplan, error) {
	f, err := scanFloorplan(db.QueryRow(ctx, `SELECT `+floorplanColumns+` FROM site_floorplans WHERE id = $1`, id))
	if err != nil {
		return nil, mapNoRows(err)
	}
	return &f, nil
}

// MissingFloorplans returns the IDs among ids that are not floor plans of
// the site.
func MissingFloorplans(ctx context.Context, db DBTX, siteID string, ids []string) ([]string, error) {
	rows, err := db.Query(ctx, `
		SELECT x FROM unnest($2::text[]) AS x
		WHERE NOT EXISTS (SELECT 1 FROM site_floorplans f WHERE f.site_id = $1 AND f.id::text = x)`, siteID, ids)
	if err != nil {
		return nil, fmt.Errorf("MissingFloorplans: %w", err)
	}
	return collect(rows, func(r pgx.Rows) (string, error) {
		var s string
		err := r.Scan(&s)
		return s, err
	})
}

// FloorplanInUse reports whether a checkpoint of any route of the site is
// placed on the floor plan.
func FloorplanInUse(ctx context.Context, db DBTX, siteID, id string) (bool, error) {
	var used bool
	err := db.QueryRow(ctx, `
		SELECT EXISTS (
		  SELECT 1 FROM routes r, jsonb_array_elements(r.checkpoints) cp
		  WHERE r.site_id = $1 AND cp->>'floorplan_id' = $2)`, siteID, id).Scan(&used)
	if err != nil {
		return false, fmt.Errorf("FloorplanInUse: %w", err)
	}
	return used, nil
}

// DeleteFloorplan removes floor plan metadata.
func DeleteFloorplan(ctx context.Context, db DBTX, id string) error {
	tag, err := db.Exec(ctx, `DELETE FROM site_floorplans WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("DeleteFloorplan: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// GuardNames returns guard names by ID (one query for many guards).
func GuardNames(ctx context.Context, db DBTX, ids []string) (map[string]string, error) {
	rows, err := db.Query(ctx, `SELECT id, name FROM guards WHERE id = ANY($1::uuid[])`, ids)
	if err != nil {
		return nil, fmt.Errorf("GuardNames: %w", err)
	}
	out := map[string]string{}
	_, err = collect(rows, func(r pgx.Rows) (struct{}, error) {
		var id, name string
		err := r.Scan(&id, &name)
		out[id] = name
		return struct{}{}, err
	})
	return out, err
}
