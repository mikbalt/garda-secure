package middleware

import (
	"context"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
)

func TestLimiterBurstAndRecovery(t *testing.T) {
	mr := miniredis.RunT(t)
	l := NewLimiter(redis.NewClient(&redis.Options{Addr: mr.Addr()}), zerolog.Nop())
	now := time.Unix(1_800_000_000, 0)
	l.now = func() time.Time { return now }
	ctx := context.Background()

	if ok, _ := l.Peek(ctx, "k", 3); !ok {
		t.Fatal("peek denied on a fresh key")
	}
	for i := 0; i < 3; i++ {
		if ok, _ := l.Allow(ctx, "k", 3); !ok {
			t.Fatalf("request %d denied within burst", i+1)
		}
	}
	ok, retry := l.Allow(ctx, "k", 3)
	if ok || retry <= 0 || retry > 20*time.Second {
		t.Fatalf("4th request: ok=%v retry=%v, want denied with retry ≤ 20s", ok, retry)
	}
	if ok, _ := l.Peek(ctx, "k", 3); ok {
		t.Fatal("peek allowed while exhausted")
	}
	if ok, _ := l.Allow(ctx, "other", 3); !ok {
		t.Fatal("keys are not independent")
	}
	now = now.Add(21 * time.Second) // one emission interval later
	if ok, _ := l.Allow(ctx, "k", 3); !ok {
		t.Fatal("not allowed again after the interval")
	}
	if ok, _ := l.Allow(ctx, "k", 0); !ok {
		t.Fatal("limit 0 must disable limiting")
	}
}

func TestLimiterFailsOpen(t *testing.T) {
	mr := miniredis.RunT(t)
	l := NewLimiter(redis.NewClient(&redis.Options{Addr: mr.Addr()}), zerolog.Nop())
	mr.Close()
	if ok, _ := l.Allow(context.Background(), "k", 1); !ok {
		t.Fatal("limiter must fail open when Redis is down")
	}
}

func TestLimitMiddlewares(t *testing.T) {
	mr := miniredis.RunT(t)
	l := NewLimiter(redis.NewClient(&redis.Options{Addr: mr.Addr()}), zerolog.Nop())
	app := fiber.New()
	app.Use(LimitAuthFailures(l, 2))
	app.Use(func(c *fiber.Ctx) error {
		if c.Get("Authorization") == "" {
			return c.SendStatus(fiber.StatusUnauthorized)
		}
		return c.Next()
	})
	app.All("/*", func(c *fiber.Ctx) error { return c.SendString("ok") })

	do := func(method, path, auth, body string) (int, string) {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		if auth != "" {
			req.Header.Set("Authorization", auth)
		}
		res, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(res.Body)
		return res.StatusCode, res.Header.Get("Retry-After") + " " + string(b)
	}

	// Failed authentications: two 401s, then the IP is throttled.
	do("GET", "/x", "", "")
	do("GET", "/x", "", "")
	if st, b := do("GET", "/x", "", ""); st != 429 || !strings.Contains(b, "rate_limited") {
		t.Fatalf("after 2 auth failures: %d %s", st, b)
	}

	app2 := fiber.New()
	app2.Use(func(c *fiber.Ctx) error {
		c.Locals("principal", &Principal{Realm: "r", Subject: "dev1", Roles: []string{RoleDevice}})
		return c.Next()
	})
	app2.Use(LimitJSONBody(16, "/media"))
	app2.Use(LimitWrites(l, 100, 1))
	app2.All("/*", func(c *fiber.Ctx) error { return c.SendString("ok") })
	app = app2

	if st, b := do("POST", "/x", "", strings.Repeat("a", 17)); st != 413 || !strings.Contains(b, "body_too_large") {
		t.Fatalf("big JSON body: %d %s", st, b)
	}
	if st, _ := do("GET", "/x", "", ""); st != 200 {
		t.Fatalf("reads are not write-limited: %d", st)
	}
	if st, _ := do("POST", "/media", "", strings.Repeat("a", 17)); st != 200 {
		t.Fatalf("media upload exempt from JSON limit: %d", st)
	}
	st, b := do("POST", "/x", "", "{}")
	if st != 429 || !strings.HasPrefix(b, "60 ") && !strings.HasPrefix(b, "59 ") {
		t.Fatalf("second device write within a minute (limit 1): %d %q", st, b)
	}
}
