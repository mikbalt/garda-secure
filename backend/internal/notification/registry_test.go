package notification

import (
	"errors"
	"slices"
	"testing"

	"github.com/securepatrol/backend/internal/settings"
)

func TestChannelRegistry(t *testing.T) {
	if got := ChannelNames(); !slices.Equal(got, []string{"email", "whatsapp"}) {
		t.Fatalf("channels = %v", got)
	}
	if _, err := NormalizeRecipient("line", "x"); !errors.Is(err, ErrUnknownChannel) {
		t.Fatalf("unknown channel: %v", err)
	}
	for _, c := range []struct{ ch, in, want string }{
		{"whatsapp", "0812-3456-789", "628123456789"},
		{"whatsapp", "+62 812 3456 789", "628123456789"},
		{"email", " ops@example.com ", "ops@example.com"},
	} {
		got, err := NormalizeRecipient(c.ch, c.in)
		if err != nil || got != c.want {
			t.Errorf("%s %q = %q, %v; want %q", c.ch, c.in, got, err, c.want)
		}
	}
	for _, c := range []struct{ ch, in string }{{"whatsapp", "123"}, {"email", "nobody"}, {"email", "a b@c"}} {
		if _, err := NormalizeRecipient(c.ch, c.in); !errors.Is(err, ErrInvalidRecipient) {
			t.Errorf("%s %q: %v, want ErrInvalidRecipient", c.ch, c.in, err)
		}
	}
	states := ChannelStates(settings.Channels{SMTP: settings.SMTP{Host: "smtp.example.com"}})
	if len(states) != 2 || !states[0].Configured || states[1].Configured {
		t.Fatalf("states = %+v", states)
	}
	if len(senders()) != len(Channels) {
		t.Fatal("every registered channel needs a sender")
	}
}
