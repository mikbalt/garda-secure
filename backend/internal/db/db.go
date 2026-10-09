package db

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Options tunes the connection pool. Zero values use the defaults.
type Options struct {
	// MaxConns caps the pool (default 20).
	MaxConns int32
	// StatementTimeout aborts runaway statements server-side (default 30 s;
	// negative disables it).
	StatementTimeout time.Duration
}

// Defaults for Options.
const (
	DefaultMaxConns         = 20
	DefaultStatementTimeout = 30 * time.Second
)

// New creates and validates a PostgreSQL connection pool.
func New(ctx context.Context, databaseURL string, o Options) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("pgxpool.ParseConfig: %w", err)
	}
	applyOptions(cfg, o)
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("pgxpool.New: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("db ping: %w", err)
	}
	return pool, nil
}

func applyOptions(cfg *pgxpool.Config, o Options) {
	cfg.MaxConns = o.MaxConns
	if cfg.MaxConns <= 0 {
		cfg.MaxConns = DefaultMaxConns
	}
	timeout := o.StatementTimeout
	if timeout == 0 {
		timeout = DefaultStatementTimeout
	}
	if timeout > 0 {
		if cfg.ConnConfig.RuntimeParams == nil {
			cfg.ConnConfig.RuntimeParams = map[string]string{}
		}
		cfg.ConnConfig.RuntimeParams["statement_timeout"] = strconv.FormatInt(timeout.Milliseconds(), 10)
	}
}

// RunMigrations applies all pending up migrations from the given directory.
func RunMigrations(databaseURL, migrationsPath string) error {
	m, err := migrate.New("file://"+migrationsPath, databaseURL)
	if err != nil {
		return fmt.Errorf("migrate.New: %w", err)
	}
	defer m.Close()

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("migrate.Up: %w", err)
	}
	return nil
}
