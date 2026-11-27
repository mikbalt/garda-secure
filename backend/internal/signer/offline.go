package signer

// Offline verification (TAQ-18): the same key and head rules as Verify, on
// data that is not in a database (an export bundle). Nothing here signs.

// Trust is the verdict on a keyring: which keys a client holding pins trusts.
type Trust struct{ keys map[string]*keyTrust }

// NewTrust decides which keys of ring to trust, see verifyKeyring.
func NewTrust(ring []KeyringEntry, pins []string) *Trust {
	return &Trust{keys: verifyKeyring(ring, pins)}
}

// HeadProblem says why a signed head cannot be relied on ("" = it can): key
// unknown or not reachable from a pin, bad signature, or signed after its key
// was retired. It does not look at the chain.
func (t *Trust) HeadProblem(h SignedHead) string { return headProblem(h, t.keys) }
