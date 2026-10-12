package middleware

import (
	"slices"
	"testing"
)

func TestExtractSites(t *testing.T) {
	a, b := "11111111-1111-1111-1111-111111111111", "22222222-2222-2222-2222-222222222222"
	for _, c := range []struct {
		claim      any
		want       []string
		restricted bool
	}{
		{nil, nil, false},
		{[]interface{}{}, nil, false},
		{"", nil, false},
		{[]interface{}{a, b, a}, []string{a, b}, true},
		{a + "," + b, []string{a, b}, true},
		{[]interface{}{"11111111-1111-1111-1111-111111111111 , 22222222-2222-2222-2222-222222222222"}, []string{a, b}, true},
		{[]interface{}{"not-a-uuid"}, []string{}, true}, // fail closed
	} {
		got, restricted := extractSites(c.claim)
		if restricted != c.restricted || !slices.Equal(got, c.want) {
			t.Errorf("%v: got %v %v, want %v %v", c.claim, got, restricted, c.want, c.restricted)
		}
	}
	p := &Principal{Sites: []string{a}, SitesRestricted: true}
	if !p.CanSeeSite("11111111-1111-1111-1111-111111111111") || p.CanSeeSite(b) {
		t.Fatal("CanSeeSite")
	}
	if !(&Principal{}).CanSeeSite(b) {
		t.Fatal("unrestricted principal sees every site")
	}
}

func TestPathMatches(t *testing.T) {
	for _, c := range []struct {
		pattern, path string
		want          bool
	}{
		{"/api/v1/media", "/api/v1/media", true},
		{"/api/v1/media", "/api/v1/media/x", false},
		{"/api/v1/sites/*/floorplans", "/api/v1/sites/abc/floorplans", true},
		{"/api/v1/sites/*/floorplans", "/api/v1/sites//floorplans", false},
		{"/api/v1/sites/*/floorplans", "/api/v1/sites/abc/floorplans/x", false},
		{"/api/v1/sites/*/floorplans", "/api/v1/sites/abc/routes", false},
	} {
		if got := pathMatches(c.pattern, c.path); got != c.want {
			t.Errorf("%s ~ %s = %v", c.pattern, c.path, got)
		}
	}
}
