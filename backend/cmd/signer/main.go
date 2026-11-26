// Command signer is the chain-head signer (TAQ-56, D-071): its own process,
// key and database role. Commands:
//
//	signer run          sign due heads every SIGNER_INTERVAL; serves /health and POST /sign
//	signer keygen       create the key file (0400); prints the public key and its fingerprint
//	signer rotate       --old-key f --new-key f: new key joins the current epoch (both sign)
//	signer new-epoch    --new-key f --reason "...": start a new epoch after loss or compromise
//	signer verify       --site uuid [--pin fp]... [--max-age 26h]: check the site's signed heads against its chain,
//	                    the signer's own record and the age of the newest head; non-zero exit on any problem
//	signer export       [--site uuid] [--out file]: signed heads from the signer's own record + keyring, as JSON
//	signer restore-heads  re-insert heads on the signer volume that signed_heads lacks (after investigating)
//	signer pubkey       print the fingerprint of the key file
//	signer healthcheck  exit 0 when the local signer reports healthy
package main

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/securepatrol/backend/internal/signer"
)

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: signer run|keygen|rotate|new-epoch|pubkey|healthcheck")
		os.Exit(2)
	}
	if err := dispatch(os.Args[1], os.Args[2:]); err != nil {
		fmt.Fprintln(os.Stderr, "signer:", err)
		os.Exit(1)
	}
}

func dispatch(cmd string, args []string) error {
	keyFile := env("SIGNER_KEY_FILE", "/keys/signer.key")
	switch cmd {
	case "keygen":
		pub, err := signer.GenerateKeyFile(keyFile)
		if err != nil {
			return err
		}
		fmt.Printf("key file: %s\npublic key: %s\nfingerprint (pin this out of band): %s\n", keyFile, hex.EncodeToString(pub), signer.KeyID(pub))
		return nil
	case "pubkey":
		k, err := signer.LoadKeyFile(keyFile)
		if err != nil {
			return err
		}
		fmt.Println(signer.KeyID(k.Public().(ed25519.PublicKey)))
		return nil
	case "healthcheck":
		c := http.Client{Timeout: 3 * time.Second}
		resp, err := c.Get("http://127.0.0.1" + env("SIGNER_ADDR", ":9092") + "/health")
		if err != nil {
			return err
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("health: %s", resp.Status)
		}
		return nil
	case "run":
		return run(keyFile)
	case "rotate", "new-epoch":
		return keyring(cmd, args)
	case "verify":
		return verifyCmd(args)
	case "export":
		return exportCmd(args)
	case "restore-heads":
		return restoreCmd()
	}
	return fmt.Errorf("unknown command %q", cmd)
}

// stateFile is the signer's own record of every head it signed, on its volume
// (outside the database); see signer.HighWater.
func stateFile() string { return env("SIGNER_STATE_FILE", "/keys/signed-heads.jsonl") }

func connect(ctx context.Context) (*pgxpool.Pool, error) {
	url := os.Getenv("SIGNER_DATABASE_URL")
	if url == "" {
		return nil, errors.New("SIGNER_DATABASE_URL is required")
	}
	return pgxpool.New(ctx, url)
}

func keyring(cmd string, args []string) error {
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	oldKey := fs.String("old-key", "", "current key file (rotate)")
	newKey := fs.String("new-key", "", "new key file")
	reason := fs.String("reason", "", "why (new-epoch)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	nk, err := signer.LoadKeyFile(*newKey)
	if err != nil {
		return fmt.Errorf("new key: %w", err)
	}
	ctx := context.Background()
	pool, err := connect(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()
	if cmd == "rotate" {
		ok, err := signer.LoadKeyFile(*oldKey)
		if err != nil {
			return fmt.Errorf("old key: %w", err)
		}
		return signer.Rotate(ctx, pool, ok, nk, time.Now())
	}
	if *reason == "" {
		return errors.New("--reason is required")
	}
	return signer.NewEpoch(ctx, pool, nk, *reason, time.Now())
}

func run(keyFile string) error {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	key, err := signer.LoadKeyFile(keyFile)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	pool, err := connect(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()
	s, err := signer.New(pool, key, stateFile())
	if err != nil {
		return err
	}
	if err := s.Bootstrap(ctx); err != nil {
		return err
	}
	keyID := signer.KeyID(key.Public().(ed25519.PublicKey))
	log.Info("signer started", "key_id", keyID)

	interval, err := time.ParseDuration(env("SIGNER_INTERVAL", "1m"))
	if err != nil || interval <= 0 {
		return errors.New("SIGNER_INTERVAL: not a duration")
	}
	var lastOK atomic.Int64 // unix seconds of the last tick that reached the database
	lastOK.Store(time.Now().Unix())

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		if time.Since(time.Unix(lastOK.Load(), 0)) > 3*interval+30*time.Second {
			http.Error(w, "no successful tick", http.StatusServiceUnavailable)
			return
		}
		fmt.Fprintln(w, "ok")
	})
	mux.HandleFunc("GET /pubkey", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"key_id": keyID, "public_key": hex.EncodeToString(key.Public().(ed25519.PublicKey))})
	})
	// On demand (proof-pack export): signs the current head of one site. Internal network only.
	mux.HandleFunc("POST /sign", func(w http.ResponseWriter, r *http.Request) {
		site := r.URL.Query().Get("site_id")
		if _, err := uuid.Parse(site); err != nil {
			http.Error(w, "site_id must be a uuid", http.StatusBadRequest)
			return
		}
		h, err := s.SignSite(r.Context(), site, signer.ReasonOnDemand)
		switch {
		case errors.Is(err, signer.ErrNoChain):
			http.Error(w, err.Error(), http.StatusNotFound)
		case err != nil:
			log.Error("on-demand sign refused", "site_id", site, "err", err)
			http.Error(w, err.Error(), http.StatusConflict)
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"site_id": h.SiteID, "seq": h.Seq,
				"entry_hash": hex.EncodeToString(h.EntryHash), "signed_at": h.SignedAt, "key_id": h.KeyID,
				"signature": hex.EncodeToString(h.Signature)})
		}
	})
	srv := &http.Server{Addr: env("SIGNER_ADDR", ":9092"), Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("http", "err", err)
			stop()
		}
	}()

	tick := func() {
		n, err := s.SignDue(ctx)
		if err != nil {
			log.Error("sign due heads", "signed", n, "err", err)
		} else if n > 0 {
			log.Info("signed heads", "count", n)
		}
		// A refused site (broken chain) is an alert, not a dead signer: only
		// a database that cannot be reached, or that lost heads this signer
		// has on disk (the owner wiped signed_heads?), turns the health red.
		if errors.Is(err, signer.ErrHeadsLost) {
			log.Error("SIGNED HEADS LOST FROM THE DATABASE: signing is refused for the affected sites", "err", err)
		} else if pool.Ping(ctx) == nil {
			lastOK.Store(time.Now().Unix())
		}
	}
	tick()
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			return srv.Shutdown(shutdown)
		case <-t.C:
			tick()
		}
	}
}

type pinList []string

func (p *pinList) String() string     { return fmt.Sprint(*p) }
func (p *pinList) Set(v string) error { *p = append(*p, v); return nil }

// verifyCmd exits non-zero when a signed head does not hold.
func verifyCmd(args []string) error {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	site := fs.String("site", "", "site id (uuid)")
	var pins pinList
	fs.Var(&pins, "pin", "key fingerprint pinned out of band (repeatable; none = trust the genesis key)")
	maxAge := fs.Duration("max-age", 0, "fail when the chain has entries after the newest signed head and that head is older than this (0 = off)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if _, err := uuid.Parse(*site); err != nil {
		return errors.New("--site must be a uuid")
	}
	ctx := context.Background()
	pool, err := connect(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()
	opts := signer.VerifyOptions{MaxAge: *maxAge}
	if hw, err := signer.LoadHighWater(stateFile()); err == nil {
		opts.Local = hw.Heads()
	}
	b, err := signer.Verify(ctx, pool, *site, pins, opts)
	if err != nil {
		return err
	}
	if b != nil {
		return fmt.Errorf("SIGNED HEAD %s at seq %d: %s", b.Status, b.Seq, b.Reason)
	}
	fmt.Println("signed heads verify")
	return nil
}

func openSigner(ctx context.Context) (*signer.Signer, *pgxpool.Pool, error) {
	key, err := signer.LoadKeyFile(env("SIGNER_KEY_FILE", "/keys/signer.key"))
	if err != nil {
		return nil, nil, err
	}
	pool, err := connect(ctx)
	if err != nil {
		return nil, nil, err
	}
	s, err := signer.New(pool, key, stateFile())
	if err != nil {
		pool.Close()
		return nil, nil, err
	}
	return s, pool, nil
}

func exportCmd(args []string) error {
	fs := flag.NewFlagSet("export", flag.ContinueOnError)
	site := fs.String("site", "", "only this site (uuid)")
	out := fs.String("out", "", "write to this file instead of stdout")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *site != "" {
		u, err := uuid.Parse(*site)
		if err != nil {
			return errors.New("--site must be a uuid")
		}
		*site = u.String()
	}
	ctx := context.Background()
	s, pool, err := openSigner(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()
	w := os.Stdout
	if *out != "" {
		f, err := os.OpenFile(*out, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600) // #nosec G304 -- operator-supplied path
		if err != nil {
			return err
		}
		defer f.Close()
		w = f
	}
	return s.Export(ctx, w, *site)
}

func restoreCmd() error {
	ctx := context.Background()
	s, pool, err := openSigner(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()
	n, err := s.RestoreHeads(ctx)
	fmt.Printf("restored %d head(s) from %s\n", n, stateFile())
	return err
}
