package kcadmin

import (
	"context"
	"crypto/rand"
	"math/big"
	"slices"
	"strings"
	"time"
	"unicode"
)

// Realm roles the user-management API knows about. Only these appear in
// views and are added/removed by SetRealmRoles; other mappings (e.g.
// default-roles-<realm>, mfa-required composites) are left alone.
var (
	WebRoles   = []string{"admin", "supervisor", "hr", "client-readonly", "platform-admin"}
	RoleDevice = "device"
	// StaffRoles must enrol OTP (CONFIGURE_TOTP), like the mfa-required
	// composite of keycloak-config.sh.
	StaffRoles = []string{"admin", "supervisor", "hr", "platform-admin"}
)

// SitesAttribute is the multi-valued user attribute holding allowed site
// IDs; the web client maps it into the access-token claim "sites" (D-035).
const SitesAttribute = "sites"

// View is a user as the API answers it.
type View struct {
	ID              string    `json:"id"`
	Username        string    `json:"username"`
	Email           string    `json:"email"`
	FirstName       string    `json:"first_name"`
	LastName        string    `json:"last_name"`
	Enabled         bool      `json:"enabled"`
	Roles           []string  `json:"roles"`
	Sites           []string  `json:"sites"`
	OTPConfigured   bool      `json:"otp_configured"`
	RequiredActions []string  `json:"required_actions"`
	CreatedAt       time.Time `json:"created_at"`
}

// ViewOf builds the API view of a user with its (managed) roles.
func ViewOf(u *User, roles []string) View {
	v := View{
		ID: u.ID, Username: u.Username, Email: u.Email, FirstName: u.FirstName, LastName: u.LastName,
		Enabled: u.Enabled, OTPConfigured: u.Totp, Roles: []string{}, Sites: SitesOf(u),
		RequiredActions: u.RequiredActions,
	}
	if v.RequiredActions == nil {
		v.RequiredActions = []string{}
	}
	for _, r := range roles {
		if slices.Contains(WebRoles, r) || r == RoleDevice {
			v.Roles = append(v.Roles, r)
		}
	}
	slices.Sort(v.Roles)
	if u.CreatedTimestamp > 0 {
		v.CreatedAt = time.UnixMilli(u.CreatedTimestamp).UTC()
	}
	return v
}

// SitesOf returns the user's allowed site IDs (never nil).
func SitesOf(u *User) []string {
	out := []string{}
	for _, s := range u.Attributes[SitesAttribute] {
		for _, p := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' }) {
			if !slices.Contains(out, p) {
				out = append(out, p)
			}
		}
	}
	return out
}

// SetSites stores the allowed sites (empty = whole realm).
func SetSites(u *User, sites []string) {
	if u.Attributes == nil {
		u.Attributes = map[string][]string{}
	}
	if len(sites) == 0 {
		delete(u.Attributes, SitesAttribute)
		return
	}
	u.Attributes[SitesAttribute] = append([]string{}, sites...)
}

// RoleIndex maps user ID -> the managed realm roles they hold (one call per
// role instead of one per user).
func (c *Client) RoleIndex(ctx context.Context, realm string) (map[string][]string, error) {
	idx := map[string][]string{}
	for _, role := range append(append([]string{}, WebRoles...), RoleDevice) {
		users, err := c.RoleMembers(ctx, realm, role)
		if err != nil {
			return nil, err
		}
		for _, u := range users {
			idx[u.ID] = append(idx[u.ID], role)
		}
	}
	return idx, nil
}

// maxUsers bounds ListAllUsers (a client realm has tens to hundreds).
const maxUsers = 10_000

// ListAllUsers returns every user of the realm (service-account users of
// confidential clients excluded).
func (c *Client) ListAllUsers(ctx context.Context, realm string) ([]User, error) {
	const page = 500
	var all []User
	for first := 0; first < maxUsers; first += page {
		users, err := c.ListUsers(ctx, realm, "", first, page)
		if err != nil {
			return nil, err
		}
		for _, u := range users {
			if !strings.HasPrefix(u.Username, "service-account-") {
				all = append(all, u)
			}
		}
		if len(users) < page {
			break
		}
	}
	return all, nil
}

// SetRealmRoles makes the user's managed roles exactly want (roles outside
// managed are not touched).
func (c *Client) SetRealmRoles(ctx context.Context, realm, userID string, want, managed []string) error {
	current, err := c.RealmRoles(ctx, realm, userID)
	if err != nil {
		return err
	}
	have := map[string]bool{}
	var remove []Role
	for _, r := range current {
		if !slices.Contains(managed, r.Name) {
			continue
		}
		have[r.Name] = true
		if !slices.Contains(want, r.Name) {
			remove = append(remove, r)
		}
	}
	var add []Role
	for _, name := range want {
		if have[name] {
			continue
		}
		r, err := c.Role(ctx, realm, name)
		if err != nil {
			return err
		}
		add = append(add, *r)
	}
	if err := c.RemoveRealmRoles(ctx, realm, userID, remove); err != nil {
		return err
	}
	return c.AddRealmRoles(ctx, realm, userID, add)
}

// ResetOTP removes every OTP credential; the user enrols again at the next
// login (CONFIGURE_TOTP is added for staff by the caller).
func (c *Client) ResetOTP(ctx context.Context, realm, userID string) (int, error) {
	creds, err := c.Credentials(ctx, realm, userID)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, cr := range creds {
		if cr.Type != "otp" {
			continue
		}
		if err := c.DeleteCredential(ctx, realm, userID, cr.ID); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

const (
	pwUpper  = "ABCDEFGHJKLMNPQRSTUVWXYZ"
	pwLower  = "abcdefghijkmnopqrstuvwxyz"
	pwDigits = "23456789"
	pwSymbol = "!#%+-=?@^_"
)

// GeneratePassword returns a random password of n (>= 12) characters with
// upper- and lower-case letters, digits and symbols (realm policy: length
// >= 10, an upper-case letter and a digit). Look-alike characters are left out.
func GeneratePassword(n int) string {
	if n < 12 {
		n = 12
	}
	all := pwUpper + pwLower + pwDigits + pwSymbol
	pick := func(set string) byte {
		i, err := rand.Int(rand.Reader, big.NewInt(int64(len(set))))
		if err != nil {
			panic(err) // crypto/rand never fails on supported platforms
		}
		return set[i.Int64()]
	}
	b := []byte{pick(pwUpper), pick(pwLower), pick(pwDigits), pick(pwSymbol)}
	for len(b) < n {
		b = append(b, pick(all))
	}
	for i := len(b) - 1; i > 0; i-- { // Fisher-Yates
		j, _ := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		b[i], b[j.Int64()] = b[j.Int64()], b[i]
	}
	return string(b)
}

// Slug turns a device name into a username part: lower-case ASCII letters,
// digits and dashes, at most 40 characters.
func Slug(name string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		case unicode.IsSpace(r) || r == '-' || r == '_' || r == '.' || r == '/':
			if !dash && b.Len() > 0 {
				b.WriteByte('-')
				dash = true
			}
		}
		if b.Len() >= 40 {
			break
		}
	}
	return strings.Trim(b.String(), "-")
}
