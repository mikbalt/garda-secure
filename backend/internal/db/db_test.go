package db

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestApplyOptions(t *testing.T) {
	cfg, err := pgxpool.ParseConfig("postgres://u:p@localhost:5432/db?sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	applyOptions(cfg, Options{})
	if cfg.MaxConns != DefaultMaxConns {
		t.Errorf("MaxConns = %d", cfg.MaxConns)
	}
	if got := cfg.ConnConfig.RuntimeParams["statement_timeout"]; got != "30000" {
		t.Errorf("statement_timeout = %q", got)
	}

	cfg, _ = pgxpool.ParseConfig("postgres://u:p@localhost:5432/db")
	applyOptions(cfg, Options{MaxConns: 7, StatementTimeout: -1})
	if cfg.MaxConns != 7 {
		t.Errorf("MaxConns = %d", cfg.MaxConns)
	}
	if _, ok := cfg.ConnConfig.RuntimeParams["statement_timeout"]; ok {
		t.Error("negative timeout should disable statement_timeout")
	}

	cfg, _ = pgxpool.ParseConfig("postgres://u:p@localhost:5432/db")
	applyOptions(cfg, Options{StatementTimeout: 1500 * time.Millisecond})
	if got := cfg.ConnConfig.RuntimeParams["statement_timeout"]; got != "1500" {
		t.Errorf("statement_timeout = %q", got)
	}
}
