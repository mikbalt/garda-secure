package notification

import (
	"errors"
	"fmt"
	"net/http"
	"slices"
	"sort"
	"strings"

	"github.com/securepatrol/backend/internal/settings"
)

// ChannelSpec registers a delivery channel (audit F10). Rules, ops rules and
// the test endpoint validate channel names against this registry instead of
// a database CHECK, so a new channel (LINE, Telegram, ...) is one Go file
// that calls RegisterChannel from init() plus its settings struct.
type ChannelSpec struct {
	Name string
	// New builds the sender used by the worker and the test endpoint.
	New func() Channel
	// Configured reports whether the effective settings allow sending.
	Configured func(settings.Channels) bool
	// NormalizeRecipient validates a recipient and returns its stored form;
	// errors wrap ErrInvalidRecipient.
	NormalizeRecipient func(string) (string, error)
}

// ErrUnknownChannel is returned for a channel name that is not registered.
var ErrUnknownChannel = errors.New("unknown channel")

// Channels is the channel registry, keyed by name.
var Channels = map[string]ChannelSpec{}

// RegisterChannel adds a channel to the registry; call it from init().
func RegisterChannel(s ChannelSpec) {
	if s.Name == "" || s.New == nil || s.NormalizeRecipient == nil {
		panic("notification: incomplete channel spec " + s.Name)
	}
	if s.Configured == nil {
		s.Configured = func(settings.Channels) bool { return true }
	}
	Channels[s.Name] = s
}

// ChannelNames lists the registered channels in a stable order.
func ChannelNames() []string {
	out := make([]string, 0, len(Channels))
	for n := range Channels {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// NormalizeRecipient validates a recipient for a channel (ErrUnknownChannel,
// ErrInvalidRecipient).
func NormalizeRecipient(channel, recipient string) (string, error) {
	spec, ok := Channels[channel]
	if !ok {
		return "", fmt.Errorf("%w %q", ErrUnknownChannel, channel)
	}
	return spec.NormalizeRecipient(strings.TrimSpace(recipient))
}

// ChannelState is one channel as listed by GET /notifications/channels.
type ChannelState struct {
	Name       string `json:"name"`
	Configured bool   `json:"configured"`
}

// ChannelStates reports every registered channel and whether the effective
// settings configure it.
func ChannelStates(cfg settings.Channels) []ChannelState {
	out := []ChannelState{}
	for _, n := range ChannelNames() {
		out = append(out, ChannelState{Name: n, Configured: Channels[n].Configured(cfg)})
	}
	return out
}

func senders() map[string]Channel {
	out := make(map[string]Channel, len(Channels))
	for n, s := range Channels {
		out[n] = s.New()
	}
	return out
}

// ── v1 channels ─────────────────────────────────────────────────────────────

func init() {
	RegisterChannel(ChannelSpec{
		Name:       "email",
		New:        func() Channel { return emailChannel{} },
		Configured: func(c settings.Channels) bool { return c.SMTP.Host != "" },
		NormalizeRecipient: func(r string) (string, error) {
			if len(r) < 3 || len(r) > 320 || !strings.Contains(r, "@") || strings.ContainsAny(r, " \r\n<>") {
				return "", fmt.Errorf("%w: recipient must be an e-mail address", ErrInvalidRecipient)
			}
			return r, nil
		},
	})
	RegisterChannel(ChannelSpec{
		Name: "whatsapp",
		New: func() Channel {
			return whatsAppChannel{client: &http.Client{
				Timeout: sendTimeout,
				// Never follow redirects: the token must only go to the
				// allow-listed API URL.
				CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
			}}
		},
		Configured: func(c settings.Channels) bool {
			return c.WhatsApp.Token != "" && c.WhatsApp.PhoneNumberID != ""
		},
		// Indonesian local format 08xx... becomes 628xx... (D-023).
		NormalizeRecipient: func(r string) (string, error) {
			digits := digitsOnly(r)
			if strings.HasPrefix(digits, "0") {
				digits = "62" + digits[1:]
			}
			if len(digits) < 8 || len(digits) > 15 {
				return "", fmt.Errorf("%w: recipient must be a phone number in international format, e.g. 62812...", ErrInvalidRecipient)
			}
			return digits, nil
		},
	})
}

// IsChannel reports whether name is a registered channel.
func IsChannel(name string) bool { return slices.Contains(ChannelNames(), name) }
