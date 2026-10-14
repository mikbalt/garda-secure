package handler

import (
	"context"
	"errors"
	"net/mail"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/securepatrol/backend/internal/kcadmin"
	"github.com/securepatrol/backend/internal/middleware"
	"github.com/securepatrol/backend/internal/model"
	"github.com/securepatrol/backend/internal/repository"
	"github.com/securepatrol/backend/internal/service"
)

// User and device-account management through the Keycloak Admin REST API
// (audit F1, NFR-09): realm admins manage the people and tablet accounts of
// their own realm without the Keycloak console. Every change is audited with
// before/after state (never passwords).

var kcClients sync.Map // *config.Config -> *kcadmin.Client (one per API configuration)

// kc returns the Keycloak admin client for this API's configuration.
func (h *Handler) kc() *kcadmin.Client {
	if v, ok := kcClients.Load(h.Cfg); ok {
		return v.(*kcadmin.Client)
	}
	k := h.Cfg.KeycloakAdmin
	c := kcadmin.New(kcadmin.Options{BaseURL: k.InternalURL, ClientID: k.ClientID, Secrets: k.Secrets,
		SecretsFile: k.SecretsFile, Timeout: k.Timeout})
	v, _ := kcClients.LoadOrStore(h.Cfg, c)
	return v.(*kcadmin.Client)
}

// kcError maps Keycloak client errors to API errors.
func (h *Handler) kcError(c *fiber.Ctx, err error) error {
	var bad *kcadmin.BadRequestError
	switch {
	case errors.Is(err, kcadmin.ErrUnavailable):
		h.Log.Error().Err(err).Str("request_id", requestID(c)).Msg("keycloak admin call failed")
		return service.Unavailable("kc_unavailable",
			"user management is unavailable: Keycloak is not reachable or not configured for this realm")
	case errors.Is(err, kcadmin.ErrNotFound):
		return service.NotFound("user_not_found", "user not found")
	case errors.Is(err, kcadmin.ErrUsername):
		return service.Conflict("username_taken", "this username is already used").WithField("username")
	case errors.Is(err, kcadmin.ErrEmail):
		return service.Conflict("email_taken", "this e-mail address is already used").WithField("email")
	case errors.As(err, &bad):
		if strings.Contains(strings.ToLower(bad.Message), "password") {
			return service.BadRequest("password_policy", "the password does not meet the realm's password policy (%s)", bad.Message).WithField("temporary_password")
		}
		return service.BadRequest("invalid_user", "Keycloak rejected the request: %s", bad.Message)
	}
	return err
}

func (h *Handler) failKC(c *fiber.Ctx, err error) error { return h.fail(c, h.kcError(c, err)) }

// canGrantPlatform reports whether the caller may grant/revoke platform-admin.
func (h *Handler) canGrantPlatform(p *middleware.Principal) bool {
	return p.HasRole(middleware.RolePlatformAdmin) &&
		(len(h.Cfg.PlatformAdminRealms) == 0 || slices.Contains(h.Cfg.PlatformAdminRealms, p.Realm))
}

// checkRoles validates requested realm roles (allow-list; platform-admin
// only by a platform admin; device never — tablets use /device-accounts).
func (h *Handler) checkRoles(p *middleware.Principal, roles []string) ([]string, error) {
	out := []string{}
	for _, r := range roles {
		r = strings.TrimSpace(r)
		switch {
		case r == "":
			continue
		case r == middleware.RolePlatformAdmin && !h.canGrantPlatform(p):
			return nil, service.Forbidden("role_not_allowed", "only a platform admin can grant or revoke platform-admin").WithField("roles")
		case !slices.Contains(kcadmin.WebRoles, r):
			return nil, service.Forbidden("role_not_allowed", "role %q cannot be assigned here", r).WithField("roles")
		}
		if !slices.Contains(out, r) {
			out = append(out, r)
		}
	}
	slices.Sort(out)
	return out, nil
}

// checkSites validates allowed sites: UUIDs of sites the caller may see. A
// site-scoped caller may only create users limited to (some of) their sites.
func (h *Handler) checkSites(c *fiber.Ctx, sites []string) ([]string, error) {
	out := []string{}
	for _, s := range sites {
		u, err := uuid.Parse(strings.TrimSpace(s))
		if err != nil {
			return nil, service.BadRequest("invalid_site", "site %q is not a UUID", s).WithField("sites")
		}
		id := u.String()
		if _, err := h.siteFor(c, id); err != nil {
			return nil, service.BadRequest("invalid_site", "site %s is not a site of your organisation", id).WithField("sites")
		}
		if !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	if middleware.GetPrincipal(c).SitesRestricted && len(out) == 0 {
		return nil, service.Forbidden("sites_required", "you can only manage users limited to your sites").WithField("sites")
	}
	slices.Sort(out)
	return out, nil
}

// visibleTo reports whether a site-scoped caller may see a user: only users
// limited to a non-empty subset of the caller's sites.
func visibleTo(p *middleware.Principal, sites []string) bool {
	if !p.SitesRestricted {
		return true
	}
	if len(sites) == 0 {
		return false
	}
	for _, s := range sites {
		if !p.CanSeeSite(s) {
			return false
		}
	}
	return true
}

// userFor loads a (non-device) user the caller may manage.
func (h *Handler) userFor(c *fiber.Ctx, id string) (*kcadmin.User, kcadmin.View, error) {
	p := middleware.GetPrincipal(c)
	if _, err := uuid.Parse(id); err != nil {
		return nil, kcadmin.View{}, service.NotFound("user_not_found", "user not found")
	}
	u, err := h.kc().GetUser(c.Context(), p.Realm, id)
	if err != nil {
		return nil, kcadmin.View{}, h.kcError(c, err)
	}
	roles, err := h.kc().RealmRoles(c.Context(), p.Realm, id)
	if err != nil {
		return nil, kcadmin.View{}, h.kcError(c, err)
	}
	names := make([]string, len(roles))
	for i, r := range roles {
		names[i] = r.Name
	}
	v := kcadmin.ViewOf(u, names)
	if slices.Contains(v.Roles, kcadmin.RoleDevice) || strings.HasPrefix(u.Username, "service-account-") || !visibleTo(p, v.Sites) {
		return nil, v, service.NotFound("user_not_found", "user not found")
	}
	if slices.Contains(v.Roles, middleware.RolePlatformAdmin) && !h.canGrantPlatform(p) {
		return nil, v, service.Forbidden("role_not_allowed", "only a platform admin can manage a platform admin")
	}
	return u, v, nil
}

func requiredActionsFor(roles []string, needPassword bool) []string {
	acts := []string{}
	if needPassword {
		acts = append(acts, "UPDATE_PASSWORD")
	}
	for _, r := range roles {
		if slices.Contains(kcadmin.StaffRoles, r) {
			return append(acts, "CONFIGURE_TOTP")
		}
	}
	return acts
}

// GET /users?search=&role=&enabled=&first=&max= — people of the caller's realm
// (device accounts are under /device-accounts), sorted by username.
func (h *Handler) listUsers(c *fiber.Ctx) error {
	p := middleware.GetPrincipal(c)
	first, err := strconv.Atoi(c.Query("first", "0"))
	if err != nil || first < 0 {
		return h.fail(c, service.BadRequest("invalid_first", "first must be a non-negative number"))
	}
	max, err := strconv.Atoi(c.Query("max", "100"))
	if err != nil || max < 1 || max > 500 {
		return h.fail(c, service.BadRequest("invalid_max", "max must be between 1 and 500"))
	}
	var enabled *bool
	if v := c.Query("enabled"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return h.fail(c, service.BadRequest("invalid_enabled", "enabled must be true or false"))
		}
		enabled = &b
	}
	role, search := c.Query("role"), strings.ToLower(strings.TrimSpace(c.Query("search")))
	users, err := h.kc().ListAllUsers(c.Context(), p.Realm)
	if err != nil {
		return h.failKC(c, err)
	}
	idx, err := h.kc().RoleIndex(c.Context(), p.Realm)
	if err != nil {
		return h.failKC(c, err)
	}
	out := []kcadmin.View{}
	for i := range users {
		v := kcadmin.ViewOf(&users[i], idx[users[i].ID])
		switch {
		case slices.Contains(v.Roles, kcadmin.RoleDevice), !visibleTo(p, v.Sites),
			role != "" && !slices.Contains(v.Roles, role),
			enabled != nil && v.Enabled != *enabled,
			search != "" && !strings.Contains(strings.ToLower(v.Username+" "+v.Email+" "+v.FirstName+" "+v.LastName), search):
			continue
		}
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Username < out[j].Username })
	total := len(out)
	if first > total {
		first = total
	}
	end := min(first+max, total)
	setPageHeaders(c, total, first, end-first)
	return c.JSON(out[first:end])
}

// GET /users/:id
func (h *Handler) getUser(c *fiber.Ctx) error {
	_, v, err := h.userFor(c, c.Params("id"))
	if err != nil {
		return h.fail(c, err)
	}
	return c.JSON(v)
}

var usernameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._@-]{2,63}$`)

func checkEmail(email string, required bool) (string, error) {
	email = strings.TrimSpace(email)
	if email == "" {
		if required {
			return "", service.BadRequest("email_required", "an e-mail address is required to send an invitation").WithField("email")
		}
		return "", nil
	}
	a, err := mail.ParseAddress(email)
	if err != nil || a.Address != email || len(email) > 254 {
		return "", service.BadRequest("invalid_email", "invalid e-mail address").WithField("email")
	}
	return strings.ToLower(email), nil
}

// inviteLifespan is how long an invitation / password e-mail link is valid.
const inviteLifespan = 72 * time.Hour

// POST /users {username, email, first_name, last_name, roles[], sites[],
// send_invite, temporary_password?}
func (h *Handler) createUser(c *fiber.Ctx) error {
	p := middleware.GetPrincipal(c)
	var b struct {
		Username          string   `json:"username"`
		Email             string   `json:"email"`
		FirstName         string   `json:"first_name"`
		LastName          string   `json:"last_name"`
		Roles             []string `json:"roles"`
		Sites             []string `json:"sites"`
		SendInvite        bool     `json:"send_invite"`
		TemporaryPassword string   `json:"temporary_password"`
	}
	if err := parse(c, &b); err != nil {
		return h.fail(c, err)
	}
	b.Username = strings.ToLower(strings.TrimSpace(b.Username))
	if !usernameRe.MatchString(b.Username) || strings.HasPrefix(b.Username, "device-") || strings.HasPrefix(b.Username, "service-account-") {
		return h.fail(c, service.BadRequest("invalid_username",
			"username: 3-64 characters a-z, 0-9 and . _ @ -, not starting with device- or service-account-").WithField("username"))
	}
	email, err := checkEmail(b.Email, b.SendInvite)
	if err != nil {
		return h.fail(c, err)
	}
	b.FirstName, b.LastName = strings.TrimSpace(b.FirstName), strings.TrimSpace(b.LastName)
	if err := service.CheckLengths(service.S("first_name", b.FirstName, service.MaxName),
		service.S("last_name", b.LastName, service.MaxName), service.S("temporary_password", b.TemporaryPassword, 128)); err != nil {
		return h.fail(c, err)
	}
	roles, err := h.checkRoles(p, b.Roles)
	if err != nil {
		return h.fail(c, err)
	}
	if len(roles) == 0 {
		return h.fail(c, service.BadRequest("roles_required", "choose at least one role").WithField("roles"))
	}
	sites, err := h.checkSites(c, b.Sites)
	if err != nil {
		return h.fail(c, err)
	}
	middleware.SetAuditBefore(c, nil)

	ctx := c.Context()
	u := kcadmin.User{
		Username: b.Username, Email: email, FirstName: b.FirstName, LastName: b.LastName, Enabled: true,
		RequiredActions: requiredActionsFor(roles, true),
	}
	kcadmin.SetSites(&u, sites)
	id, err := h.kc().CreateUser(ctx, p.Realm, u)
	if err != nil {
		return h.failKC(c, err)
	}
	u.ID = id
	rollback := func(cause error) error {
		if derr := h.kc().DeleteUser(context.Background(), p.Realm, id); derr != nil {
			h.Log.Error().Err(derr).Str("user_id", id).Msg("could not roll back a half-created user")
		}
		return h.failKC(c, cause)
	}
	if err := h.kc().SetRealmRoles(ctx, p.Realm, id, roles, kcadmin.WebRoles); err != nil {
		return rollback(err)
	}
	out := fiber.Map{"invite_sent": false}
	if b.SendInvite {
		if err := h.kc().ExecuteActionsEmail(ctx, p.Realm, id, u.RequiredActions, inviteLifespan); err != nil {
			// The user exists; the admin can set a temporary password instead.
			h.Log.Warn().Err(err).Str("user_id", id).Msg("invitation e-mail failed")
			out["invite_error"] = "invite_failed"
		} else {
			out["invite_sent"] = true
		}
	} else {
		pw := b.TemporaryPassword
		if pw == "" {
			pw = kcadmin.GeneratePassword(16)
		}
		if err := h.kc().ResetPassword(ctx, p.Realm, id, pw, true); err != nil {
			return rollback(err)
		}
		out["temporary_password"] = pw // shown once; never stored or audited
	}
	view := kcadmin.ViewOf(&u, roles)
	view.CreatedAt = service.Now()
	if fresh, err := h.kc().GetUser(ctx, p.Realm, id); err == nil {
		view = kcadmin.ViewOf(fresh, roles)
	}
	out["user"] = view
	middleware.SetAuditAfter(c, view)
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.Status(fiber.StatusCreated).JSON(out)
}

// PUT /users/:id {first_name?, last_name?, email?, enabled?, roles?, sites?}
// — omitted fields are kept.
func (h *Handler) updateUser(c *fiber.Ctx) error {
	p := middleware.GetPrincipal(c)
	u, before, err := h.userFor(c, c.Params("id"))
	if err != nil {
		return h.fail(c, err)
	}
	var b struct {
		FirstName *string   `json:"first_name"`
		LastName  *string   `json:"last_name"`
		Email     *string   `json:"email"`
		Enabled   *bool     `json:"enabled"`
		Roles     *[]string `json:"roles"`
		Sites     *[]string `json:"sites"`
	}
	if err := parse(c, &b); err != nil {
		return h.fail(c, err)
	}
	middleware.SetAuditBefore(c, before)
	self := u.ID == p.Subject
	roles := before.Roles
	if b.Roles != nil {
		if roles, err = h.checkRoles(p, *b.Roles); err != nil {
			return h.fail(c, err)
		}
		if len(roles) == 0 {
			return h.fail(c, service.BadRequest("roles_required", "choose at least one role").WithField("roles"))
		}
		if self && (!slices.Contains(roles, middleware.RoleAdmin) ||
			slices.Contains(before.Roles, middleware.RolePlatformAdmin) && !slices.Contains(roles, middleware.RolePlatformAdmin)) {
			return h.fail(c, service.Forbidden("cannot_modify_self", "you cannot remove your own admin roles"))
		}
	}
	if b.Enabled != nil {
		if self && !*b.Enabled {
			return h.fail(c, service.Forbidden("cannot_modify_self", "you cannot disable your own account"))
		}
		u.Enabled = *b.Enabled
	}
	if b.Sites != nil {
		sites, err := h.checkSites(c, *b.Sites)
		if err != nil {
			return h.fail(c, err)
		}
		if self && !slices.Equal(sites, before.Sites) {
			return h.fail(c, service.Forbidden("cannot_modify_self", "you cannot change your own sites"))
		}
		kcadmin.SetSites(u, sites)
	}
	if b.Email != nil {
		email, err := checkEmail(*b.Email, false)
		if err != nil {
			return h.fail(c, err)
		}
		if email != u.Email {
			u.EmailVerified = false
		}
		u.Email = email
	}
	for _, f := range []struct {
		v   *string
		dst *string
		n   string
	}{{b.FirstName, &u.FirstName, "first_name"}, {b.LastName, &u.LastName, "last_name"}} {
		if f.v != nil {
			v := strings.TrimSpace(*f.v)
			if err := service.CheckLengths(service.S(f.n, v, service.MaxName)); err != nil {
				return h.fail(c, err)
			}
			*f.dst = v
		}
	}
	// A new staff role must enrol OTP.
	for _, a := range requiredActionsFor(roles, false) {
		if !u.Totp && !slices.Contains(u.RequiredActions, a) {
			u.RequiredActions = append(u.RequiredActions, a)
		}
	}
	ctx := c.Context()
	if err := h.kc().UpdateUser(ctx, p.Realm, u); err != nil {
		return h.failKC(c, err)
	}
	if b.Roles != nil {
		if err := h.kc().SetRealmRoles(ctx, p.Realm, u.ID, roles, kcadmin.WebRoles); err != nil {
			return h.failKC(c, err)
		}
	}
	if before.Enabled && !u.Enabled {
		if err := h.kc().Logout(ctx, p.Realm, u.ID); err != nil {
			h.Log.Warn().Err(err).Str("user_id", u.ID).Msg("logout of disabled user failed")
		}
	}
	after := kcadmin.ViewOf(u, roles)
	h.updatePushAudience(c, p.Realm, after)
	middleware.SetAuditAfter(c, after)
	return c.JSON(after)
}

// updatePushAudience applies an admin's change to the user's push audience
// right away: a disabled user loses every push subscription, new roles or
// sites count from now on (not only from the user's next request).
func (h *Handler) updatePushAudience(c *fiber.Ctx, realm string, v kcadmin.View) {
	var err error
	if !v.Enabled {
		err = repository.DeletePushUser(c.Context(), h.DB, realm, v.ID)
	} else {
		var sites []string // nil = whole realm; invalid entries restrict (D-042)
		if len(v.Sites) > 0 {
			sites = []string{}
			for _, s := range v.Sites {
				if id, err := uuid.Parse(s); err == nil {
					sites = append(sites, id.String())
				}
			}
		}
		err = repository.SetPushUserAudience(c.Context(), h.DB, realm, v.ID, v.Roles, sites)
	}
	if err != nil {
		h.Log.Warn().Err(err).Str("user_id", v.ID).Msg("update push audience failed")
	}
}

// POST /users/:id/reset-password {send_email?, temporary_password?} — a
// temporary password (returned once) or an e-mail with a reset link.
func (h *Handler) resetUserPassword(c *fiber.Ctx) error {
	p := middleware.GetPrincipal(c)
	u, v, err := h.userFor(c, c.Params("id"))
	if err != nil {
		return h.fail(c, err)
	}
	if u.ID == p.Subject {
		return h.fail(c, service.Forbidden("cannot_modify_self", "change your own password in your Keycloak account"))
	}
	var b struct {
		SendEmail         bool   `json:"send_email"`
		TemporaryPassword string `json:"temporary_password"`
	}
	if len(c.Body()) > 0 {
		if err := parse(c, &b); err != nil {
			return h.fail(c, err)
		}
	}
	if err := service.CheckLengths(service.S("temporary_password", b.TemporaryPassword, 128)); err != nil {
		return h.fail(c, err)
	}
	middleware.SetAuditBefore(c, fiber.Map{"user_id": u.ID, "username": v.Username})
	if b.SendEmail {
		if u.Email == "" {
			return h.fail(c, service.BadRequest("email_required", "the user has no e-mail address"))
		}
		if err := h.kc().ExecuteActionsEmail(c.Context(), p.Realm, u.ID, []string{"UPDATE_PASSWORD"}, inviteLifespan); err != nil {
			return h.failKC(c, err)
		}
		middleware.SetAuditAfter(c, fiber.Map{"user_id": u.ID, "password_reset": "email"})
		return c.JSON(fiber.Map{"email_sent": true})
	}
	pw := b.TemporaryPassword
	if pw == "" {
		pw = kcadmin.GeneratePassword(16)
	}
	if err := h.kc().ResetPassword(c.Context(), p.Realm, u.ID, pw, true); err != nil {
		return h.failKC(c, err)
	}
	middleware.SetAuditAfter(c, fiber.Map{"user_id": u.ID, "password_reset": "temporary"})
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.JSON(fiber.Map{"temporary_password": pw})
}

// POST /users/:id/reset-otp — removes the OTP credentials and ends the
// sessions; the user enrols a new authenticator at the next login.
func (h *Handler) resetUserOTP(c *fiber.Ctx) error {
	p := middleware.GetPrincipal(c)
	u, v, err := h.userFor(c, c.Params("id"))
	if err != nil {
		return h.fail(c, err)
	}
	if u.ID == p.Subject {
		return h.fail(c, service.Forbidden("cannot_modify_self", "reset your own authenticator in your Keycloak account"))
	}
	middleware.SetAuditBefore(c, fiber.Map{"user_id": u.ID, "otp_configured": v.OTPConfigured})
	ctx := c.Context()
	removed, err := h.kc().ResetOTP(ctx, p.Realm, u.ID)
	if err != nil {
		return h.failKC(c, err)
	}
	if !slices.Contains(u.RequiredActions, "CONFIGURE_TOTP") {
		u.RequiredActions = append(u.RequiredActions, "CONFIGURE_TOTP")
		if err := h.kc().UpdateUser(ctx, p.Realm, u); err != nil {
			return h.failKC(c, err)
		}
	}
	if err := h.kc().Logout(ctx, p.Realm, u.ID); err != nil {
		return h.failKC(c, err)
	}
	out := fiber.Map{"user_id": u.ID, "otp_removed": removed, "otp_configured": false}
	middleware.SetAuditAfter(c, out)
	return c.JSON(out)
}

// POST /users/:id/logout — ends every session of the user.
func (h *Handler) logoutUser(c *fiber.Ctx) error {
	p := middleware.GetPrincipal(c)
	u, _, err := h.userFor(c, c.Params("id"))
	if err != nil {
		return h.fail(c, err)
	}
	middleware.SetAuditBefore(c, fiber.Map{"user_id": u.ID})
	if err := h.kc().Logout(c.Context(), p.Realm, u.ID); err != nil {
		return h.failKC(c, err)
	}
	middleware.SetAuditAfter(c, fiber.Map{"user_id": u.ID, "logged_out": true})
	return c.JSON(fiber.Map{"logged_out": true})
}

// ── Device accounts ──────────────────────────────────────────────────────────

// DeviceAccount is a tablet's Keycloak account with its registered device.
type DeviceAccount struct {
	ID        string        `json:"id"`
	Username  string        `json:"username"`
	Name      string        `json:"name"`
	Enabled   bool          `json:"enabled"`
	CreatedAt time.Time     `json:"created_at"`
	Device    *model.Device `json:"device,omitempty"` // registered tablet using this account (by keycloak sub)
}

// devicePasswordLen is the generated tablet password length.
const devicePasswordLen = 24

// deviceIndex maps keycloak user ID -> registered device of the realm.
func (h *Handler) deviceIndex(c *fiber.Ctx) (map[string]*model.Device, error) {
	devices, err := repository.ListDevicesByRealm(c.Context(), h.DB, middleware.GetPrincipal(c).Realm)
	if err != nil {
		return nil, err
	}
	idx := map[string]*model.Device{}
	for i := range devices {
		idx[devices[i].KeycloakUserID] = &devices[i]
	}
	return idx, nil
}

func deviceAccountOf(u *kcadmin.User, d *model.Device) DeviceAccount {
	a := DeviceAccount{ID: u.ID, Username: u.Username, Name: u.FirstName, Enabled: u.Enabled, Device: d}
	if u.CreatedTimestamp > 0 {
		a.CreatedAt = time.UnixMilli(u.CreatedTimestamp).UTC()
	}
	return a
}

// deviceVisible: a site-scoped admin sees accounts of tablets at their
// sites and accounts not linked to an assigned tablet yet.
func deviceVisible(p *middleware.Principal, d *model.Device) bool {
	return d == nil || d.SiteID == nil || p.CanSeeSite(*d.SiteID)
}

// GET /device-accounts
func (h *Handler) listDeviceAccounts(c *fiber.Ctx) error {
	p := middleware.GetPrincipal(c)
	users, err := h.kc().RoleMembers(c.Context(), p.Realm, kcadmin.RoleDevice)
	if err != nil {
		return h.failKC(c, err)
	}
	idx, err := h.deviceIndex(c)
	if err != nil {
		return h.fail(c, err)
	}
	out := []DeviceAccount{}
	for i := range users {
		d := idx[users[i].ID]
		if deviceVisible(p, d) {
			out = append(out, deviceAccountOf(&users[i], d))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Username < out[j].Username })
	return c.JSON(out)
}

// deviceAccountFor loads a device account (role device, no web role).
func (h *Handler) deviceAccountFor(c *fiber.Ctx, id string) (*kcadmin.User, DeviceAccount, error) {
	p := middleware.GetPrincipal(c)
	if _, err := uuid.Parse(id); err != nil {
		return nil, DeviceAccount{}, service.NotFound("user_not_found", "device account not found")
	}
	u, err := h.kc().GetUser(c.Context(), p.Realm, id)
	if err != nil {
		return nil, DeviceAccount{}, h.kcError(c, err)
	}
	roles, err := h.kc().RealmRoles(c.Context(), p.Realm, id)
	if err != nil {
		return nil, DeviceAccount{}, h.kcError(c, err)
	}
	isDevice := false
	for _, r := range roles {
		if slices.Contains(kcadmin.WebRoles, r.Name) {
			return nil, DeviceAccount{}, service.NotFound("user_not_found", "device account not found")
		}
		isDevice = isDevice || r.Name == kcadmin.RoleDevice
	}
	if !isDevice {
		return nil, DeviceAccount{}, service.NotFound("user_not_found", "device account not found")
	}
	idx, err := h.deviceIndex(c)
	if err != nil {
		return nil, DeviceAccount{}, err
	}
	d := idx[u.ID]
	if !deviceVisible(p, d) {
		return nil, DeviceAccount{}, service.NotFound("user_not_found", "device account not found")
	}
	return u, deviceAccountOf(u, d), nil
}

// POST /device-accounts {name} — creates Keycloak user device-<slug> with
// only the device role and a random password, returned once for the
// tablet's first login.
func (h *Handler) createDeviceAccount(c *fiber.Ctx) error {
	p := middleware.GetPrincipal(c)
	var b struct {
		Name string `json:"name"`
	}
	if err := parse(c, &b); err != nil {
		return h.fail(c, err)
	}
	b.Name = strings.TrimSpace(b.Name)
	slug := kcadmin.Slug(b.Name)
	if slug == "" || len(b.Name) > 100 {
		return h.fail(c, service.BadRequest("invalid_name", "name is required (max 100 characters, letters or digits)").WithField("name"))
	}
	middleware.SetAuditBefore(c, nil)
	ctx := c.Context()
	username := "device-" + slug
	// Device accounts log in with the password grant, which fails while a
	// profile is incomplete (VERIFY_PROFILE, Keycloak 24+): set names and a
	// non-deliverable e-mail (.invalid, RFC 2606).
	u := kcadmin.User{
		Username: username, FirstName: b.Name, LastName: "Tablet", Email: username + "@device.invalid",
		EmailVerified: true, Enabled: true, RequiredActions: []string{},
	}
	id, err := h.kc().CreateUser(ctx, p.Realm, u)
	if err != nil {
		return h.failKC(c, err)
	}
	u.ID = id
	rollback := func(cause error) error {
		if derr := h.kc().DeleteUser(context.Background(), p.Realm, id); derr != nil {
			h.Log.Error().Err(derr).Str("user_id", id).Msg("could not roll back a half-created device account")
		}
		return h.failKC(c, cause)
	}
	if err := h.kc().SetRealmRoles(ctx, p.Realm, id, []string{kcadmin.RoleDevice}, []string{kcadmin.RoleDevice}); err != nil {
		return rollback(err)
	}
	pw := kcadmin.GeneratePassword(devicePasswordLen)
	if err := h.kc().ResetPassword(ctx, p.Realm, id, pw, false); err != nil {
		return rollback(err)
	}
	acc := deviceAccountOf(&u, nil)
	acc.CreatedAt = service.Now()
	middleware.SetAuditAfter(c, acc)
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"account": acc, "password": pw})
}

// POST /device-accounts/:id/reset-password — new random password (returned
// once); the tablet's sessions end, so it must log in again.
func (h *Handler) resetDevicePassword(c *fiber.Ctx) error {
	p := middleware.GetPrincipal(c)
	u, acc, err := h.deviceAccountFor(c, c.Params("id"))
	if err != nil {
		return h.fail(c, err)
	}
	middleware.SetAuditBefore(c, acc)
	pw := kcadmin.GeneratePassword(devicePasswordLen)
	if err := h.kc().ResetPassword(c.Context(), p.Realm, u.ID, pw, false); err != nil {
		return h.failKC(c, err)
	}
	if err := h.kc().Logout(c.Context(), p.Realm, u.ID); err != nil {
		h.Log.Warn().Err(err).Str("user_id", u.ID).Msg("device logout after password reset failed")
	}
	middleware.SetAuditAfter(c, fiber.Map{"account_id": u.ID, "password_reset": true})
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.JSON(fiber.Map{"account": acc, "password": pw})
}

// PUT /device-accounts/:id {enabled} — disabling ends the tablet's sessions.
func (h *Handler) updateDeviceAccount(c *fiber.Ctx) error {
	p := middleware.GetPrincipal(c)
	u, acc, err := h.deviceAccountFor(c, c.Params("id"))
	if err != nil {
		return h.fail(c, err)
	}
	var b struct {
		Enabled *bool `json:"enabled"`
	}
	if err := parse(c, &b); err != nil {
		return h.fail(c, err)
	}
	if b.Enabled == nil {
		return h.fail(c, service.BadRequest("enabled_required", "enabled is required"))
	}
	middleware.SetAuditBefore(c, acc)
	u.Enabled = *b.Enabled
	if err := h.kc().UpdateUser(c.Context(), p.Realm, u); err != nil {
		return h.failKC(c, err)
	}
	if !u.Enabled {
		if err := h.kc().Logout(c.Context(), p.Realm, u.ID); err != nil {
			h.Log.Warn().Err(err).Str("user_id", u.ID).Msg("device logout after disable failed")
		}
	}
	acc.Enabled = u.Enabled
	middleware.SetAuditAfter(c, acc)
	return c.JSON(acc)
}
