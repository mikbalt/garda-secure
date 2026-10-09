package middleware

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"

	"github.com/securepatrol/backend/internal/model"
	"github.com/securepatrol/backend/internal/repository"
)

// GuardHeader carries the ID of the guard operating a shared tablet. The
// guard unlocked the tablet with a PIN checked locally; the API only checks
// that the guard is active and assigned to the device's site.
const GuardHeader = "X-Guard-ID"

// Denial is an authorization failure to render as JSON.
type Denial struct {
	Status int
	Msg    string
	Code   string
}

// Send writes the denial response.
func (d *Denial) Send(c *fiber.Ctx) error {
	body := fiber.Map{"error": d.Msg}
	if d.Code != "" {
		body["code"] = d.Code
	}
	return c.Status(d.Status).JSON(body)
}

// GetDevice returns the device loaded by LoadDevice, or nil.
func GetDevice(c *fiber.Ctx) *model.Device {
	d, _ := c.Locals("device").(*model.Device)
	return d
}

// GetGuard returns the acting guard set by RequireGuard, or nil.
func GetGuard(c *fiber.Ctx) *model.Guard {
	g, _ := c.Locals("guard").(*model.Guard)
	return g
}

// LoadDevice resolves the device record for principals with the device role.
// Unregistered devices pass through with no device set so they can register.
func LoadDevice(pool *pgxpool.Pool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		p := GetPrincipal(c)
		if p == nil || !p.HasRole(RoleDevice) {
			return c.Next()
		}
		d, err := repository.GetDeviceByIdentity(c.Context(), pool, p.Realm, p.Subject)
		switch {
		case errors.Is(err, repository.ErrNotFound):
			return c.Next()
		case err != nil:
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "device lookup failed"})
		}
		c.Locals("device", d)
		if d.Status == "active" && touchDue(d.ID, time.Now()) {
			id := d.ID
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := repository.TouchDevice(ctx, pool, id); err != nil {
					log.Warn().Err(err).Str("device_id", id).Msg("touch device failed")
				}
			}()
		}
		return c.Next()
	}
}

// touchInterval throttles last-seen updates per device and process, so a
// busy tablet causes at most one background write a minute (audit BE-R6).
const touchInterval = time.Minute

var lastTouch sync.Map // device ID → time.Time of the last touch

// touchDue reports whether the device's last-seen time should be written now
// and, if so, records the attempt.
func touchDue(deviceID string, now time.Time) bool {
	if v, ok := lastTouch.Load(deviceID); ok && now.Sub(v.(time.Time)) < touchInterval {
		return false
	}
	lastTouch.Store(deviceID, now)
	return true
}

// CheckActiveDevice verifies the caller is an approved device with a site.
func CheckActiveDevice(c *fiber.Ctx) *Denial {
	d := GetDevice(c)
	if d == nil {
		return &Denial{fiber.StatusForbidden, "device not registered", "device_unregistered"}
	}
	if d.Status != "active" || d.SiteID == nil {
		return &Denial{fiber.StatusForbidden, "device is not active", "device_" + d.Status}
	}
	return nil
}

// ResolveGuard loads the guard in X-Guard-ID, checks they may act on this
// device and stores them in the request locals.
func ResolveGuard(c *fiber.Ctx, pool *pgxpool.Pool) *Denial {
	d := GetDevice(c)
	guardID := c.Get(GuardHeader)
	if guardID == "" {
		return &Denial{fiber.StatusBadRequest, GuardHeader + " header is required", "guard_required"}
	}
	g, err := repository.GetGuard(c.Context(), pool, guardID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) || repository.IsInvalidInput(err) {
			return &Denial{fiber.StatusForbidden, "unknown guard", "guard_unknown"}
		}
		return &Denial{fiber.StatusInternalServerError, "guard lookup failed", ""}
	}
	if g.Status != "active" || g.SiteID == nil || d == nil || d.SiteID == nil || *g.SiteID != *d.SiteID {
		return &Denial{fiber.StatusForbidden, "guard is not allowed on this device", "guard_not_allowed"}
	}
	c.Locals("guard", g)
	return nil
}

// RequireActiveDevice allows only approved devices that are assigned a site.
func RequireActiveDevice() fiber.Handler {
	return func(c *fiber.Ctx) error {
		if d := CheckActiveDevice(c); d != nil {
			return d.Send(c)
		}
		return c.Next()
	}
}

// RequireGuard requires an active device acting for a valid guard.
func RequireGuard(pool *pgxpool.Pool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if d := CheckActiveDevice(c); d != nil {
			return d.Send(c)
		}
		if d := ResolveGuard(c, pool); d != nil {
			return d.Send(c)
		}
		return c.Next()
	}
}

// RequireRolesOrGuard admits web principals holding one of roles, or an
// active tablet acting for a valid guard.
func RequireRolesOrGuard(pool *pgxpool.Pool, roles ...string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		p := GetPrincipal(c)
		if p != nil && p.HasRole(RoleDevice) {
			if d := CheckActiveDevice(c); d != nil {
				return d.Send(c)
			}
			if d := ResolveGuard(c, pool); d != nil {
				return d.Send(c)
			}
			return c.Next()
		}
		if p == nil || !p.HasRole(roles...) {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "forbidden"})
		}
		return c.Next()
	}
}
