package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata" // site time zones must resolve in the minimal runtime image

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/fiber/v2/middleware/requestid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/securepatrol/backend/internal/config"
	"github.com/securepatrol/backend/internal/db"
	"github.com/securepatrol/backend/internal/handler"
	"github.com/securepatrol/backend/internal/jobs"
	"github.com/securepatrol/backend/internal/middleware"
	"github.com/securepatrol/backend/internal/notification"
	"github.com/securepatrol/backend/internal/settings"
	"github.com/securepatrol/backend/internal/storage"
)

func main() {
	// ── Config ───────────────────────────────────────────────────────────────
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		os.Exit(1)
	}

	// ── Logger ───────────────────────────────────────────────────────────────
	level, _ := zerolog.ParseLevel(cfg.LogLevel)
	mode := "api"
	if len(os.Args) > 1 && os.Args[1] == "worker" {
		mode = "worker"
	}
	logger := zerolog.New(os.Stdout).
		Level(level).
		With().Timestamp().Str("service", "securepatrol").Str("mode", mode).Logger()
	log.Logger = logger

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	// ── Postgres (5-retry exponential backoff) ───────────────────────────────
	var pool *pgxpool.Pool
	for attempt := 0; attempt < 5; attempt++ {
		wait := time.Duration(1<<attempt) * time.Second
		p, connErr := db.New(ctx, cfg.DatabaseURL)
		if connErr != nil {
			logger.Warn().Err(connErr).Int("attempt", attempt+1).
				Dur("retry_in", wait).Msg("postgres unavailable, retrying")
			time.Sleep(wait)
			continue
		}
		pool = p
		break
	}
	if pool == nil {
		logger.Fatal().Msg("could not connect to postgres after 5 attempts")
	}
	defer pool.Close()

	// ── Migrations (API only, so two processes never race on them) ─────────
	if mode == "api" {
		if err := db.RunMigrations(cfg.DatabaseURL, cfg.MigrationsPath); err != nil {
			logger.Fatal().Err(err).Msg("migrations failed")
		}
		logger.Info().Msg("migrations applied")
	}

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
	stg, err := storage.New(storage.Options{
		Endpoint: cfg.MinioEndpoint, PublicEndpoint: cfg.MinioPublicEndpoint,
		AccessKey: cfg.MinioAccessKey, SecretKey: cfg.MinioSecretKey,
		UseSSL: cfg.MinioUseSSL, PublicUseSSL: cfg.MinioPublicUseSSL,
	})
	if err != nil {
		logger.Fatal().Err(err).Msg("minio init failed")
	}
	if err := stg.EnsureBuckets(ctx, []string{cfg.MinioBucketPhotos, cfg.MinioBucketBackups}); err != nil {
		logger.Fatal().Err(err).Msg("minio bucket setup failed")
	}
	if err := stg.EnableVersioning(ctx, cfg.MinioBucketBackups); err != nil {
		logger.Warn().Err(err).Msg("could not enable versioning on backups bucket")
	}

	settingsStore, err := settings.NewStore(pool, cfg)
	if err != nil {
		logger.Fatal().Err(err).Msg("settings store init failed")
	}
	notifier := notification.NewNotifier(pool, rdb, logger)

	// ── Worker mode: notification dispatch + scheduled jobs ─────────────────
	if mode == "worker" {
		logger.Info().Msg("starting notification worker and scheduler")
		go notification.NewWorker(pool, rdb, settingsStore, logger).Run(ctx)
		go jobs.NewScheduler(pool, rdb, notifier, stg, cfg, logger).Run(ctx)
		<-ctx.Done()
		logger.Info().Msg("worker shutting down")
		return
	}

	// ── JWT validation (one issuer per client realm) ─────────────────────────
	realms := make([]middleware.RealmKeys, 0, len(cfg.KeycloakRealms))
	for _, r := range cfg.KeycloakRealms {
		realms = append(realms, middleware.RealmKeys{Realm: r, Issuer: cfg.IssuerFor(r), JWKSURL: cfg.JWKSURLFor(r)})
	}
	validator := middleware.NewValidator(realms, cfg.KeycloakAllowedClients, logger)
	validator.BindClients(cfg.KeycloakWebClient, cfg.KeycloakTabletClient)

	// ── Fiber app ────────────────────────────────────────────────────────────
	app := fiber.New(handler.AppConfig(cfg, logger))

	app.Use(requestid.New())
	app.Use(middleware.Logger(logger))
	app.Use(recover.New())
	if cfg.CORSAllowedOrigins != "" {
		app.Use(cors.New(cors.Config{
			AllowOrigins: cfg.CORSAllowedOrigins,
			AllowHeaders: "Origin, Content-Type, Accept, Authorization, " + middleware.GuardHeader,
			AllowMethods: "GET,POST,PUT,PATCH,DELETE,OPTIONS",
		}))
	}

	// Public routes
	app.Get("/health", handler.Health(pool, rdb))

	h := &handler.Handler{
		DB: pool, Redis: rdb, Storage: stg, Notifier: notifier, Settings: settingsStore, Cfg: cfg, Log: logger,
	}
	h.Mount(app, validator)

	// ── Graceful shutdown ────────────────────────────────────────────────────
	go func() {
		addr := ":" + cfg.Port
		logger.Info().Str("addr", addr).Msg("server starting")
		if err := app.Listen(addr); err != nil {
			logger.Fatal().Err(err).Msg("server error")
		}
	}()

	<-ctx.Done()
	logger.Info().Msg("shutting down server")
	if err := app.ShutdownWithTimeout(10 * time.Second); err != nil {
		logger.Error().Err(err).Msg("shutdown error")
	}
	logger.Info().Msg("server stopped")
}
