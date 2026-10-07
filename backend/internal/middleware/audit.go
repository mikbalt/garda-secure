package middleware

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// Audit returns a Fiber middleware that writes an audit log entry to PostgreSQL
// for every mutating request (POST, PUT, PATCH, DELETE) after the handler runs.
// Must be registered after JWT so the principal is known; the acting guard is
// recorded too when the request came from a tablet.
func Audit(pool *pgxpool.Pool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		err := c.Next()

		switch c.Method() {
		case fiber.MethodPost, fiber.MethodPut, fiber.MethodPatch, fiber.MethodDelete:
		default:
			return err
		}

		user := ""
		if p := GetPrincipal(c); p != nil {
			user = p.Realm + "/" + p.Actor()
		}
		if g := GetGuard(c); g != nil {
			user += " guard:" + g.ID
		}
		action := c.Method() + " " + c.Route().Path
		resourceID := c.Params("id")
		ip := c.IP()
		userAgent := c.Get(fiber.HeaderUserAgent)
		statusCode := c.Response().StatusCode()

		// Fire-and-forget; don't fail the request if the audit write fails.
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			const q = `INSERT INTO audit_log
				(user_id, action, resource_id, ip, user_agent, status_code)
				VALUES ($1, $2, NULLIF($3,''), $4, $5, $6)`
			if _, err := pool.Exec(ctx, q, user, action, resourceID, ip, userAgent, statusCode); err != nil {
				log.Error().Err(err).Str("action", action).Msg("audit log write failed")
			}
		}()

		return err
	}
}
