package notification

import (
	"strings"
	"testing"
)

func TestRenderLanguagesAndPlaceholders(t *testing.T) {
	params := map[string]string{"guard": "Budi", "site": "Gudang A", "time": "07 Oct 21:00"}
	subj, body := Render(EventSOS, "id", params)
	if subj != "[SOS] Budi di Gudang A" {
		t.Errorf("subject = %q", subj)
	}
	if !strings.Contains(body, "Lokasi terakhir: -") {
		t.Errorf("missing placeholder should render '-': %q", body)
	}
	subj, _ = Render(EventSOS, "en", params)
	if subj != "[SOS] Budi at Gudang A" {
		t.Errorf("english subject = %q", subj)
	}
	subj, _ = Render(EventSOS, "th", params) // unknown language → Indonesian
	if !strings.Contains(subj, " di ") {
		t.Errorf("fallback subject = %q", subj)
	}
}

func TestEveryEventHasTemplates(t *testing.T) {
	for _, lang := range []string{"id", "en"} {
		for _, ev := range EventTypes {
			if _, ok := templates[lang][ev]; !ok {
				t.Errorf("missing %s template for %s", lang, ev)
			}
		}
	}
}

func TestOneLineForWhatsAppTemplates(t *testing.T) {
	got := oneLine("a\nb\r\nc\td" + strings.Repeat(" ", 12) + "e")
	if strings.ContainsAny(got, "\n\r\t") || strings.Contains(got, "     ") {
		t.Errorf("oneLine left forbidden characters: %q", got)
	}
}
