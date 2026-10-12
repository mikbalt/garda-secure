package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"

	"github.com/securepatrol/backend/internal/model"
	"github.com/securepatrol/backend/internal/repository"
)

// Limits of what an audit row stores.
const (
	auditSummaryMax = 4 << 10
	auditStateMax   = 16 << 10
	auditUAMax      = 300
)

// localsAuditBefore holds the JSON "before" state a handler recorded with
// SetAuditBefore.
const localsAuditBefore = "audit.before"

// localsAuditGuard holds the guard a request concerned without acting as
// them (e.g. a server PIN check), set with SetAuditGuard.
const localsAuditGuard = "audit.guard"

// SetAuditGuard records the guard a request is about (e.g. a failed PIN
// check) in its audit row when no guard is acting.
func SetAuditGuard(c *fiber.Ctx, guardID string) {
	c.Locals(localsAuditGuard, guardID)
}

// SetAuditBefore records the state of the resource before a change. v is
// marshalled immediately, so later mutation of v does not matter. The
// response body becomes the "after" state when the request succeeds.
func SetAuditBefore(c *fiber.Ctx, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	c.Locals(localsAuditBefore, b)
}

// Audit writes an audit_log row for every mutating request (POST, PUT,
// PATCH, DELETE) once the handler has finished. Everything taken from the
// request/response buffers is copied before use, and the row is written
// synchronously, so nothing outlives the request context. Must run after JWT
// and LoadDevice. Web writes by staff roles also store a redacted copy of
// the JSON body.
func Audit(pool *pgxpool.Pool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		switch c.Method() {
		case fiber.MethodPost, fiber.MethodPut, fiber.MethodPatch, fiber.MethodDelete:
		default:
			return c.Next()
		}
		p := GetPrincipal(c)
		var summary []byte
		if p != nil && p.HasRole(RoleAdmin, RoleSupervisor, RoleHR, RolePlatformAdmin) &&
			strings.HasPrefix(c.Get(fiber.HeaderContentType), fiber.MIMEApplicationJSON) && len(c.Body()) > 0 {
			summary = RedactJSON(c.Body(), auditSummaryMax)
		}

		err := c.Next()

		a := model.AuditLog{Summary: summary, Roles: []string{}}
		a.Method = ptr(strings.Clone(c.Method()))
		a.Route = ptr(strings.Clone(c.Route().Path))
		a.Action = *a.Method + " " + *a.Route
		a.ResourceType = ptr(resourceType(*a.Route))
		if p != nil {
			a.Realm, a.Actor = ptr(strings.Clone(p.Realm)), ptr(strings.Clone(p.Actor()))
			a.Roles = append(a.Roles, p.Roles...)
			a.UserID = *a.Realm + "/" + *a.Actor
		}
		if g := GetGuard(c); g != nil {
			a.GuardID = ptr(strings.Clone(g.ID))
			a.UserID += " guard:" + *a.GuardID
		} else if id, isStr := c.Locals(localsAuditGuard).(string); isStr && id != "" {
			a.GuardID = ptr(strings.Clone(id))
			a.UserID += " guard:" + *a.GuardID
		}
		if names := c.Route().Params; len(names) > 0 {
			params := map[string]string{}
			for _, n := range names {
				params[n] = strings.Clone(c.Params(n))
			}
			a.Params, _ = json.Marshal(params)
			a.ResourceID = ptr(params[names[0]])
		}
		a.StatusCode = c.Response().StatusCode()
		if err != nil { // the error handler sets the status after the chain returns
			a.StatusCode = fiber.StatusInternalServerError
			var fe *fiber.Error
			if errors.As(err, &fe) {
				a.StatusCode = fe.Code
			}
		}
		ok := a.StatusCode >= 200 && a.StatusCode < 300
		respJSON := strings.HasPrefix(string(c.Response().Header.ContentType()), fiber.MIMEApplicationJSON)
		if ok && respJSON && a.ResourceID == nil && *a.Method == fiber.MethodPost {
			var created struct {
				ID string `json:"id"`
			}
			if json.Unmarshal(c.Response().Body(), &created) == nil && created.ID != "" {
				a.ResourceID = ptr(created.ID)
			}
		}
		if before, isSet := c.Locals(localsAuditBefore).([]byte); isSet {
			a.Before = before
			if ok && respJSON && len(c.Response().Body()) <= auditStateMax {
				a.After = bytes.Clone(c.Response().Body())
			}
		}
		if rid, isStr := c.Locals("requestid").(string); isStr && rid != "" {
			a.RequestID = ptr(strings.Clone(rid))
		}
		a.IP = ClientIP(c)
		a.UserAgent = strings.Clone(truncate(c.Get(fiber.HeaderUserAgent), auditUAMax))

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if werr := repository.InsertAudit(ctx, pool, a); werr != nil {
			log.Error().Err(werr).Str("action", a.Action).Msg("audit log write failed")
		}
		return err
	}
}

// resourceType names the resource of a route: its first two static path
// segments below /api/v1, e.g. "/api/v1/patrol/sessions/:id/scans" →
// "patrol.sessions", "/api/v1/guards/:id/pin" → "guards".
func resourceType(route string) string {
	route = strings.TrimPrefix(route, "/api/v1/")
	var parts []string
	for _, seg := range strings.Split(route, "/") {
		if seg == "" || strings.HasPrefix(seg, ":") {
			break
		}
		parts = append(parts, seg)
		if len(parts) == 2 {
			break
		}
	}
	return strings.Join(parts, ".")
}

// sensitiveKeys are JSON keys whose values never reach the audit log.
var sensitiveKeys = []string{"password", "token", "secret", "pin", "authorization", "api_key", "apikey", "credential", "id_number"}

// RedactJSON returns a compact copy of a JSON document with secret values
// replaced by "[redacted]". Bodies over max bytes are reduced to their
// top-level keys; non-JSON bodies to their size.
func RedactJSON(body []byte, max int) []byte {
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		out, _ := json.Marshal(map[string]any{"_non_json_bytes": len(body)})
		return out
	}
	v = redact(v)
	out, err := json.Marshal(v)
	if err == nil && len(out) <= max {
		return out
	}
	keys := []string{}
	if m, isMap := v.(map[string]any); isMap {
		for k := range m {
			keys = append(keys, k)
		}
	}
	out, _ = json.Marshal(map[string]any{"_truncated_bytes": len(body), "keys": keys})
	return out
}

func redact(v any) any {
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			lk := strings.ToLower(k)
			hit := false
			for _, s := range sensitiveKeys {
				if strings.Contains(lk, s) {
					hit = true
					break
				}
			}
			if hit {
				t[k] = "[redacted]"
			} else {
				t[k] = redact(val)
			}
		}
		return t
	case []any:
		for i := range t {
			t[i] = redact(t[i])
		}
		return t
	}
	return v
}

func ptr(s string) *string { return &s }

// truncate cuts s to at most n bytes of valid UTF-8 (the database rejects
// invalid sequences).
func truncate(s string, n int) string {
	if len(s) > n {
		s = s[:n]
	}
	return strings.ToValidUTF8(s, "")
}
