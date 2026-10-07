package middleware

import (
	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/securepatrol/backend/internal/storage"
)

// Inject sets the db pool and storage client into the request context locals
// so handlers can retrieve them with c.Locals("db") and c.Locals("storage").
func Inject(pool *pgxpool.Pool, stg *storage.Client) fiber.Handler {
	return func(c *fiber.Ctx) error {
		c.Locals("db", pool)
		c.Locals("storage", stg)
		return c.Next()
	}
}
