package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/fiber/v2/middleware/requestid"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/securepatrol/backend/internal/config"
	"github.com/securepatrol/backend/internal/db"
	"github.com/securepatrol/backend/internal/handler"
	"github.com/securepatrol/backend/internal/middleware"
	"github.com/securepatrol/backend/internal/storage"
)

func main() {
	// ── Config ──────────────────────────────────────────────────────────────
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		os.Exit(1)
	}

	// ── Logger ───────────────────────────────────────────────────────────────
	level, _ := zerolog.ParseLevel(cfg.LogLevel)
	logger := zerolog.New(os.Stdout).
		Level(level).
		With().Timestamp().Str("service", "securepatrol").Logger()
	log.Logger = logger

	// ── Postgres ─────────────────────────────────────────────────────────────
	var pool interface{ Ping(context.Context) error }
	ctx := context.Background()

	var pgPool interface {
		Ping(context.Context) error
		Close()
	}

	for attempt := 0; attempt < 5; attempt++ {
		wait := time.Duration(1<<attempt) * time.Second
		p, err := db.New(ctx, cfg.DatabaseURL)
		if err != nil {
			logger.Warn().Err(err).Int("attempt", attempt+1).
				Dur("retry_in", wait).Msg("postgres unavailable, retrying")
			time.Sleep(wait)
			continue
		}
		pgPool = p
		pool = p
		break
	}
	if pool == nil {
		logger.Fatal().Msg("could not connect to postgres after 5 attempts")
	}
	defer pgPool.Close()

	// ── Migrations ───────────────────────────────────────────────────────────
	if err := db.RunMigrations(cfg.DatabaseURL, "/migrations"); err != nil {
		logger.Fatal().Err(err).Msg("migrations failed")
	}
	logger.Info().Msg("migrations applied")

	// ── Redis ────────────────────────────────────────────────────────────────
	opt, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		logger.Fatal().Err(err).Msg("invalid REDIS_URL")
	}
	rdb := redis.NewClient(opt)
	defer rdb.Close()
	if err := rdb.Ping(ctx).Err(); err != nil {
		logger.Fatal().Err(err).Msg("redis ping failed")
	}

	// ── MinIO ────────────────────────────────────────────────────────────────
	stg, err := storage.New(cfg.MinioEndpoint, cfg.MinioAccessKey, cfg.MinioSecretKey, false)
	if err != nil {
		logger.Fatal().Err(err).Msg("minio init failed")
	}
	if err := stg.EnsureBuckets(ctx, []string{cfg.MinioBucketPhotos, cfg.MinioBucketBackups}); err != nil {
		logger.Fatal().Err(err).Msg("minio bucket setup failed")
	}

	// ── Worker mode ──────────────────────────────────────────────────────────
	if len(os.Args) > 1 && os.Args[1] == "worker" {
		logger.Info().Msg("starting notification worker")
		// TODO: consume Redis queue and dispatch SMTP / WhatsApp notifications
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)
		<-quit
		logger.Info().Msg("worker shutting down")
		return
	}

	// ── JWT middleware ───────────────────────────────────────────────────────
	jwtMiddleware, err := middleware.JWT(cfg.KeycloakJWKSURL, cfg.KeycloakIssuer)
	if err != nil {
		logger.Fatal().Err(err).Msg("JWT middleware init failed")
	}

	// ── Fiber app ────────────────────────────────────────────────────────────
	app := fiber.New(fiber.Config{
		AppName:      "SecurePatrol API v1",
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	})

	app.Use(requestid.New())
	app.Use(middleware.Logger(logger))
	app.Use(recover.New())
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowHeaders: "Origin, Content-Type, Accept, Authorization",
		AllowMethods: "GET,POST,PUT,PATCH,DELETE,OPTIONS",
	}))

	// Public routes
	app.Get("/health", handler.Health(pgPool.(*interface{ Ping(context.Context) error }), rdb))

	// Protected API group
	_ = jwtMiddleware // used via closure below
	api := app.Group("/api/v1", jwtMiddleware)
	api.Mount("/attendance", handler.AttendanceRouter())
	api.Mount("/patrol", handler.PatrolRouter())
	api.Mount("/incidents", handler.IncidentRouter())

	// ── Graceful shutdown ────────────────────────────────────────────────────
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		addr := ":" + cfg.Port
		logger.Info().Str("addr", addr).Msg("server starting")
		if err := app.Listen(addr); err != nil {
			logger.Fatal().Err(err).Msg("server error")
		}
	}()

	<-quit
	logger.Info().Msg("shutting down server")
	if err := app.ShutdownWithTimeout(10 * time.Second); err != nil {
		logger.Error().Err(err).Msg("shutdown error")
	}
	logger.Info().Msg("server stopped")
}
