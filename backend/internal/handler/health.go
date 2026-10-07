package handler

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

const version = "1.0.0"

// Health returns a handler that checks DB and Redis liveness.
func Health(pool *pgxpool.Pool, rdb *redis.Client) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		// Health is public: report status only, never error details.
		dbStatus := "ok"
		if err := pool.Ping(ctx); err != nil {
			dbStatus = "error"
		}

		redisStatus := "ok"
		if err := rdb.Ping(ctx).Err(); err != nil {
			redisStatus = "error"
		}

		overall := "ok"
		httpStatus := fiber.StatusOK
		if dbStatus != "ok" || redisStatus != "ok" {
			overall = "degraded"
			httpStatus = fiber.StatusServiceUnavailable
		}

		return c.Status(httpStatus).JSON(fiber.Map{
			"status":  overall,
			"db":      dbStatus,
			"redis":   redisStatus,
			"version": version,
		})
	}
}
