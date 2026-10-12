package service

import (
	"math"
	"testing"

	"github.com/securepatrol/backend/internal/model"
)

func TestCheckCheckpointPlans(t *testing.T) {
	f := func(v float64) *float64 { return &v }
	s := func(v string) *string { return &v }
	fp := "6F9619FF-8B86-D011-B42D-00C04FC964FF"
	cps := []model.Checkpoint{
		{Name: "A", FloorplanID: s(fp), PlanX: f(0), PlanY: f(1)},
		{Name: "B", FloorplanID: s(fp), PlanX: f(0.5), PlanY: f(0.25)},
		{Name: "C"},
		{Name: "D", FloorplanID: s("")},
	}
	ids, err := CheckCheckpointPlans(cps)
	if err != nil || len(ids) != 1 || ids[0] != "6f9619ff-8b86-d011-b42d-00c04fc964ff" {
		t.Fatalf("ids = %v, %v", ids, err)
	}
	if cps[3].FloorplanID != nil {
		t.Fatal("empty floorplan_id should be cleared")
	}
	for _, bad := range []model.Checkpoint{
		{Name: "x", FloorplanID: s(fp), PlanX: f(0.5)},
		{Name: "x", PlanX: f(0.5), PlanY: f(0.5)},
		{Name: "x", FloorplanID: s("nope"), PlanX: f(0.5), PlanY: f(0.5)},
		{Name: "x", FloorplanID: s(fp), PlanX: f(1.01), PlanY: f(0.5)},
		{Name: "x", FloorplanID: s(fp), PlanX: f(math.NaN()), PlanY: f(0.5)},
	} {
		if _, err := CheckCheckpointPlans([]model.Checkpoint{bad}); err == nil {
			t.Errorf("accepted %+v", bad)
		}
	}
}
