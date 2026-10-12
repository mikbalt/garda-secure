package handler

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/securepatrol/backend/internal/storage"
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

// Ready returns a readiness handler: 200 only when Postgres, Redis and MinIO
// (photos bucket) are all reachable, else 503 with per-dependency status.
func Ready(pool *pgxpool.Pool, rdb *redis.Client, stg *storage.Client, photosBucket string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		checks := map[string]func(context.Context) error{
			"db":    pool.Ping,
			"redis": func(ctx context.Context) error { return rdb.Ping(ctx).Err() },
			"minio": func(ctx context.Context) error { return stg.Ping(ctx, photosBucket) },
		}
		type result struct {
			name string
			ok   bool
		}
		results := make(chan result, len(checks))
		for name, check := range checks {
			go func(name string, check func(context.Context) error) {
				results <- result{name, check(ctx) == nil}
			}(name, check)
		}
		body := fiber.Map{"version": version}
		ready := true
		for range checks {
			r := <-results
			if r.ok {
				body[r.name] = "ok"
			} else {
				body[r.name] = "error"
				ready = false
			}
		}
		if !ready {
			body["status"] = "not_ready"
			return c.Status(fiber.StatusServiceUnavailable).JSON(body)
		}
		body["status"] = "ready"
		return c.JSON(body)
	}
}
