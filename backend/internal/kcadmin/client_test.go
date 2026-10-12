package kcadmin_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/securepatrol/backend/internal/kcadmin"
	"github.com/securepatrol/backend/internal/kcadmin/kcfake"
)

const realm = "client1"

func setup(t *testing.T) (*kcfake.Server, *kcadmin.Client) {
	t.Helper()
	f := kcfake.New("securepatrol-admin-api", map[string]string{realm: "s3cret", "other": "x"})
	t.Cleanup(f.Close)
	c := kcadmin.New(kcadmin.Options{BaseURL: f.URL, ClientID: "securepatrol-admin-api",
		Secrets: map[string]string{realm: "s3cret"}, Timeout: 2 * time.Second})
	return f, c
}

func TestUserLifecycle(t *testing.T) {
	f, c := setup(t)
	ctx := context.Background()
	u := kcadmin.User{Username: "budi", Email: "budi@example.com", FirstName: "Budi", Enabled: true}
	kcadmin.SetSites(&u, []string{"s1", "s2"})
	id, err := c.CreateUser(ctx, realm, u)
	if err != nil || id == "" {
		t.Fatalf("create: %q %v", id, err)
	}
	if _, err := c.CreateUser(ctx, realm, kcadmin.User{Username: "budi"}); !errors.Is(err, kcadmin.ErrUsername) {
		t.Fatalf("duplicate username: %v", err)
	}
	if _, err := c.CreateUser(ctx, realm, kcadmin.User{Username: "budi2", Email: "BUDI@example.com"}); !errors.Is(err, kcadmin.ErrEmail) {
		t.Fatalf("duplicate email: %v", err)
	}
	if err := c.SetRealmRoles(ctx, realm, id, []string{"admin", "hr"}, kcadmin.WebRoles); err != nil {
		t.Fatal(err)
	}
	if err := c.SetRealmRoles(ctx, realm, id, []string{"hr"}, kcadmin.WebRoles); err != nil {
		t.Fatal(err)
	}
	if got := f.Get(realm, id).Roles; !slices.Equal(got, []string{"hr"}) {
		t.Fatalf("roles = %v", got)
	}
	idx, err := c.RoleIndex(ctx, realm)
	if err != nil || !slices.Equal(idx[id], []string{"hr"}) {
		t.Fatalf("role index = %v %v", idx, err)
	}
	got, err := c.GetUser(ctx, realm, id)
	if err != nil || !slices.Equal(kcadmin.SitesOf(got), []string{"s1", "s2"}) {
		t.Fatalf("get: %+v %v", got, err)
	}
	got.Enabled = false
	kcadmin.SetSites(got, nil)
	if err := c.UpdateUser(ctx, realm, got); err != nil {
		t.Fatal(err)
	}
	if s := f.Get(realm, id); s.Enabled || len(s.Attributes["sites"]) != 0 || s.FirstName != "Budi" {
		t.Fatalf("after update: %+v", s)
	}
	if err := c.ResetPassword(ctx, realm, id, "short", true); err == nil {
		t.Fatal("policy violation accepted")
	} else if bad := (*kcadmin.BadRequestError)(nil); !errors.As(err, &bad) || !strings.Contains(bad.Message, "password") {
		t.Fatalf("policy error = %v", err)
	}
	if _, err := c.GetUser(ctx, realm, "missing"); !errors.Is(err, kcadmin.ErrNotFound) {
		t.Fatalf("missing user: %v", err)
	}
	all, err := c.ListAllUsers(ctx, realm)
	if err != nil || len(all) != 1 {
		t.Fatalf("list: %d %v", len(all), err)
	}
}

func TestOTPResetAndLogout(t *testing.T) {
	f, c := setup(t)
	ctx := context.Background()
	u := f.AddUser(realm, kcfake.User{Username: "sari", Enabled: true, Totp: true, Password: "x"})
	n, err := c.ResetOTP(ctx, realm, u.ID)
	if err != nil || n != 1 || f.Get(realm, u.ID).Totp {
		t.Fatalf("reset otp: %d %v", n, err)
	}
	if err := c.Logout(ctx, realm, u.ID); err != nil || f.Get(realm, u.ID).LoggedOut != 1 {
		t.Fatalf("logout: %v", err)
	}
}

func TestTokenCachingAndUnavailable(t *testing.T) {
	f, c := setup(t)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if _, err := c.ListUsers(ctx, realm, "", 0, 10); err != nil {
			t.Fatal(err)
		}
	}
	if f.TokenRequests != 1 {
		t.Fatalf("token requests = %d, want 1 (cached)", f.TokenRequests)
	}
	if _, err := c.ListUsers(ctx, "other", "", 0, 10); !errors.Is(err, kcadmin.ErrUnavailable) {
		t.Fatalf("realm without secret: %v", err)
	}
	if c.Configured("other") || !c.Configured(realm) {
		t.Fatal("Configured")
	}
	f.Down = true
	_, err := c.ListUsers(ctx, realm, "", 0, 10)
	if !errors.Is(err, kcadmin.ErrUnavailable) || strings.Contains(err.Error(), "s3cret") {
		t.Fatalf("down: %v", err)
	}
	f.Close()
	if _, err := c.ListUsers(ctx, realm, "", 0, 10); !errors.Is(err, kcadmin.ErrUnavailable) {
		t.Fatalf("closed: %v", err)
	}
	bad := kcadmin.New(kcadmin.Options{BaseURL: "http://127.0.0.1:1", ClientID: "x", Secrets: map[string]string{realm: "wrong"}})
	if _, err := bad.ListUsers(ctx, realm, "", 0, 1); !errors.Is(err, kcadmin.ErrUnavailable) || strings.Contains(err.Error(), "wrong") {
		t.Fatalf("unreachable: %v", err)
	}
}

func TestGeneratePasswordAndSlug(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		p := kcadmin.GeneratePassword(16)
		if len(p) != 16 || seen[p] {
			t.Fatalf("password %q", p)
		}
		seen[p] = true
		var up, dig bool
		for _, r := range p {
			up = up || unicode.IsUpper(r)
			dig = dig || unicode.IsDigit(r)
		}
		if !up || !dig {
			t.Fatalf("password %q misses a class", p)
		}
	}
	for in, want := range map[string]string{
		"Pos Jaga Utara": "pos-jaga-utara", "  Tablet #2 / Lobby ": "tablet-2-lobby", "ÄÖÜ": "", "a__b": "a-b",
	} {
		if got := kcadmin.Slug(in); got != want {
			t.Errorf("Slug(%q) = %q, want %q", in, got, want)
		}
	}
}
