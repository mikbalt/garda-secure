package jobs

import (
	"context"
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

// PackStore is the object store the scheduled proof packs are kept in. The
// status of the pack is stored with the object.
type PackStore interface {
	Exists(ctx context.Context, bucket, key string) (bool, error)
	// PutNew writes once (storage.ErrExists when the key is taken).
	PutNew(ctx context.Context, bucket, key string, data []byte, status string) error
	// Put replaces.
	Put(ctx context.Context, bucket, key string, data []byte, status string) error
}

// objectStore adapts the MinIO client.
type objectStore struct{ *storage.Client }

func (o objectStore) PutNew(ctx context.Context, bucket, key string, data []byte, status string) error {
	return o.PutPack(ctx, bucket, key, data, status, false)
}

func (o objectStore) Put(ctx context.Context, bucket, key string, data []byte, status string) error {
	return o.PutPack(ctx, bucket, key, data, status, true)
}

// PackMaxEntries bounds the chain read of one month, as the export endpoint does; tests lower it.
var PackMaxEntries int64 = 250_000

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
	return s.RunProofPacks(ctx, objectStore{s.stg}, service.Now())
}

// RunProofPacks is one run of the job at time now.
//
// Rules: the final copy "<month>.pdf" is written once, only when the pack's
// status is signed (a signature made after the month ended covers it), and a
// month that has one is skipped. Until then the latest pack is kept as
// "<month>.open.pdf" with its real status (unsigned_tail, failed, not_proven,
// no_activity) stored with it. The open copy is written at the first run and
// then replaced by the latest pack in the first ten minutes of each hour (the
// signer signs hourly); at other times it is left alone.
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
	if ok, err := store.Exists(ctx, bucket, proofpack.KeyFinal(site.ID, month)); err != nil || ok {
		return err
	}
	openKey := proofpack.KeyOpen(site.ID, month)
	kept, err := store.Exists(ctx, bucket, openKey)
	if err != nil {
		return err
	}
	if kept && now.Minute() >= 10 {
		return nil
	}
	from, to, err := service.MonthRange(month, site.Location())
	if err != nil {
		return err
	}
	// The chain and the figures are read in one snapshot, inside the statement timeout.
	var data *proofpack.Data
	collect := func(tx pgx.Tx, withChain bool) (err error) {
		var b *chainproof.Bundle
		if withChain {
			if b, err = chainproof.Export(ctx, tx, site.ID, site.Timezone, from, to, now, PackMaxEntries); err != nil {
				return err
			}
		}
		data, err = proofpack.Collect(ctx, tx, site, month, b, now)
		return err
	}
	err = s.snapshot(ctx, func(tx pgx.Tx) error { return collect(tx, true) })
	var big *chainproof.TooLargeError
	if errors.As(err, &big) || chainproof.IsTimeout(err) {
		// Too much to check here: still make the pack, saying the chain was not checked (status not_proven).
		err = s.snapshot(ctx, func(tx pgx.Tx) error { return collect(tx, false) })
	}
	if err != nil {
		return fmt.Errorf("proof pack %s %s: %w", site.ID, month, err)
	}
	pdf, err := proofpack.Compose(data, site.Config().Language).Render(now.In(site.Location()))
	if err != nil {
		return err
	}
	status := string(data.Chain.Status)
	if data.Chain.Status == proofpack.StatusSigned {
		if err := store.PutNew(ctx, bucket, proofpack.KeyFinal(site.ID, month), pdf, status); err != nil && !errors.Is(err, storage.ErrExists) {
			return err
		}
		return nil
	}
	return store.Put(ctx, bucket, openKey, pdf, status)
}

// snapshot runs fn in one read-only repeatable-read transaction with a statement timeout.
func (s *Scheduler) snapshot(ctx context.Context, fn func(pgx.Tx) error) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, fmt.Sprintf(`SET LOCAL statement_timeout = %d`, packReadTimeout.Milliseconds())); err != nil {
		return err
	}
	return fn(tx)
}
