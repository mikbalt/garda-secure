package settings

import (
	"context"
	"errors"
	"testing"

	"github.com/securepatrol/backend/internal/config"
	"github.com/securepatrol/backend/internal/service"
)

func newTestStore(t *testing.T, cfg *config.Config) *Store {
	t.Helper()
	cfg.SettingsEncryptionKey = "k"
	st, err := NewStore(nil, cfg)
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func TestAllowedWhatsAppURL(t *testing.T) {
	st := newTestStore(t, &config.Config{})
	cases := map[string]bool{
		"https://graph.facebook.com/v21.0":                 true,
		"https://graph.facebook.com":                       true,
		"https://GRAPH.facebook.com/v21.0/":                true,
		"https://graph.facebook.com.evil.example/v1":       false,
		"https://graph.facebook.com@evil.example/":         false,
		"http://graph.facebook.com/v21.0":                  false,
		"https://evil.example/https://graph.facebook.com/": false,
		"https://graph.facebook.com/v21.0?x=1":             false,
		"ftp://graph.facebook.com/":                        false,
	}
	for u, want := range cases {
		if got := st.AllowedWhatsAppURL(u); got != want {
			t.Errorf("AllowedWhatsAppURL(%q) = %v, want %v", u, got, want)
		}
	}

	onPrem := newTestStore(t, &config.Config{WhatsAppAPIURLAllowlist: []string{"http://wa-gw.internal:8080"}})
	if !onPrem.AllowedWhatsAppURL("http://wa-gw.internal:8080/v1") || onPrem.AllowedWhatsAppURL("https://graph.facebook.com/") {
		t.Error("custom allow-list not applied")
	}
	if onPrem.AllowedWhatsAppURL("http://wa-gw.internal:80800/v1") {
		t.Error("prefix must end at a path boundary")
	}
}

func TestValidateDestinations(t *testing.T) {
	st := newTestStore(t, &config.Config{SMTPHost: "relay.lan", SMTPAllowedPrivateNets: []string{"10.9.0.0/16"}})
	ctx := context.Background()
	code := func(c Channels) string {
		err := st.validate(ctx, c)
		var se *service.Error
		if errors.As(err, &se) {
			return se.Code
		}
		if err != nil {
			return err.Error()
		}
		return ""
	}
	cases := []struct {
		name string
		in   Channels
		want string
	}{
		{"public relay", Channels{SMTP: SMTP{Host: "8.8.8.8", Port: "587"}}, ""},
		{"loopback", Channels{SMTP: SMTP{Host: "127.0.0.1"}}, "smtp_host_not_allowed"},
		{"docker network", Channels{SMTP: SMTP{Host: "172.18.0.5"}}, "smtp_host_not_allowed"},
		{"allow-listed private relay", Channels{SMTP: SMTP{Host: "10.9.1.1"}}, ""},
		{"operator env relay", Channels{SMTP: SMTP{Host: "relay.lan"}}, ""},
		{"redis port", Channels{SMTP: SMTP{Host: "8.8.8.8", Port: "6379"}}, "invalid_smtp_port"},
		{"url as host", Channels{SMTP: SMTP{Host: "http://x/"}}, "invalid_smtp_host"},
		{"whatsapp elsewhere", Channels{WhatsApp: WhatsApp{APIURL: "https://evil.example/"}}, "whatsapp_url_not_allowed"},
		{"phone id", Channels{WhatsApp: WhatsApp{PhoneNumberID: "12/../x"}}, "invalid_phone_number_id"},
	}
	for _, c := range cases {
		if got := code(c.in); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}
