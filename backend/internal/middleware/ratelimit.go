package middleware

import (
	"context"
	"math"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
)

// gcra is a generic cell rate algorithm (token bucket) limiter keyed in
// Redis: `n` requests per minute with a burst of `n`. ARGV: now (ms),
// emission interval (ms), burst tolerance (ms), peek ("1" = check only).
// Returns {allowed (0/1), retry-after (ms)}.
var gcra = redis.NewScript(`
local now = tonumber(ARGV[1])
local interval = tonumber(ARGV[2])
local burst = tonumber(ARGV[3])
local tat = tonumber(redis.call('GET', KEYS[1]))
if not tat or tat < now then tat = now end
local allow_at = tat + interval - burst
if allow_at > now then
  return {0, math.ceil(allow_at - now)}
end
if ARGV[4] ~= "1" then
  local new_tat = tat + interval
  redis.call('SET', KEYS[1], string.format("%d", new_tat), 'PX', math.ceil(new_tat - now))
end
return {1, 0}
`)

// Limiter enforces per-key request rates in Redis. When Redis is unavailable
// it fails open (requests pass) and logs at most every 30 s.
type Limiter struct {
	rdb      *redis.Client
	log      zerolog.Logger
	lastWarn atomic.Int64
	now      func() time.Time
}

// NewLimiter creates a limiter; rdb may be nil (no limiting).
func NewLimiter(rdb *redis.Client, log zerolog.Logger) *Limiter {
	return &Limiter{rdb: rdb, log: log, now: time.Now}
}

// Allow consumes one request of key's budget of perMinute requests per
// minute. perMinute <= 0 disables the limit.
func (l *Limiter) Allow(ctx context.Context, key string, perMinute int) (bool, time.Duration) {
	return l.run(ctx, key, perMinute, false)
}

// Peek reports whether a request would be allowed, without consuming.
func (l *Limiter) Peek(ctx context.Context, key string, perMinute int) (bool, time.Duration) {
	return l.run(ctx, key, perMinute, true)
}

func (l *Limiter) run(ctx context.Context, key string, perMinute int, peek bool) (bool, time.Duration) {
	if l == nil || l.rdb == nil || perMinute <= 0 {
		return true, 0
	}
	window := time.Minute.Milliseconds()
	interval := int64(math.Max(1, float64(window)/float64(perMinute)))
	p := "0"
	if peek {
		p = "1"
	}
	ctx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	res, err := gcra.Run(ctx, l.rdb, []string{"rl:" + key},
		l.now().UnixMilli(), interval, interval*int64(perMinute), p).Int64Slice()
	if err != nil || len(res) != 2 {
		if now := time.Now().Unix(); now-l.lastWarn.Load() >= 30 {
			l.lastWarn.Store(now)
			l.log.Warn().Err(err).Msg("rate limiter unavailable; allowing requests")
		}
		return true, 0
	}
	return res[0] == 1, time.Duration(res[1]) * time.Millisecond
}

// TooManyRequests renders the 429 response with Retry-After (seconds).
func TooManyRequests(c *fiber.Ctx, retry time.Duration) error {
	secs := int(math.Ceil(retry.Seconds()))
	if secs < 1 {
		secs = 1
	}
	c.Set(fiber.HeaderRetryAfter, strconv.Itoa(secs))
	return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
		"error": "too many requests, retry later", "code": "rate_limited",
	})
}

// LimitAuthFailures throttles clients (by IP) that keep failing
// authentication: each 401 consumes one of perMinute; once exhausted, the IP
// gets 429 before its token is even parsed. Must run before JWT.
func LimitAuthFailures(l *Limiter, perMinute int) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if perMinute <= 0 {
			return c.Next()
		}
		key := "authfail:" + ClientIP(c)
		if ok, retry := l.Peek(c.Context(), key, perMinute); !ok {
			return TooManyRequests(c, retry)
		}
		err := c.Next()
		if c.Response().StatusCode() == fiber.StatusUnauthorized {
			l.Allow(c.Context(), key, perMinute)
		}
		return err
	}
}

// LimitWrites caps mutating requests per authenticated principal: tablets
// (role device) get devicePerMinute, web users userPerMinute. Must run after
// JWT.
func LimitWrites(l *Limiter, userPerMinute, devicePerMinute int) fiber.Handler {
	return func(c *fiber.Ctx) error {
		switch c.Method() {
		case fiber.MethodPost, fiber.MethodPut, fiber.MethodPatch, fiber.MethodDelete:
		default:
			return c.Next()
		}
		p := GetPrincipal(c)
		if p == nil {
			return c.Next()
		}
		key, limit := "w:user:"+p.Realm+":"+p.Subject, userPerMinute
		if p.HasRole(RoleDevice) {
			key, limit = "w:dev:"+p.Realm+":"+p.Subject, devicePerMinute
		}
		if ok, retry := l.Allow(c.Context(), key, limit); !ok {
			return TooManyRequests(c, retry)
		}
		return c.Next()
	}
}

// LimitJSONBody rejects request bodies larger than max bytes (413
// body_too_large), except for the given POST paths (photo uploads), which
// keep the server-wide limit.
func LimitJSONBody(max int, exemptPOST ...string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if c.Method() == fiber.MethodPost {
			for _, p := range exemptPOST {
				if c.Path() == p {
					return c.Next()
				}
			}
		}
		if c.Request().Header.ContentLength() > max || len(c.Body()) > max {
			return c.Status(fiber.StatusRequestEntityTooLarge).JSON(fiber.Map{
				"error": "request body too large", "code": "body_too_large",
			})
		}
		return c.Next()
	}
}

// ClientIP is the caller's IP: X-Real-IP when the request came through a
// trusted proxy (Fiber TrustedProxies), otherwise the socket address. The
// result is a copy, safe to keep after the request.
func ClientIP(c *fiber.Ctx) string {
	ip := c.IP()
	if ip == "" {
		ip = c.Context().RemoteIP().String()
	}
	return string([]byte(ip))
}
