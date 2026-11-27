package handler

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"

	"github.com/securepatrol/backend/internal/chainproof"
	"github.com/securepatrol/backend/internal/middleware"
	"github.com/securepatrol/backend/internal/model"
	"github.com/securepatrol/backend/internal/repository"
	"github.com/securepatrol/backend/internal/service"
	"github.com/securepatrol/backend/internal/signer"
)

// Bounds of the online check (TAQ-18, D-073). The check reads the chain from a
// signed head before the range to a signed head after it, so its cost is the
// distance between heads, not the site's history. Past these limits the API
// says so and the caller uses the export and garda-verify.
const (
	chainMaxDays = 93
	verifyMaxAge = 26 * time.Hour // as `signer verify --max-age`
)

// ChainLimits are the bounds of the two chain endpoints; tests lower them.
var ChainLimits = struct {
	VerifyMaxEntries, ExportMaxEntries int64
	VerifyTimeout, ExportTimeout       time.Duration
	ExportMaxBytes                     int64 // size of the zip held in memory
	PerUserPerMinute                   int
}{20_000, 250_000, 15 * time.Second, 60 * time.Second, 256 << 20, 6}

// Chain reads are heavy and rarely needed: two verifications and two exports
// at a time per API process, in separate slots so that a client-readonly user
// running verifications cannot make admin exports wait, and each user may
// have one of each running and a few per minute.
var (
	verifySlots = make(chan struct{}, 2)
	exportSlots = make(chan struct{}, 2)
	chainUsers  = &userGate{running: map[string]bool{}, starts: map[string][]time.Time{}}
)

type userGate struct {
	mu      sync.Mutex
	running map[string]bool
	starts  map[string][]time.Time
}

// enter returns a release func, or false when the user already has this kind
// of read running or started too many in the last minute.
func (g *userGate) enter(user, kind string, perMinute int) (func(), bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	key := kind + "/" + user
	now := time.Now()
	recent := g.starts[key][:0]
	for _, t := range g.starts[key] {
		if now.Sub(t) < time.Minute {
			recent = append(recent, t)
		}
	}
	if g.running[key] || len(recent) >= perMinute {
		g.starts[key] = recent
		return nil, false
	}
	g.running[key] = true
	g.starts[key] = append(recent, now)
	return func() {
		g.mu.Lock()
		delete(g.running, key)
		g.mu.Unlock()
	}, true
}

// capWriter fails once more than max bytes are written.
type capWriter struct {
	buf bytes.Buffer
	max int64
}

var errTooBig = errors.New("bundle too large")

func (w *capWriter) Write(p []byte) (int, error) {
	if int64(w.buf.Len()+len(p)) > w.max {
		return 0, errTooBig
	}
	return w.buf.Write(p)
}

var pinRE = regexp.MustCompile(`^[0-9a-f]{64}$`)

// chainRange reads ?from=&to= as dates (to inclusive) in the site's time zone.
func chainRange(c *fiber.Ctx, site *model.Site) (from, to time.Time, err error) {
	if c.Query("from") == "" || c.Query("to") == "" {
		return from, to, service.BadRequest("range_required", "from and to are required (YYYY-MM-DD)")
	}
	loc := site.Location()
	f, err1 := time.ParseInLocation("2006-01-02", c.Query("from"), loc)
	t, err2 := time.ParseInLocation("2006-01-02", c.Query("to"), loc)
	if err1 != nil || err2 != nil {
		return from, to, service.BadRequest("invalid_time", "use YYYY-MM-DD")
	}
	t = t.AddDate(0, 0, 1)
	if !t.After(f) {
		return from, to, service.BadRequest("invalid_time", "to must not be before from")
	}
	if t.Sub(f) > chainMaxDays*24*time.Hour+time.Hour {
		return from, to, service.BadRequest("range_too_long", "date range is limited to %d days", chainMaxDays)
	}
	return f, t, nil
}

// chainGate takes the caller's per-user turn and one of the slots of a kind of
// chain read; release must be called when the read is done. 429 verify_busy otherwise.
func (h *Handler) chainGate(c *fiber.Ctx, kind string, slots chan struct{}) (release func(), err error) {
	user, _ := c.Locals("userID").(string)
	releaseUser, ok := chainUsers.enter(user, kind, ChainLimits.PerUserPerMinute)
	if !ok {
		return nil, &service.Error{Status: fiber.StatusTooManyRequests, Code: "verify_busy", Msg: "you have one running or started too many in the last minute, try again in a minute"}
	}
	select {
	case slots <- struct{}{}:
		return func() { <-slots; releaseUser() }, nil
	default:
		releaseUser()
		return nil, &service.Error{Status: fiber.StatusTooManyRequests, Code: "verify_busy", Msg: "another verification is running, try again in a minute"}
	}
}

// chainSnapshot begins one read-only repeatable-read transaction with a statement timeout.
func (h *Handler) chainSnapshot(c *fiber.Ctx, timeout time.Duration) (pgx.Tx, error) {
	ctx := c.Context()
	tx, err := h.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, fmt.Sprintf(`SET LOCAL statement_timeout = %d`, timeout.Milliseconds())); err != nil {
		_ = tx.Rollback(ctx)
		return nil, err
	}
	return tx, nil
}

// exportErr turns an export failure into the API's answer.
func exportErr(err error) error {
	var big *chainproof.TooLargeError
	switch {
	case errors.As(err, &big):
		return service.BadRequest("verify_too_large", "the range needs %d chain entries (limit %d): use a shorter range, or export the bundle and check it offline with garda-verify", big.Entries, big.Limit)
	case chainproof.IsTimeout(err):
		return service.BadRequest("verify_too_large", "the check took too long: use a shorter range, or export the bundle and check it offline with garda-verify")
	}
	return err
}

// exportChain reads the bundle in one read-only snapshot, with its own timeout.
//
// With sign set (the exports that leave the system, not the online check) the
// signer is asked for a fresh head first, outside the snapshot, so the
// snapshot sees it and the bundle ends on a signed head. When the signer does
// not sign, the bundle is still made and says why in its manifest.
func (h *Handler) exportChain(c *fiber.Ctx, kind string, slots chan struct{}, site *model.Site, from, to time.Time, limit int64, timeout time.Duration, sign bool) (*chainproof.Bundle, error) {
	release, err := h.chainGate(c, kind, slots)
	if err != nil {
		return nil, err
	}
	defer release()
	var signed signer.Outcome
	if sign {
		signed = h.signNow(c.Context(), site.ID)
	}
	tx, err := h.chainSnapshot(c, timeout)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(c.Context()) }()
	b, err := chainproof.Export(c.Context(), tx, site.ID, site.Timezone, from, to, time.Now(), limit)
	if err != nil {
		return nil, exportErr(err)
	}
	if sign {
		b.NoteSigning(signed)
	}
	return b, nil
}

// signNow asks the signer for a fresh head of the site; no signer configured
// is an outcome too, never an error.
func (h *Handler) signNow(ctx context.Context, siteID string) signer.Outcome {
	if h.Signer == nil {
		return signer.Outcome{Status: signer.OutcomeNotSetup, Reason: signer.ReasonUnconfigured, Detail: "no SIGNER_URL"}
	}
	o := h.Signer.SignNow(ctx, siteID)
	if !o.Signed() {
		h.Log.Warn().Str("site_id", siteID).Str("outcome", o.Status).Str("reason", o.Reason).Str("detail", o.Detail).Msg("export: no fresh signed head")
	}
	return o
}

// GET /sites/:id/chain/verify?from=&to=&pin=
// Checks the site's record chain and signed heads for a date range (dates in
// the site's time zone, to inclusive). pin (repeatable) = signer key
// fingerprints the caller holds; without one the first key of the keyring is
// trusted on first use, and valid is null instead of true (trust: unverified_trust; valid_pinned says
// a pinned check passed). Always 200 with a result; first_break names the first
// broken record. Returns no record content.
func (h *Handler) verifyChain(c *fiber.Ctx) error {
	site, err := h.siteFor(c, c.Params("id"))
	if err != nil {
		return h.fail(c, err)
	}
	from, to, err := chainRange(c, site)
	if err != nil {
		return h.fail(c, err)
	}
	var pins []string
	for _, p := range c.Context().QueryArgs().PeekMulti("pin") {
		pin := strings.ToLower(string(p))
		if !pinRE.MatchString(pin) {
			return h.fail(c, service.BadRequest("invalid_pin", "pin must be a 64 character hex key fingerprint"))
		}
		pins = append(pins, pin)
	}
	b, err := h.exportChain(c, "verify", verifySlots, site, from, to, ChainLimits.VerifyMaxEntries, ChainLimits.VerifyTimeout, false)
	if err != nil {
		return h.fail(c, err)
	}
	res := chainproof.Verify(b, chainproof.Options{Pins: pins, MaxAge: verifyMaxAge, NoRangeCheck: true /* the range is this request, read by the server itself */})
	// Without a pin the keyring that vouches for the heads comes from the same
	// server as the chain: a pass proves nothing to the caller, so it is not
	// shown as true. Pass pin (the fingerprint you were given) for a real answer.
	// valid is true only for a pinned pass and false only for a break; a chain
	// that holds but is not proven (unsigned tail, unverified records, no
	// entries, no pin) is null, and summary.state says which.
	pinned := len(pins) > 0
	sum := chainproof.Summarize(res, pinned)
	trust, valid := "pinned", any(nil)
	if !pinned {
		trust = "unverified_trust"
	}
	switch sum.State {
	case chainproof.StateValidPinned:
		valid = true
	case chainproof.StateBroken:
		valid = false
	}
	return c.JSON(fiber.Map{
		"valid": valid, "valid_pinned": pinned && res.Status == chainproof.StatusValid, "status": res.Status, "first_break": res.Break,
		"summary": sum,
		"site_id": site.ID, "from": c.Query("from"), "to": c.Query("to"), "trust": trust,
		"checked": fiber.Map{"entries": res.Entries, "heads": res.Heads, "from_seq": res.FirstSeq, "to_seq": res.LastSeq,
			"anchor_seq": res.AnchorSeq, "newest_head_seq": res.NewestHeadSeq, "records_not_checked": res.RecordsNotChecked},
	})
}

// GET /sites/:id/chain/export?from=&to=
// The bundle for garda-verify (zip). Holds the text of the records in the
// range, so it is limited to admin and supervisor.
func (h *Handler) exportChainBundle(c *fiber.Ctx) error {
	site, err := h.siteFor(c, c.Params("id"))
	if err != nil {
		return h.fail(c, err)
	}
	from, to, err := chainRange(c, site)
	if err != nil {
		return h.fail(c, err)
	}
	audit := func(result string, bytes int64, signing, detail string) error {
		return h.auditExport(c, site, repository.ExportBundle, from, to, bytes, result, signing, detail)
	}
	b, err := h.exportChain(c, "export", exportSlots, site, from, to, ChainLimits.ExportMaxEntries, ChainLimits.ExportTimeout, true)
	if err != nil {
		h.auditFailure(c, site, repository.ExportBundle, from, to, err)
		return h.fail(c, err)
	}
	buf := &capWriter{max: ChainLimits.ExportMaxBytes}
	if err := b.Write(buf); err != nil {
		if errors.Is(err, errTooBig) {
			err = service.BadRequest("verify_too_large", "the bundle would be larger than %d MB: use a shorter range", ChainLimits.ExportMaxBytes>>20)
		}
		h.auditFailure(c, site, repository.ExportBundle, from, to, err)
		return h.fail(c, err)
	}
	// The export is recorded before it leaves: no row, no download.
	if err := audit(repository.ExportOK, int64(buf.buf.Len()), b.Manifest.Signing, b.Manifest.SigningReason); err != nil {
		return h.fail(c, err)
	}
	name := fmt.Sprintf("garda-chain-%s-%s_%s.zip", strings.SplitN(site.ID, "-", 2)[0], c.Query("from"), c.Query("to"))
	c.Set(fiber.HeaderContentType, "application/zip")
	c.Set(fiber.HeaderContentDisposition, `attachment; filename="`+name+`"`)
	c.Set(fiber.HeaderCacheControl, "no-store")
	c.Set("X-Bundle-SHA256", b.ManifestSHA256)
	c.Set("X-Bundle-Signing", b.Manifest.Signing)
	return c.Send(buf.buf.Bytes())
}

// auditExport appends the export_audit row of an export by the caller.
func (h *Handler) auditExport(c *fiber.Ctx, site *model.Site, kind string, from, to time.Time, bytes int64, result, signing, detail string) error {
	p := middleware.GetPrincipal(c)
	actor := p.Username
	if actor == "" {
		actor = p.Subject
	}
	return repository.InsertExportAudit(c.Context(), h.DB, repository.ExportAuditNew{
		Realm: p.Realm, SiteID: site.ID, Actor: actor, ActorSub: p.Subject, Kind: kind, From: from, To: to,
		Bytes: bytes, Result: result, Signing: signing, Detail: detail})
}

// auditFailure records an export that was tried and did not produce a file. A
// request turned away at the gate (verify_busy: slot or rate limit) is not an
// export attempt and is not recorded: the rows are append-only, and a loop of
// rejected requests must not be able to fill them. A failure of the audit
// itself is logged (the caller already answers with an error).
func (h *Handler) auditFailure(c *fiber.Ctx, site *model.Site, kind string, from, to time.Time, cause error) {
	result := repository.ExportFailed
	var se *service.Error
	if errors.As(cause, &se) && se.Code == "verify_busy" {
		return
	}
	if errors.As(cause, &se) && se.Code == "verify_too_large" {
		result = repository.ExportTooLarge
	}
	detail := "error"
	if se != nil {
		detail = se.Code
	}
	if err := h.auditExport(c, site, kind, from, to, 0, result, "", detail); err != nil {
		h.Log.Error().Err(err).Str("site_id", site.ID).Msg("export audit row not written")
	}
}

// GET /sites/:id/chain/exports?actor=&kind=&include_system=&limit=&offset=
// The export audit of the site: who exported a bundle or proof pack, for
// which range, when, how big, and how the head was signed. Admin and
// supervisor of the site's realm; no content of the exports. Rows of the
// monthly job (kind scheduled, actor system) are left out unless kind=scheduled
// or include_system=true.
func (h *Handler) listExports(c *fiber.Ctx) error {
	site, err := h.siteFor(c, c.Params("id"))
	if err != nil {
		return h.fail(c, err)
	}
	limit, _ := strconv.Atoi(c.Query("limit", "100"))
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	offset, _ := strconv.Atoi(c.Query("offset", "0"))
	if offset < 0 || offset > 100_000 {
		return h.fail(c, service.BadRequest("invalid_offset", "offset must be between 0 and 100000"))
	}
	kind := c.Query("kind")
	switch kind {
	case "", repository.ExportBundle, repository.ExportProofPack, repository.ExportScheduled:
	default:
		return h.fail(c, service.BadRequest("invalid_kind", "kind must be bundle, proof_pack or scheduled"))
	}
	rows, err := repository.ListExportAudit(c.Context(), h.DB, repository.ExportAuditFilter{
		Realm: middleware.GetPrincipal(c).Realm, SiteID: site.ID, Actor: c.Query("actor"), Kind: kind,
		IncludeSystem: c.Query("include_system") == "true", Limit: limit + 1, Offset: offset})
	if err != nil {
		return h.fail(c, err)
	}
	more := len(rows) > limit
	if more {
		rows = rows[:limit]
	}
	return c.JSON(fiber.Map{"rows": rows, "limit": limit, "offset": offset, "has_more": more})
}
