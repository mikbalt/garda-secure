package middleware

import (
	"context"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Audit returns a Fiber middleware that writes an audit log entry to PostgreSQL
// for every mutating request (POST, PUT, PATCH, DELETE) after the handler runs.
func Audit(pool *pgxpool.Pool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		err := c.Next()

		method := c.Method()
		if method != fiber.MethodPost &&
			method != fiber.MethodPut &&
			method != fiber.MethodPatch &&
			method != fiber.MethodDelete {
			return err
		}

		userID, _ := c.Locals("userID").(string)
		action := method + " " + c.Path()
		resourceID := c.Params("id")
		ip := c.IP()
		userAgent := c.Get("User-Agent")
		statusCode := c.Response().StatusCode()

		// Fire-and-forget; don't fail the request if audit write fails.
		go func() {
			const q = `INSERT INTO audit_log
				(user_id, action, resource_id, ip, user_agent, status_code)
				VALUES ($1, $2, NULLIF($3,''), $4, $5, $6)`
			_, _ = pool.Exec(context.Background(), q,
				userID, action, resourceID, ip, userAgent, statusCode)
		}()

		return err
	}
}
