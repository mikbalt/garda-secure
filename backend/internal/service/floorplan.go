package service

import (
	"math"
	"slices"

	"github.com/google/uuid"

	"github.com/securepatrol/backend/internal/model"
)

// CheckCheckpointPlans validates the floor plan positions of a route's
// checkpoints (F3): floorplan_id, plan_x and plan_y are set together, the
// coordinates are relative (0..1). It returns the distinct floor plan IDs
// referenced, for the caller to check they belong to the route's site.
func CheckCheckpointPlans(cps []model.Checkpoint) ([]string, error) {
	var ids []string
	for i := range cps {
		cp := &cps[i]
		if cp.FloorplanID != nil && *cp.FloorplanID == "" {
			cp.FloorplanID = nil
		}
		set := 0
		for _, v := range []bool{cp.FloorplanID != nil, cp.PlanX != nil, cp.PlanY != nil} {
			if v {
				set++
			}
		}
		if set == 0 {
			continue
		}
		if set != 3 {
			return nil, BadRequest("invalid_plan_position",
				"checkpoint %q: floorplan_id, plan_x and plan_y must be set together", cp.Name)
		}
		u, err := uuid.Parse(*cp.FloorplanID)
		if err != nil {
			return nil, BadRequest("invalid_plan_position", "checkpoint %q: floorplan_id must be a UUID", cp.Name)
		}
		id := u.String()
		cp.FloorplanID = &id
		for _, v := range []float64{*cp.PlanX, *cp.PlanY} {
			if math.IsNaN(v) || v < 0 || v > 1 {
				return nil, BadRequest("invalid_plan_position",
					"checkpoint %q: plan_x and plan_y must be between 0 and 1", cp.Name)
			}
		}
		if !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	return ids, nil
}
