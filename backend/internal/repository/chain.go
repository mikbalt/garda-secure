package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/securepatrol/backend/internal/signer"
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

// VerifyChain is FirstChainBreak plus the signature check (TAQ-56): the chain
// must also match every head the signer has signed. pins are the signer key
// fingerprints the caller holds out of band (none = trust the genesis key on
// first use); opts adds the staleness limit and the signer's own record. A past that was rewritten and re-chained passes FirstChainBreak
// but fails here. The first break by seq is returned.
func VerifyChain(ctx context.Context, db DBTX, siteID string, pins []string, opts signer.VerifyOptions) (*ChainBreak, error) {
	b, err := FirstChainBreak(ctx, db, siteID)
	if err != nil {
		return nil, err
	}
	hb, err := signer.Verify(ctx, db, siteID, pins, opts)
	if err != nil {
		return nil, fmt.Errorf("VerifyChain: %w", err)
	}
	if hb != nil && (b == nil || hb.Seq < b.Seq) {
		return &ChainBreak{Seq: hb.Seq, Reason: "signed head " + hb.Status + ": " + hb.Reason}, nil
	}
	return b, nil
}
