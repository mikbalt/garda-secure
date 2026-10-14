package handler

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/securepatrol/backend/internal/middleware"
	"github.com/securepatrol/backend/internal/model"
	"github.com/securepatrol/backend/internal/repository"
	"github.com/securepatrol/backend/internal/service"
)

// Server-side PIN checks (D-035).
const (
	pinMaxFailures = 5                // consecutive wrong PINs before the lock
	pinLockFor     = 15 * time.Minute // lock duration
	maxPinLength   = 64               // bcrypt only reads 72 bytes
)

// pinResult is the outcome of a server PIN check.
type pinResult struct {
	guard        *model.Guard
	ok           bool
	attemptsLeft int
	lockedUntil  *time.Time
	denial       error // guard_not_allowed, pin_not_set
}

// POST /device/guard-sessions {guard_id, pin} — the tablet verifies a guard's
// PIN with the server and gets a guard token for X-Guard-Token.
func (h *Handler) createGuardSession(c *fiber.Ctx) error {
	var b struct {
		GuardID string `json:"guard_id"`
		Pin     string `json:"pin"`
	}
	if err := parse(c, &b); err != nil {
		return h.fail(c, err)
	}
	if b.Pin == "" {
		return h.fail(c, service.BadRequest("pin_required", "pin is required"))
	}
	if len(b.Pin) > maxPinLength {
		return h.fail(c, service.TooLong("pin", maxPinLength))
	}
	dev := middleware.GetDevice(c)
	if ok, retry := h.limiter.Allow(c.Context(), "guardpin:"+dev.ID, h.Cfg.RateLimitGuardSessions); !ok {
		return middleware.TooManyRequests(c, retry)
	}
	gid, err := uuid.Parse(b.GuardID)
	if err != nil {
		return h.fail(c, service.Forbidden("guard_not_allowed", "guard is not allowed on this device"))
	}
	middleware.SetAuditGuard(c, gid.String())

	res, err := h.checkGuardPin(c.Context(), gid.String(), *dev.SiteID, b.Pin)
	if err != nil {
		return h.fail(c, err)
	}
	if res.guard != nil && !res.ok {
		// Failed and locked attempts (401/423) record the guard's lockout
		// state before the attempt in their audit row. Not on success: the
		// "after" state would be the response, which holds the token.
		middleware.SetAuditBefore(c, fiber.Map{
			"guard_id": res.guard.ID, "pin_failed_attempts": res.guard.PinFailedAttempts,
			"pin_locked_until": res.guard.PinLockedUntil,
		})
	}
	if res.denial != nil {
		return h.fail(c, res.denial)
	}
	if res.lockedUntil != nil {
		secs := int(math.Ceil(time.Until(*res.lockedUntil).Seconds()))
		if secs < 1 {
			secs = 1
		}
		h.Log.Warn().Str("guard_id", res.guard.ID).Str("device_id", dev.ID).Time("locked_until", *res.lockedUntil).
			Msg("guard PIN locked after repeated failures")
		c.Set(fiber.HeaderRetryAfter, itoa(secs))
		return c.Status(fiber.StatusLocked).JSON(fiber.Map{
			"error": "too many wrong PINs; try again later", "code": "pin_locked", "retry_after": secs,
		})
	}
	if !res.ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "wrong PIN", "code": "invalid_pin", "attempts_left": res.attemptsLeft,
		})
	}
	token, exp, err := h.guardTokens.Issue(res.guard.ID, dev.ID, *dev.SiteID, time.Now())
	if err != nil {
		return h.fail(c, err)
	}
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.JSON(fiber.Map{"token": token, "guard_id": res.guard.ID, "expires_at": exp})
}

// checkGuardPin verifies a PIN under a row lock of the guard, so parallel
// attempts are counted exactly; failures and locks are committed.
func (h *Handler) checkGuardPin(ctx context.Context, guardID, siteID, pin string) (*pinResult, error) {
	res := &pinResult{}
	err := pgx.BeginFunc(ctx, h.DB, func(tx pgx.Tx) error {
		g, err := repository.GetGuardForUpdate(ctx, tx, guardID)
		if errors.Is(err, repository.ErrNotFound) {
			res.denial = service.Forbidden("guard_not_allowed", "guard is not allowed on this device")
			return nil
		}
		if err != nil {
			return err
		}
		res.guard = g
		if g.Status != "active" || g.SiteID == nil || *g.SiteID != siteID {
			res.denial = service.Forbidden("guard_not_allowed", "guard is not allowed on this device")
			return nil
		}
		if !g.HasPin {
			res.denial = service.Conflict("pin_not_set", "no PIN is set for this guard; ask an admin")
			return nil
		}
		now := time.Now()
		if g.PinLockedUntil != nil && g.PinLockedUntil.After(now) {
			res.lockedUntil = g.PinLockedUntil
			return nil
		}
		if bcrypt.CompareHashAndPassword([]byte(*g.PinHash), []byte(pin)) == nil {
			res.ok = true
			if g.PinFailedAttempts == 0 && g.PinLockedUntil == nil {
				return nil
			}
			return repository.SetGuardPinFailures(ctx, tx, g.ID, 0, nil)
		}
		failures := g.PinFailedAttempts + 1
		if failures >= pinMaxFailures {
			until := now.Add(pinLockFor)
			res.lockedUntil = &until
			return repository.SetGuardPinFailures(ctx, tx, g.ID, 0, &until)
		}
		res.attemptsLeft = pinMaxFailures - failures
		return repository.SetGuardPinFailures(ctx, tx, g.ID, failures, nil)
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

// POST /patrol/routes/:id/regenerate-qr — every checkpoint gets a new signed
// code (version + 1); all previously printed codes of the route stop
// verifying, so the QR sheet must be reprinted.
func (h *Handler) regenerateQR(c *fiber.Ctx) error {
	r, err := h.routeFor(c)
	if err != nil {
		return h.fail(c, err)
	}
	middleware.SetAuditBefore(c, r)
	service.RegenerateQR(r, h.qr)
	out, err := repository.UpdateRoute(c.Context(), h.DB, *r)
	if err != nil {
		return h.fail(c, err)
	}
	markSync(c, &out.SiteID)
	service.DecorateQR(out, h.qr)
	return c.JSON(out)
}

// deviceCheckpoint is a checkpoint as cached by tablets: signed checkpoints
// carry no payload (qr_code null), so a tablet cannot produce a valid scan
// without the printed code.
type deviceCheckpoint struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	QRCode     *string  `json:"qr_code"`
	QRSigned   bool     `json:"qr_signed"`
	SLAMinutes *int     `json:"sla_minutes,omitempty"`
	OrderIndex int      `json:"order_index"`
	Lat        *float64 `json:"lat,omitempty"`
	Lng        *float64 `json:"lng,omitempty"`
}

type deviceRoute struct {
	ID          string             `json:"id"`
	SiteID      string             `json:"site_id"`
	Name        string             `json:"name"`
	Checkpoints []deviceCheckpoint `json:"checkpoints"`
	Active      bool               `json:"active"`
	CreatedAt   time.Time          `json:"created_at"`
}

func deviceRoutesView(routes []model.Route) []deviceRoute {
	out := make([]deviceRoute, 0, len(routes))
	for _, r := range routes {
		dr := deviceRoute{ID: r.ID, SiteID: r.SiteID, Name: r.Name, Active: r.Active, CreatedAt: r.CreatedAt,
			Checkpoints: make([]deviceCheckpoint, 0, len(r.Checkpoints))}
		for _, cp := range r.Checkpoints {
			dc := deviceCheckpoint{ID: cp.ID, Name: cp.Name, QRSigned: cp.QRSigned, SLAMinutes: cp.SLAMinutes,
				OrderIndex: cp.OrderIndex, Lat: cp.Lat, Lng: cp.Lng}
			if !cp.QRSigned {
				code := cp.QRCode
				dc.QRCode = &code
			}
			dr.Checkpoints = append(dr.Checkpoints, dc)
		}
		out = append(out, dr)
	}
	return out
}
