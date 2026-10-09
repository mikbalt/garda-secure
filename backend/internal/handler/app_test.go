package handler

import (
	"errors"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog"

	"github.com/securepatrol/backend/internal/config"
)

func TestErrorHandlerHidesInternalErrors(t *testing.T) {
	app := fiber.New(AppConfig(&config.Config{}, zerolog.Nop()))
	app.Get("/boom", func(*fiber.Ctx) error { return errors.New("pq: password authentication failed for user secret") })
	app.Get("/panic", func(*fiber.Ctx) error { return fiber.NewError(fiber.StatusBadGateway, "upstream 10.0.0.5 said no") })

	for path, want := range map[string]int{"/boom": 500, "/panic": 502, "/missing": 404} {
		res, err := app.Test(httptest.NewRequest("GET", path, nil))
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(res.Body)
		if res.StatusCode != want {
			t.Errorf("%s: status %d, want %d", path, res.StatusCode, want)
		}
		if strings.Contains(string(body), "secret") || strings.Contains(string(body), "10.0.0.5") {
			t.Errorf("%s leaks error text: %s", path, body)
		}
		if !strings.Contains(string(body), `"code"`) {
			t.Errorf("%s: no error code in %s", path, body)
		}
	}
}
