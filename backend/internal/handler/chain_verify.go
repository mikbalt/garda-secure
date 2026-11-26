package handler

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"

	"github.com/securepatrol/backend/internal/chainproof"
	"github.com/securepatrol/backend/internal/model"
	"github.com/securepatrol/backend/internal/service"
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
}{20_000, 250_000, 15 * time.Second, 60 * time.Second}

// Two chain reads at a time per API process: they are heavy, rarely needed.
var chainSlots = make(chan struct{}, 2)

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

// exportChain reads the bundle in one read-only snapshot, with its own timeout.
func (h *Handler) exportChain(ctx context.Context, site *model.Site, from, to time.Time, limit int64, timeout time.Duration) (*chainproof.Bundle, error) {
	select {
	case chainSlots <- struct{}{}:
		defer func() { <-chainSlots }()
	default:
		return nil, &service.Error{Status: fiber.StatusTooManyRequests, Code: "verify_busy", Msg: "another verification is running, try again in a minute"}
	}
	tx, err := h.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, fmt.Sprintf(`SET LOCAL statement_timeout = %d`, timeout.Milliseconds())); err != nil {
		return nil, err
	}
	b, err := chainproof.Export(ctx, tx, site.ID, site.Timezone, from, to, time.Now(), limit)
	var big *chainproof.TooLargeError
	switch {
	case err == nil:
		return b, nil
	case errors.As(err, &big):
		return nil, service.BadRequest("verify_too_large", "the range needs %d chain entries (limit %d): use a shorter range, or export the bundle and check it offline with garda-verify", big.Entries, big.Limit)
	case chainproof.IsTimeout(err):
		return nil, service.BadRequest("verify_too_large", "the check took too long: use a shorter range, or export the bundle and check it offline with garda-verify")
	}
	return nil, err
}

// GET /sites/:id/chain/verify?from=&to=&pin=
// Checks the site's record chain and signed heads for a date range (dates in
// the site's time zone, to inclusive). pin (repeatable) = signer key
// fingerprints the caller holds; without one the first key of the keyring is
// trusted on first use. Always 200 with a result; first_break names the first
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
	b, err := h.exportChain(c.Context(), site, from, to, ChainLimits.VerifyMaxEntries, ChainLimits.VerifyTimeout)
	if err != nil {
		return h.fail(c, err)
	}
	res := chainproof.Verify(b, chainproof.Options{Pins: pins, MaxAge: verifyMaxAge})
	trust := "pinned"
	if len(pins) == 0 {
		trust = "first_use"
	}
	return c.JSON(fiber.Map{
		"valid": res.Status == chainproof.StatusValid, "status": res.Status, "first_break": res.Break,
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
	b, err := h.exportChain(c.Context(), site, from, to, ChainLimits.ExportMaxEntries, ChainLimits.ExportTimeout)
	if err != nil {
		return h.fail(c, err)
	}
	var buf bytes.Buffer
	if err := b.Write(&buf); err != nil {
		return h.fail(c, err)
	}
	name := fmt.Sprintf("garda-chain-%s-%s_%s.zip", strings.SplitN(site.ID, "-", 2)[0], c.Query("from"), c.Query("to"))
	c.Set(fiber.HeaderContentType, "application/zip")
	c.Set(fiber.HeaderContentDisposition, `attachment; filename="`+name+`"`)
	c.Set(fiber.HeaderCacheControl, "no-store")
	c.Set("X-Bundle-SHA256", b.ManifestSHA256)
	return c.Send(buf.Bytes())
}
