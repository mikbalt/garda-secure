package middleware

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"

	"github.com/securepatrol/backend/internal/fieldauth"
	"github.com/securepatrol/backend/internal/model"
	"github.com/securepatrol/backend/internal/repository"
)

// GuardHeader carries the ID of the guard operating a shared tablet. The API
// checks that the guard is active and assigned to the device's site.
const GuardHeader = "X-Guard-ID"

// GuardTokenHeader optionally carries the guard token from
// POST /device/guard-sessions (server-verified PIN, D-035). Without it the
// guard is unverified and new records are flagged guard_unverified.
const GuardTokenHeader = "X-Guard-Token" // #nosec G101 -- a header name, not a credential

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

// GetGuardAuth returns what the request proved about the acting guard (nil =
// no valid guard token: unverified).
func GetGuardAuth(c *fiber.Ctx) *fieldauth.GuardAuth {
	a, _ := c.Locals("guard_auth").(*fieldauth.GuardAuth)
	return a
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
// device and stores them in the request locals, then evaluates an
// X-Guard-Token: none → unverified; signed by us for this guard, device and
// site → GuardAuth (verified per record by event time); anything else →
// 403 guard_token_invalid, unless lenient (SOS), where it counts as
// unverified.
func ResolveGuard(c *fiber.Ctx, pool *pgxpool.Pool, tokens *fieldauth.GuardTokens, lenient bool) *Denial {
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
	tok := c.Get(GuardTokenHeader)
	if tok == "" || tokens == nil {
		return nil
	}
	claims, err := tokens.Parse(tok)
	if err != nil || claims.GuardID != g.ID || claims.DeviceID != d.ID || claims.SiteID != *d.SiteID {
		if lenient {
			return nil
		}
		return &Denial{fiber.StatusForbidden, "guard token is not valid for this guard and device; resend without it", "guard_token_invalid"}
	}
	c.Locals("guard_auth", tokens.Auth(claims, time.Now()))
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

// RequireGuard requires an active device acting for a valid guard; an
// invalid guard token is rejected (403 guard_token_invalid).
func RequireGuard(pool *pgxpool.Pool, tokens *fieldauth.GuardTokens) fiber.Handler {
	return requireGuard(pool, tokens, false)
}

// RequireGuardLenient is RequireGuard for SOS: an invalid guard token never
// rejects the request, the guard just counts as unverified.
func RequireGuardLenient(pool *pgxpool.Pool, tokens *fieldauth.GuardTokens) fiber.Handler {
	return requireGuard(pool, tokens, true)
}

func requireGuard(pool *pgxpool.Pool, tokens *fieldauth.GuardTokens, lenient bool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if d := CheckActiveDevice(c); d != nil {
			return d.Send(c)
		}
		if d := ResolveGuard(c, pool, tokens, lenient); d != nil {
			return d.Send(c)
		}
		return c.Next()
	}
}

// RequireRolesOrGuard admits web principals holding one of roles, or an
// active tablet acting for a valid guard.
func RequireRolesOrGuard(pool *pgxpool.Pool, tokens *fieldauth.GuardTokens, roles ...string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		p := GetPrincipal(c)
		if p != nil && p.HasRole(RoleDevice) {
			if d := CheckActiveDevice(c); d != nil {
				return d.Send(c)
			}
			if d := ResolveGuard(c, pool, tokens, false); d != nil {
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
