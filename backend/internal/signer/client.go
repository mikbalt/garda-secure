package signer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// What came of asking the signer for a fresh head (Outcome.Status).
const (
	OutcomeSigned      = "signed"         // the signer signed the current head now
	OutcomeReused      = "reused"         // a head signed a few seconds ago was reused (rate limit)
	OutcomeUnreachable = "unreachable"    // no answer: the bundle may end on an unsigned tail
	OutcomeRefused     = "refused"        // the signer answered and did not sign (its refusal rules held)
	OutcomeNotSetup    = "not_configured" // this install has no signer address
)

// Fixed reasons of an Outcome that did not sign. They go into the bundle's
// manifest, the verifier output and the export audit, which outsiders read, so
// they never carry the signer's own words (those are in Detail, for the log).
const (
	ReasonUnreachable  = "signer_unreachable"
	ReasonRefused      = "signer_refused"
	ReasonUnconfigured = "signer_unconfigured"
	ReasonError        = "signer_error"
)

// Outcome is the result of SignNow. Reason is one of the fixed codes above, and
// empty when the head was signed. Detail is the raw text of the failure: for
// server logs only, never for a bundle, a response or the audit.
type Outcome struct {
	Status string
	Reason string
	Detail string
	Seq    int64
}

// Signed reports whether a head covering the chain's end now exists.
func (o Outcome) Signed() bool { return o.Status == OutcomeSigned || o.Status == OutcomeReused }

// HeadSigner asks for a fresh signed head of a site before an export.
type HeadSigner interface {
	SignNow(ctx context.Context, siteID string) Outcome
}

// ReuseWindow is how long a head signed for one export is reused for the next
// export of the same site, so that a burst of exports does not make a signed
// head each (the entries of those seconds may then still be an unsigned tail).
const ReuseWindow = 10 * time.Second

// FailWindow is how long a failure is remembered: a dead signer would
// otherwise hold an export slot for the whole request timeout on every export.
// An unreachable or failing signer is remembered for all sites, a refusal for
// the site it was about.
const FailWindow = 5 * time.Second

// Client calls POST /sign of the signer service (internal network only).
type Client struct {
	base    string
	http    *http.Client
	now     func() time.Time
	mu      sync.Mutex
	lastOK  map[string]time.Time
	lastSeq map[string]int64
	down    Outcome // the last unreachable/failed answer, any site
	downAt  time.Time
	refused map[string]Outcome // the last refusal per site
	refAt   map[string]time.Time
}

// NewClient returns a client for the signer at base (for instance
// http://signer:9092). timeout bounds one request.
func NewClient(base string, timeout time.Duration) *Client {
	return &Client{base: strings.TrimRight(base, "/"), http: &http.Client{Timeout: timeout}, now: time.Now,
		lastOK: map[string]time.Time{}, lastSeq: map[string]int64{},
		refused: map[string]Outcome{}, refAt: map[string]time.Time{}}
}

// SignNow asks the signer to sign the current head of the site. It never
// returns an error: every failure is an Outcome the export records. The
// signer's own rules (no lower seq, no other hash at a signed seq, links hold)
// decide; a refusal is passed on as it is, never retried or worked around.
func (c *Client) SignNow(ctx context.Context, siteID string) Outcome {
	id, err := uuid.Parse(siteID)
	if err != nil {
		return Outcome{Status: OutcomeRefused, Reason: ReasonError, Detail: "site id is not a uuid"}
	}
	siteID = id.String()
	now := c.now()
	c.mu.Lock()
	at, ok := c.lastOK[siteID]
	seq := c.lastSeq[siteID]
	down, downAt := c.down, c.downAt
	ref, refAt := c.refused[siteID], c.refAt[siteID]
	c.mu.Unlock()
	switch {
	case ok && now.Sub(at) < ReuseWindow:
		return Outcome{Status: OutcomeReused, Seq: seq}
	case down.Status != "" && now.Sub(downAt) < FailWindow:
		return down
	case ref.Status != "" && now.Sub(refAt) < FailWindow:
		return ref
	}
	o := c.ask(ctx, siteID)
	c.mu.Lock()
	switch o.Status {
	case OutcomeSigned:
		c.lastOK[siteID], c.lastSeq[siteID] = now, o.Seq
		c.down = Outcome{}
	case OutcomeUnreachable:
		c.down, c.downAt = o, now
	case OutcomeRefused:
		c.refused[siteID], c.refAt[siteID] = o, now
	}
	c.mu.Unlock()
	return o
}

func (c *Client) ask(ctx context.Context, siteID string) Outcome {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/sign?site_id="+url.QueryEscape(siteID), nil)
	if err != nil {
		return Outcome{Status: OutcomeNotSetup, Reason: ReasonUnconfigured, Detail: "the signer address is not valid"}
	}
	resp, err := c.http.Do(req) // #nosec G704 -- the address is the operator's SIGNER_URL
	if err != nil {
		return Outcome{Status: OutcomeUnreachable, Reason: ReasonUnreachable, Detail: err.Error()}
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode != http.StatusOK {
		detail := fmt.Sprintf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		if resp.StatusCode >= 500 {
			return Outcome{Status: OutcomeUnreachable, Reason: ReasonError, Detail: detail}
		}
		return Outcome{Status: OutcomeRefused, Reason: ReasonRefused, Detail: detail}
	}
	var h struct {
		Seq int64 `json:"seq"`
	}
	if err := json.Unmarshal(body, &h); err != nil || h.Seq < 1 {
		return Outcome{Status: OutcomeRefused, Reason: ReasonError, Detail: "the signer's answer was not understood"}
	}
	return Outcome{Status: OutcomeSigned, Seq: h.Seq}
}

// SignHandler serves POST /sign?site_id= for s: it signs the current head of
// one site (reason on_demand) and answers with the head. Refusals are 409 and
// sign nothing.
func SignHandler(s *Signer, logf func(msg, site string, err error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		site := r.URL.Query().Get("site_id")
		if _, err := uuid.Parse(site); err != nil {
			http.Error(w, "site_id must be a uuid", http.StatusBadRequest)
			return
		}
		h, err := s.SignSite(r.Context(), site, ReasonOnDemand)
		switch {
		case errors.Is(err, ErrNoChain):
			http.Error(w, err.Error(), http.StatusNotFound)
		case err != nil:
			if logf != nil {
				logf("on-demand sign refused", site, err)
			}
			http.Error(w, err.Error(), http.StatusConflict)
		default:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"site_id": h.SiteID, "seq": h.Seq,
				"entry_hash": fmt.Sprintf("%x", h.EntryHash), "signed_at": h.SignedAt, "key_id": h.KeyID,
				"signature": fmt.Sprintf("%x", h.Signature)})
		}
	}
}
