package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"sync"
	"syscall"
	"time"
	_ "time/tzdata" // site time zones must resolve in the minimal runtime image

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/fiber/v2/middleware/requestid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/securepatrol/backend/internal/config"
	"github.com/securepatrol/backend/internal/db"
	"github.com/securepatrol/backend/internal/handler"
	"github.com/securepatrol/backend/internal/jobs"
	"github.com/securepatrol/backend/internal/metrics"
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
		p, connErr := db.New(ctx, cfg.DatabaseURL, db.Options{
			MaxConns: cfg.DBMaxConns, StatementTimeout: cfg.DBStatementTimeout,
		})
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
		if mode == "api" {
			logger.Fatal().Err(err).Msg("redis ping failed")
		}
		// The worker dispatches from Postgres; Redis only shortens latency.
		logger.Warn().Err(err).Msg("redis ping failed; worker continues and polls postgres")
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
	// Photos are write-once through the API (BE-S4); versioning keeps any
	// out-of-band overwrite recoverable.
	for _, b := range []string{cfg.MinioBucketBackups, cfg.MinioBucketPhotos} {
		if err := stg.EnableVersioning(ctx, b); err != nil {
			logger.Warn().Err(err).Str("bucket", b).Msg("could not enable bucket versioning")
		}
	}

	settingsStore, err := settings.NewStore(pool, cfg)
	if err != nil {
		logger.Fatal().Err(err).Msg("settings store init failed")
	}
	notifier := notification.NewNotifier(pool, rdb, logger)

	// ── Worker mode: notification dispatch + scheduled jobs ─────────────────
	if mode == "worker" {
		runWorker(ctx, cfg, pool, rdb, settingsStore, notifier, stg, logger)
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

	metrics.RegisterAPI(prometheus.DefaultRegisterer)
	app.Use(requestid.New())
	app.Use(metrics.Fiber())
	app.Use(middleware.Logger(logger))
	app.Use(recover.New(recover.Config{
		EnableStackTrace: true,
		StackTraceHandler: func(c *fiber.Ctx, e any) {
			rid, _ := c.Locals("requestid").(string)
			logger.Error().Str("request_id", rid).Str("path", c.Path()).Str("method", c.Method()).
				Interface("panic", e).Str("stack", string(debug.Stack())).Msg("panic recovered")
		},
	}))
	if cfg.CORSAllowedOrigins != "" {
		app.Use(cors.New(cors.Config{
			AllowOrigins: cfg.CORSAllowedOrigins,
			AllowHeaders: "Origin, Content-Type, Accept, Authorization, " + middleware.GuardHeader + ", " + middleware.GuardTokenHeader,
			AllowMethods: "GET,POST,PUT,PATCH,DELETE,OPTIONS",
		}))
	}

	// Public routes. /metrics and /ready are for Prometheus and the
	// orchestrator on the internal network; nginx only proxies /api/ and
	// /health, so they are not reachable from outside.
	app.Get("/health", handler.Health(pool, rdb))
	app.Get("/ready", handler.Ready(pool, rdb, stg, cfg.MinioBucketPhotos))
	app.Get("/metrics", adaptor.HTTPHandler(promhttp.Handler()))

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

// runWorker runs the notification dispatcher and the scheduler, serves
// /health and /metrics on WORKER_HTTP_ADDR and shuts down gracefully:
// in-flight sends and jobs get WORKER_SHUTDOWN_TIMEOUT to finish, then
// running jobs (e.g. a backup) are cancelled.
func runWorker(ctx context.Context, cfg *config.Config, pool *pgxpool.Pool, rdb *redis.Client,
	st *settings.Store, notifier *notification.Notifier, stg *storage.Client, logger zerolog.Logger) {
	logger.Info().Int("senders", cfg.NotifyWorkers).Msg("starting notification worker and scheduler")
	metrics.RegisterWorker(prometheus.DefaultRegisterer)

	dispatcher := notification.NewWorker(pool, rdb, st, notifier, cfg.NotifyWorkers, logger)
	scheduler := jobs.NewScheduler(pool, rdb, notifier, stg, cfg, logger)
	abortCtx, abort := context.WithCancel(context.Background())
	defer abort()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); dispatcher.Run(ctx) }()
	go func() { defer wg.Done(); scheduler.Run(ctx, abortCtx) }()

	healthy := func() bool {
		now := time.Now()
		return now.Sub(dispatcher.Heartbeat()) <= workerStaleAfter && now.Sub(scheduler.Heartbeat()) <= workerStaleAfter
	}
	go func() {
		t := time.NewTicker(5 * time.Second)
		defer t.Stop()
		for {
			if healthy() {
				metrics.WorkerHeartbeat.SetToCurrentTime()
			}
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	}()

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		body := map[string]any{
			"dispatcher_last_tick": dispatcher.Heartbeat().UTC().Format(time.RFC3339),
			"scheduler_last_tick":  scheduler.Heartbeat().UTC().Format(time.RFC3339),
		}
		w.Header().Set("Content-Type", "application/json")
		if healthy() {
			body["status"] = "ok"
		} else {
			body["status"] = "stalled"
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		_ = json.NewEncoder(w).Encode(body)
	})
	mux.Handle("/metrics", promhttp.Handler())
	srv := &http.Server{Addr: cfg.WorkerHTTPAddr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		logger.Info().Str("addr", cfg.WorkerHTTPAddr).Msg("worker health/metrics server starting")
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error().Err(err).Msg("worker health/metrics server failed")
		}
	}()

	<-ctx.Done()
	logger.Info().Dur("timeout", cfg.WorkerShutdownTimeout).Msg("worker shutting down; finishing in-flight work")
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(cfg.WorkerShutdownTimeout):
		logger.Warn().Msg("shutdown timeout reached; cancelling running jobs")
		abort()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			logger.Error().Msg("worker did not stop in time")
		}
	}
	sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(sctx)
	logger.Info().Msg("worker stopped")
}

// workerStaleAfter is how long the dispatcher or the scheduler may go
// without a tick before the worker reports itself unhealthy.
const workerStaleAfter = 60 * time.Second
