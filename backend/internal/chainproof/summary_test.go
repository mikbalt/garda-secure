package chainproof

import (
	"strings"
	"testing"

	"github.com/securepatrol/backend/internal/signer"
)

// Only a break looks like a break (TAQ-58).
func TestSummarizeNothingButBrokenIsBroken(t *testing.T) {
	cases := []struct {
		status string
		pinned bool
		state  string
		tone   string
	}{
		{StatusValid, true, StateValidPinned, "ok"},
		{StatusValid, false, StateUnverifiedTrust, "notice"},
		{StatusUnsignedTail, true, StateUnsignedTail, "notice"},
		{StatusUnsigned, true, StateUnsignedTail, "notice"},
		{StatusStale, true, StateUnsignedTail, "notice"},
		{StatusUnverified, true, StateUnverifiedRecs, "notice"},
		{StatusUnverifiedOK, true, StateUnverifiedRecs, "notice"},
		{StatusNoEntries, true, StateNoEntries, "notice"},
		{StatusBroken, true, StateBroken, "broken"},
		{StatusBundle, true, StateBroken, "broken"},
	}
	for _, c := range cases {
		got := Summarize(Result{Status: c.status}, c.pinned)
		if got.State != c.state || got.Tone != c.tone || got.Message == "" {
			t.Errorf("%s pinned=%v: %+v, want %s/%s", c.status, c.pinned, got, c.state, c.tone)
		}
	}
}

// The reason the signer did not sign is shown, and never changes the verdict.
func TestUnsignedTailSaysWhySigningFailed(t *testing.T) {
	f := newFixture(t, 8, 4)
	f.b.Manifest.Signing, f.b.Manifest.SigningReason = signer.OutcomeUnreachable, "the signer did not answer"
	r := f.verify(Options{})
	mustNotBeValid(t, r, StatusUnsignedTail, CodeUnsignedTail)
	if !strings.Contains(r.Break.Reason, "the signer did not answer") {
		t.Fatalf("reason %q", r.Break.Reason)
	}
	// a manifest that claims the head was signed cannot turn the tail into valid
	f.b.Manifest.Signing, f.b.Manifest.SigningReason = signer.OutcomeSigned, ""
	mustNotBeValid(t, f.verify(Options{}), StatusUnsignedTail, CodeUnsignedTail)
}

func TestUnsignedTailSaysWhenAHeadWasReused(t *testing.T) {
	f := newFixture(t, 8, 4)
	f.b.Manifest.Signing = signer.OutcomeReused
	r := f.verify(Options{})
	mustNotBeValid(t, r, StatusUnsignedTail, CodeUnsignedTail)
	if !strings.Contains(r.Break.Reason, "reused") {
		t.Fatalf("reason %q", r.Break.Reason)
	}
}
