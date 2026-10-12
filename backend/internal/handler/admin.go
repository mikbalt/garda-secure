package handler

import (
	"encoding/json"
	"errors"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"golang.org/x/crypto/bcrypt"

	"github.com/securepatrol/backend/internal/middleware"
	"github.com/securepatrol/backend/internal/model"
	"github.com/securepatrol/backend/internal/repository"
	"github.com/securepatrol/backend/internal/service"
)

// ── /me ──────────────────────────────────────────────────────────────────────

func (h *Handler) me(c *fiber.Ctx) error {
	p := middleware.GetPrincipal(c)
	out := fiber.Map{
		"subject": p.Subject, "username": p.Username, "realm": p.Realm,
		"client_id": p.ClientID, "roles": p.Roles, "sites": p.Sites, "sites_restricted": p.SitesRestricted,
	}
	if p.Sites == nil {
		out["sites"] = []string{}
	}
	if d := middleware.GetDevice(c); d != nil {
		out["device"] = d
	}
	return c.JSON(out)
}

// ── Sites ────────────────────────────────────────────────────────────────────

type siteBody struct {
	Name     string          `json:"name"`
	ClientID *string         `json:"client_id"`
	Address  *string         `json:"address"`
	Timezone string          `json:"timezone"`
	Config   json.RawMessage `json:"config_json"`
}

func (b *siteBody) validate() error {
	b.Name = strings.TrimSpace(b.Name)
	if b.Name == "" {
		return service.BadRequest("name_required", "name is required")
	}
	if err := service.CheckLengths(service.S("name", b.Name, service.MaxName),
		service.F("client_id", b.ClientID, service.MaxShortText), service.F("address", b.Address, service.MaxAddress),
		service.S("timezone", b.Timezone, 64)); err != nil {
		return err
	}
	if b.Timezone == "" {
		b.Timezone = "Asia/Jakarta"
	}
	if _, err := time.LoadLocation(b.Timezone); err != nil {
		return service.BadRequest("invalid_timezone", "unknown time zone %q", b.Timezone)
	}
	if len(b.Config) == 0 || string(b.Config) == "null" {
		b.Config = json.RawMessage("{}")
	}
	cfg := model.DefaultSiteConfig()
	if err := json.Unmarshal(b.Config, &cfg); err != nil {
		return service.BadRequest("invalid_config", "config_json is invalid: %v", err)
	}
	for name, v := range map[string]int{
		"clock_window_minutes": cfg.ClockWindowMinutes, "late_grace_minutes": cfg.LateGraceMinutes,
		"early_out_grace_minutes": cfg.EarlyOutGraceMinutes, "absent_after_minutes": cfg.AbsentAfterMinutes,
		"idle_lock_minutes": cfg.IdleLockMinutes,
	} {
		if v < 0 || v > 720 {
			return service.BadRequest("invalid_config", "%s must be between 0 and 720", name)
		}
	}
	if cfg.ScanRadiusMeters < 0 || cfg.ScanRadiusMeters > 5000 {
		return service.BadRequest("invalid_config", "scan_radius_m must be between 0 and 5000")
	}
	if cfg.Language != "id" && cfg.Language != "en" {
		return service.BadRequest("invalid_config", "language must be id or en")
	}
	// Store the full effective config so the UI always sees every key.
	b.Config, _ = json.Marshal(cfg)
	return nil
}

func (h *Handler) listSites(c *fiber.Ctx) error {
	sites, err := h.visibleSites(c)
	if err != nil {
		return h.fail(c, err)
	}
	return c.JSON(sites)
}

func (h *Handler) getSite(c *fiber.Ctx) error {
	site, err := h.siteFor(c, c.Params("id"))
	if err != nil {
		return h.fail(c, err)
	}
	return c.JSON(site)
}

func (h *Handler) createSite(c *fiber.Ctx) error {
	var b siteBody
	if err := parse(c, &b); err != nil {
		return h.fail(c, err)
	}
	if err := b.validate(); err != nil {
		return h.fail(c, err)
	}
	realm := middleware.GetPrincipal(c).Realm
	site, err := repository.CreateSite(c.Context(), h.DB, model.Site{
		Name: b.Name, ClientID: b.ClientID, KeycloakRealm: &realm, Address: b.Address,
		Timezone: b.Timezone, ConfigJSON: b.Config,
	})
	if err != nil {
		return h.fail(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(site)
}

func (h *Handler) updateSite(c *fiber.Ctx) error {
	site, err := h.siteFor(c, c.Params("id"))
	if err != nil {
		return h.fail(c, err)
	}
	middleware.SetAuditBefore(c, site)
	var b siteBody
	if err := parse(c, &b); err != nil {
		return h.fail(c, err)
	}
	if err := b.validate(); err != nil {
		return h.fail(c, err)
	}
	site.Name, site.ClientID, site.Address, site.Timezone, site.ConfigJSON = b.Name, b.ClientID, b.Address, b.Timezone, b.Config
	out, err := repository.UpdateSite(c.Context(), h.DB, *site)
	if err != nil {
		return h.fail(c, err)
	}
	return c.JSON(out)
}

// ── Guards ───────────────────────────────────────────────────────────────────

type guardBody struct {
	Name           string  `json:"name"`
	EmployeeID     *string `json:"employee_id"`
	SiteID         *string `json:"site_id"`
	Role           string  `json:"role"`
	Status         string  `json:"status"`
	KeycloakUserID *string `json:"keycloak_user_id"`
}

// validateGuard checks a guard body; guardID is the guard being updated
// ("" when creating).
func (h *Handler) validateGuard(c *fiber.Ctx, b *guardBody, guardID string) error {
	b.Name = strings.TrimSpace(b.Name)
	if b.Name == "" {
		return service.BadRequest("name_required", "name is required")
	}
	if err := service.CheckLengths(service.S("name", b.Name, service.MaxName),
		service.F("employee_id", b.EmployeeID, service.MaxIdentifier),
		service.F("keycloak_user_id", b.KeycloakUserID, service.MaxIdentifier)); err != nil {
		return err
	}
	if b.Role == "" {
		b.Role = "guard"
	}
	if b.Role != "guard" && b.Role != "supervisor" {
		return service.BadRequest("invalid_role", "role must be guard or supervisor")
	}
	if b.Status == "" {
		b.Status = "active"
	}
	if b.Status != "active" && b.Status != "inactive" {
		return service.BadRequest("invalid_status", "status must be active or inactive")
	}
	b.SiteID = nilIfEmpty(b.SiteID)
	b.EmployeeID = nilIfEmpty(b.EmployeeID)
	b.KeycloakUserID = nilIfEmpty(b.KeycloakUserID)
	// Guards have no realm of their own: the site is what scopes them to a
	// client, so it is mandatory.
	if b.SiteID == nil {
		return service.BadRequest("site_id_required", "site_id is required")
	}
	if _, err := h.siteFor(c, *b.SiteID); err != nil {
		return err
	}
	if b.EmployeeID != nil {
		taken, err := repository.EmployeeIDTaken(c.Context(), h.DB, middleware.GetPrincipal(c).Realm, *b.EmployeeID, guardID)
		if err != nil {
			return err
		}
		if taken {
			return service.Conflict("employee_id_taken", "employee ID %q is already used by another guard", *b.EmployeeID).WithField("employee_id")
		}
	}
	return nil
}

// guardFor loads a guard assigned to one of the caller's sites.
func (h *Handler) guardFor(c *fiber.Ctx, id string) (*model.Guard, error) {
	g, err := repository.GetGuard(c.Context(), h.DB, id)
	if err != nil {
		return nil, err
	}
	if g.SiteID == nil {
		return nil, repository.ErrNotFound
	}
	if _, err := h.siteFor(c, *g.SiteID); err != nil {
		return nil, err
	}
	return g, nil
}

func (h *Handler) listGuards(c *fiber.Ctx) error {
	if siteID := c.Query("site_id"); siteID != "" {
		site, err := h.siteFor(c, siteID)
		if err != nil {
			return h.fail(c, err)
		}
		guards, err := repository.ListGuardsBySite(c.Context(), h.DB, site.ID)
		if err != nil {
			return h.fail(c, err)
		}
		return c.JSON(guards)
	}
	guards, err := repository.ListGuardsByRealm(c.Context(), h.DB, middleware.GetPrincipal(c).Realm)
	if err != nil {
		return h.fail(c, err)
	}
	if p := middleware.GetPrincipal(c); p.SitesRestricted {
		guards = slices.DeleteFunc(guards, func(g model.Guard) bool { return g.SiteID == nil || !p.CanSeeSite(*g.SiteID) })
	}
	return c.JSON(guards)
}

func (h *Handler) getGuard(c *fiber.Ctx) error {
	g, err := h.guardFor(c, c.Params("id"))
	if err != nil {
		return h.fail(c, err)
	}
	return c.JSON(g)
}

func (h *Handler) createGuard(c *fiber.Ctx) error {
	var b guardBody
	if err := parse(c, &b); err != nil {
		return h.fail(c, err)
	}
	if err := h.validateGuard(c, &b, ""); err != nil {
		return h.fail(c, err)
	}
	g, err := repository.CreateGuard(c.Context(), h.DB, model.Guard{
		Name: b.Name, EmployeeID: b.EmployeeID, SiteID: b.SiteID, Role: b.Role,
		Status: b.Status, KeycloakUserID: b.KeycloakUserID,
	})
	if err != nil {
		return h.fail(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(g)
}

func (h *Handler) updateGuard(c *fiber.Ctx) error {
	g, err := h.guardFor(c, c.Params("id"))
	if err != nil {
		return h.fail(c, err)
	}
	middleware.SetAuditBefore(c, g)
	var b guardBody
	if err := parse(c, &b); err != nil {
		return h.fail(c, err)
	}
	if err := h.validateGuard(c, &b, g.ID); err != nil {
		return h.fail(c, err)
	}
	g.Name, g.EmployeeID, g.SiteID, g.Role, g.Status, g.KeycloakUserID =
		b.Name, b.EmployeeID, b.SiteID, b.Role, b.Status, b.KeycloakUserID
	out, err := repository.UpdateGuard(c.Context(), h.DB, *g)
	if err != nil {
		return h.fail(c, err)
	}
	return c.JSON(out)
}

var pinRe = regexp.MustCompile(`^[0-9]{4,6}$`)

// weakPins are rejected because they are guessed first.
var weakPins = map[string]bool{
	"0000": true, "1111": true, "1234": true, "4321": true, "000000": true,
	"111111": true, "123456": true, "654321": true, "12345": true,
}

func (h *Handler) setGuardPin(c *fiber.Ctx) error {
	g, err := h.guardFor(c, c.Params("id"))
	if err != nil {
		return h.fail(c, err)
	}
	middleware.SetAuditBefore(c, fiber.Map{"guard_id": g.ID, "has_pin": g.HasPin})
	var b struct {
		Pin string `json:"pin"`
	}
	if err := parse(c, &b); err != nil {
		return h.fail(c, err)
	}
	if !pinRe.MatchString(b.Pin) {
		return h.fail(c, service.BadRequest("invalid_pin", "PIN must be 4–6 digits"))
	}
	if weakPins[b.Pin] {
		return h.fail(c, service.BadRequest("weak_pin", "PIN is too easy to guess"))
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(b.Pin), 12)
	if err != nil {
		return h.fail(c, err)
	}
	if err := repository.SetGuardPinHash(c.Context(), h.DB, g.ID, string(hash)); err != nil {
		return h.fail(c, err)
	}
	return c.JSON(fiber.Map{"has_pin": true})
}

// ── Devices (admin) ──────────────────────────────────────────────────────────

func (h *Handler) listDevices(c *fiber.Ctx) error {
	devices, err := repository.ListDevicesByRealm(c.Context(), h.DB, middleware.GetPrincipal(c).Realm)
	if err != nil {
		return h.fail(c, err)
	}
	// Site-scoped admins see the tablets of their sites and unassigned ones
	// (to approve them).
	if p := middleware.GetPrincipal(c); p.SitesRestricted {
		devices = slices.DeleteFunc(devices, func(d model.Device) bool { return d.SiteID != nil && !p.CanSeeSite(*d.SiteID) })
	}
	return c.JSON(devices)
}

func (h *Handler) updateDevice(c *fiber.Ctx) error {
	d, err := repository.GetDevice(c.Context(), h.DB, c.Params("id"))
	if err != nil {
		return h.fail(c, err)
	}
	if p := middleware.GetPrincipal(c); d.KeycloakRealm != p.Realm || (d.SiteID != nil && !p.CanSeeSite(*d.SiteID)) {
		return h.fail(c, repository.ErrNotFound)
	}
	middleware.SetAuditBefore(c, d)
	var b struct {
		Name   string  `json:"name"`
		SiteID *string `json:"site_id"`
		Status string  `json:"status"`
	}
	if err := parse(c, &b); err != nil {
		return h.fail(c, err)
	}
	b.Name = strings.TrimSpace(b.Name)
	if b.Name == "" {
		b.Name = d.Name
	}
	if err := service.CheckLengths(service.S("name", b.Name, service.MaxShortText)); err != nil {
		return h.fail(c, err)
	}
	switch b.Status {
	case "pending", "active", "revoked":
	default:
		return h.fail(c, service.BadRequest("invalid_status", "status must be pending, active or revoked"))
	}
	b.SiteID = nilIfEmpty(b.SiteID)
	if b.SiteID != nil {
		if _, err := h.siteFor(c, *b.SiteID); err != nil {
			return h.fail(c, err)
		}
	}
	if b.Status == "active" && b.SiteID == nil {
		return h.fail(c, service.BadRequest("site_required", "assign a site before activating the device"))
	}
	out, err := repository.UpdateDevice(c.Context(), h.DB, d.ID, b.Name, b.SiteID, b.Status, actor(c))
	if err != nil {
		return h.fail(c, err)
	}
	return c.JSON(out)
}

// ── Tablet bootstrap ─────────────────────────────────────────────────────────

func (h *Handler) registerDevice(c *fiber.Ctx) error {
	var b struct {
		Name string `json:"name"`
	}
	if err := parse(c, &b); err != nil {
		return h.fail(c, err)
	}
	b.Name = strings.TrimSpace(b.Name)
	if b.Name == "" || len(b.Name) > 100 {
		return h.fail(c, service.BadRequest("invalid_name", "name is required (max 100 characters)"))
	}
	p := middleware.GetPrincipal(c)
	d, err := repository.RegisterDevice(c.Context(), h.DB, p.Realm, p.Subject, b.Name)
	if err != nil {
		return h.fail(c, err)
	}
	return c.JSON(d)
}

func (h *Handler) deviceMe(c *fiber.Ctx) error {
	d := middleware.GetDevice(c)
	if d == nil {
		return h.fail(c, service.NotFound("device_unregistered", "device is not registered"))
	}
	out := fiber.Map{"device": d}
	if d.Status == "active" && d.SiteID != nil {
		site, err := repository.GetSite(c.Context(), h.DB, *d.SiteID)
		if err != nil && !errors.Is(err, repository.ErrNotFound) {
			return h.fail(c, err)
		}
		if site != nil {
			out["site"] = site
			out["config"] = site.Config()
		}
	}
	return c.JSON(out)
}

func (h *Handler) deviceRoster(c *fiber.Ctx) error {
	site, err := h.deviceSite(c)
	if err != nil {
		return h.fail(c, err)
	}
	roster, err := repository.ListRoster(c.Context(), h.DB, site.ID)
	if err != nil {
		return h.fail(c, err)
	}
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.JSON(fiber.Map{"site_id": site.ID, "guards": roster, "generated_at": time.Now().UTC()})
}

func (h *Handler) deviceRoutes(c *fiber.Ctx) error {
	site, err := h.deviceSite(c)
	if err != nil {
		return h.fail(c, err)
	}
	routes, err := repository.ListRoutes(c.Context(), h.DB, site.ID, true)
	if err != nil {
		return h.fail(c, err)
	}
	return c.JSON(deviceRoutesView(routes))
}

// deviceShifts lists the acting guard's shifts from yesterday to +7 days.
func (h *Handler) deviceShifts(c *fiber.Ctx) error {
	site, err := h.deviceSite(c)
	if err != nil {
		return h.fail(c, err)
	}
	g := middleware.GetGuard(c)
	now := service.Now()
	shifts, err := repository.ListShifts(c.Context(), h.DB, site.ID, g.ID, now.Add(-24*time.Hour), now.Add(7*24*time.Hour))
	if err != nil {
		return h.fail(c, err)
	}
	return c.JSON(shifts)
}

// deviceStatus tells the tablet where the acting guard stands right now: the
// running (or next) shift and its attendance, any open clock-in, and a
// running patrol — so the app is correct after a restart or a guard switch.
func (h *Handler) deviceStatus(c *fiber.Ctx) error {
	site, err := h.deviceSite(c)
	if err != nil {
		return h.fail(c, err)
	}
	g := middleware.GetGuard(c)
	ctx := c.Context()
	now := service.Now()
	out := fiber.Map{"server_time": now, "config": site.Config(), "guard": g,
		"guard_verified": middleware.GetGuardAuth(c).VerifiedAt(now, now)}

	opt := func(v any, err error) (any, error) {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, nil
		}
		return v, err
	}
	shift, err := repository.CurrentOrNextShift(ctx, h.DB, g.ID, site.ID, now)
	if v, err := opt(shift, err); err != nil {
		return h.fail(c, err)
	} else if v != nil {
		out["shift"] = shift
		rec, err := repository.GetAttendanceByShift(ctx, h.DB, shift.ID)
		if v, err := opt(rec, err); err != nil {
			return h.fail(c, err)
		} else if v != nil {
			out["shift_attendance"] = rec
		}
	}
	open, err := repository.GetOpenAttendance(ctx, h.DB, g.ID, site.ID)
	if v, err := opt(open, err); err != nil {
		return h.fail(c, err)
	} else if v != nil {
		out["open_attendance"] = open
	}
	sess, err := repository.GetActiveSessionForGuard(ctx, h.DB, g.ID, site.ID)
	if v, err := opt(sess, err); err != nil {
		return h.fail(c, err)
	} else if v != nil {
		p, err := service.LoadProgress(ctx, h.DB, *sess)
		if err != nil {
			return h.fail(c, err)
		}
		service.HideSignedQR(p)
		out["session"] = p
	}
	due, err := h.patrolsDue(ctx, site.ID, now)
	if err != nil {
		return h.fail(c, err)
	}
	out["patrols_due"] = due
	return c.JSON(out)
}

// deviceSos lists the site's unacknowledged SOS alerts for every tablet
// there, so supervisors on duty see alerts raised on other tablets.
func (h *Handler) deviceSos(c *fiber.Ctx) error {
	site, err := h.deviceSite(c)
	if err != nil {
		return h.fail(c, err)
	}
	alerts, err := repository.ListSosAlerts(c.Context(), h.DB, site.ID, true)
	if err != nil {
		return h.fail(c, err)
	}
	return c.JSON(alerts)
}
