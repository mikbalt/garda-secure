package middleware

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog"
)

// Logger returns a Fiber middleware that emits a structured zerolog entry per request.
func Logger(log zerolog.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()
		err := c.Next()
		if err != nil {
			// Render handler errors (incl. recovered panics) now, so the
			// logged status is the one the client gets (audit BE-O1).
			if herr := c.App().ErrorHandler(c, err); herr != nil {
				_ = c.SendStatus(fiber.StatusInternalServerError)
			}
			err = nil
		}

		status := c.Response().StatusCode()
		dur := time.Since(start)

		event := log.Info()
		if status >= 500 {
			event = log.Error()
		} else if status >= 400 {
			event = log.Warn()
		}

		userID, _ := c.Locals("userID").(string)
		traceID, _ := c.Locals("requestid").(string)

		event.
			Str("trace_id", traceID).
			Str("method", c.Method()).
			Str("path", c.Path()).
			Int("status", status).
			Int64("duration_ms", dur.Milliseconds()).
			Str("user_id", userID).
			Str("ip", c.IP()).
			Msg("request")

		return err
	}
}
