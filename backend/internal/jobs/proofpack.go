package jobs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/securepatrol/backend/internal/chainproof"
	"github.com/securepatrol/backend/internal/model"
	"github.com/securepatrol/backend/internal/proofpack"
	"github.com/securepatrol/backend/internal/repository"
	"github.com/securepatrol/backend/internal/service"
	"github.com/securepatrol/backend/internal/storage"
)

// PackStore is the object store the scheduled proof packs are kept in.
type PackStore interface {
	Exists(ctx context.Context, bucket, key string) (bool, error)
	PutNew(ctx context.Context, bucket, key string, data []byte, contentType, sha256 string) error
}

const (
	// packMaxEntries bounds the chain read of one month, as the export endpoint does.
	packMaxEntries  = 250_000
	packReadTimeout = 2 * time.Minute
)

// ProofPacks keeps the proof pack of the previous month for every site
// (TAQ-20, D-074), from 07:00 site time on the first day of the month, as
// the monthly recap is sent.
func (s *Scheduler) ProofPacks(ctx context.Context) error {
	if s.stg == nil {
		return nil
	}
	return s.RunProofPacks(ctx, s.stg, service.Now())
}

// RunProofPacks is one run of the job at time now.
//
// Idempotent: the final copy "<month>.pdf" is written once, only when a
// signature made after the month ended covers it, and the job does nothing
// for a month that has one. Until then the latest pack is kept as
// "<month>.unsigned.pdf" (written once; a pack with the UNSIGNED TAIL notice
// is never kept under the final name) and the month is tried again once an
// hour, as the signer signs hourly.
func (s *Scheduler) RunProofPacks(ctx context.Context, store PackStore, now time.Time) error {
	sites, err := repository.ListAllSites(ctx, s.pool)
	if err != nil {
		return err
	}
	return eachIsolated(s.log, "proof pack", len(sites), func(i int) error {
		return s.packSite(ctx, store, &sites[i], now)
	})
}

func (s *Scheduler) packSite(ctx context.Context, store PackStore, site *model.Site, now time.Time) error {
	month, due := recapDue(now.In(site.Location()))
	if !due {
		return nil
	}
	bucket := s.cfg.MinioBucketBackups
	if ok, err := store.Exists(ctx, bucket, proofpack.Key(site.ID, month, true)); err != nil || ok {
		return err
	}
	unsignedKey := proofpack.Key(site.ID, month, false)
	kept, err := store.Exists(ctx, bucket, unsignedKey)
	if err != nil {
		return err
	}
	if kept && now.Minute() >= 10 { // retry once an hour
		return nil
	}
	from, to, err := service.MonthRange(month, site.Location())
	if err != nil {
		return err
	}
	b, err := s.readChain(ctx, site, from, to, now)
	if err != nil {
		return fmt.Errorf("proof pack %s %s: %w", site.ID, month, err)
	}
	data, err := proofpack.Collect(ctx, s.pool, site, month, b, now)
	if err != nil {
		return err
	}
	pdf, err := proofpack.Compose(data, site.Config().Language).Render(now.In(site.Location()))
	if err != nil {
		return err
	}
	key := unsignedKey
	if data.Chain.Status == proofpack.StatusSigned {
		key = proofpack.Key(site.ID, month, true)
	} else if kept {
		return nil
	}
	sum := sha256.Sum256(pdf)
	if err := store.PutNew(ctx, bucket, key, pdf, "application/pdf", hex.EncodeToString(sum[:])); err != nil && !errors.Is(err, storage.ErrExists) {
		return err
	}
	return nil
}

// readChain reads the month's bundle in one read-only snapshot, as the export endpoint does.
func (s *Scheduler) readChain(ctx context.Context, site *model.Site, from, to, now time.Time) (*chainproof.Bundle, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, fmt.Sprintf(`SET LOCAL statement_timeout = %d`, packReadTimeout.Milliseconds())); err != nil {
		return nil, err
	}
	return chainproof.Export(ctx, tx, site.ID, site.Timezone, from, to, now, packMaxEntries)
}
