package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// Kinds of records in the per-site hash chain (migration 011, TAQ-17).
const (
	chainAttendance        = "attendance"
	chainScan              = "scan"
	chainIncident          = "incident"
	chainIncidentAmendment = "incident_amendment"
)

// ChainBreak is the first broken link of a site's record chain.
type ChainBreak struct {
	Seq    int64  `json:"seq"`
	Reason string `json:"reason"`
}

// FirstChainBreak checks a site's chain and returns its first broken link, or
// nil when the chain is intact.
func FirstChainBreak(ctx context.Context, db DBTX, siteID string) (*ChainBreak, error) {
	var b ChainBreak
	err := db.QueryRow(ctx, `SELECT seq, reason FROM chain_first_break($1::uuid)`, siteID).Scan(&b.Seq, &b.Reason)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("FirstChainBreak: %w", err)
	}
	return &b, nil
}

// chainAppend chains the record's current state. Call it in the transaction
// that wrote the record (see inTx).
func chainAppend(ctx context.Context, db DBTX, kind, id string) error {
	if _, err := db.Exec(ctx, `SELECT chain_append($1, $2::uuid)`, kind, id); err != nil {
		return fmt.Errorf("chain %s %s: %w", kind, id, err)
	}
	return nil
}

type beginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// inTx runs fn in a transaction, so a record and its chain entry commit
// together. On a pool it opens one; on a transaction it uses a savepoint, so
// an error rolls back only fn's writes.
func inTx(ctx context.Context, db DBTX, fn func(DBTX) error) error {
	b, ok := db.(beginner)
	if !ok {
		return fn(db)
	}
	tx, err := b.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}
	return tx.Commit(ctx)
}
