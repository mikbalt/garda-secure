package handler

import (
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog"

	"github.com/securepatrol/backend/internal/config"
	"github.com/securepatrol/backend/internal/middleware"
)

// MaxJSONBodyBytes caps every request body except photo uploads (BE-S5).
const MaxJSONBodyBytes = 64 << 10

// AppConfig is the Fiber configuration of the API server: body limit sized
// for photo uploads (JSON routes are capped lower by Mount), client IP from
// X-Real-IP only when the request comes through a trusted proxy, and an
// error handler that never leaks internal error text.
func AppConfig(cfg *config.Config, log zerolog.Logger) fiber.Config {
	return fiber.Config{
		AppName:      "SecurePatrol API v1",
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
		BodyLimit:    MaxPhotoBytes + 1<<20, // photo + multipart overhead
		// nginx sets X-Real-IP to the socket address it saw; X-Forwarded-For
		// is client-controlled and never trusted.
		ProxyHeader:             "X-Real-IP",
		EnableTrustedProxyCheck: true,
		TrustedProxies:          cfg.TrustedProxies,
		EnableIPValidation:      true,
		ErrorHandler:            ErrorHandler(log),
	}
}

// ErrorHandler renders errors that escape handlers (unknown routes, body
// too large, panics) as {"error","code"} without exposing err.Error() for
// server errors.
func ErrorHandler(log zerolog.Logger) fiber.ErrorHandler {
	return func(c *fiber.Ctx, err error) error {
		status := fiber.StatusInternalServerError
		var fe *fiber.Error
		if errors.As(err, &fe) {
			status = fe.Code
		}
		if status >= fiber.StatusInternalServerError {
			rid := requestID(c)
			log.Error().Err(err).Str("method", c.Method()).Str("path", c.Path()).
				Str("request_id", rid).Msg("unhandled error")
			body := fiber.Map{"error": "internal error", "code": "internal"}
			if rid != "" {
				body["request_id"] = rid
			}
			return c.Status(status).JSON(body)
		}
		msg, code := "request failed", "bad_request"
		switch status {
		case fiber.StatusNotFound:
			msg, code = "not found", "not_found"
		case fiber.StatusMethodNotAllowed:
			msg, code = "method not allowed", "method_not_allowed"
		case fiber.StatusRequestEntityTooLarge:
			msg, code = "request body too large", "body_too_large"
		case fiber.StatusRequestTimeout:
			msg, code = "request timeout", "timeout"
		case fiber.StatusUnauthorized:
			msg, code = "unauthorized", "unauthorized"
		case fiber.StatusForbidden:
			msg, code = "forbidden", "forbidden"
		case fiber.StatusTooManyRequests:
			msg, code = "too many requests", "rate_limited"
		case fiber.StatusUnsupportedMediaType:
			msg, code = "unsupported media type", "unsupported_media_type"
		}
		return c.Status(status).JSON(fiber.Map{"error": msg, "code": code})
	}
}

// Mount registers /api/v1 with its middleware chain: failed-auth throttling,
// JWT, device lookup, JSON body limit, per-principal write limits and the
// audit log, then every route.
func (h *Handler) Mount(app *fiber.App, v *middleware.Validator) {
	h.limiter = middleware.NewLimiter(h.Redis, h.Log)
	api := app.Group("/api/v1",
		middleware.LimitAuthFailures(h.limiter, h.Cfg.RateLimitAuthFailures),
		middleware.JWT(v),
		middleware.LoadDevice(h.DB),
		middleware.LimitJSONBody(MaxJSONBodyBytes, "/api/v1/media"),
		middleware.LimitWrites(h.limiter, h.Cfg.RateLimitUserWrites, h.Cfg.RateLimitDeviceWrites),
		middleware.Audit(h.DB),
	)
	h.Register(api)
}
