package middleware

import (
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/rs/zerolog"
)

// Realm roles.
const (
	RoleAdmin          = "admin"
	RoleSupervisor     = "supervisor"
	RoleHR             = "hr"
	RoleClientReadonly = "client-readonly"
	RoleDevice         = "device"
)

// Principal is the authenticated caller, derived from a validated JWT.
type Principal struct {
	Subject  string
	Username string
	Realm    string
	ClientID string
	Roles    []string
}

// HasRole reports whether the principal holds any of the given roles.
func (p *Principal) HasRole(roles ...string) bool {
	for _, r := range roles {
		if slices.Contains(p.Roles, r) {
			return true
		}
	}
	return false
}

// Actor is the human-readable identity stored in audit fields.
func (p *Principal) Actor() string {
	if p.Username != "" {
		return p.Username
	}
	return p.Subject
}

// GetPrincipal returns the principal set by the JWT middleware, or nil.
func GetPrincipal(c *fiber.Ctx) *Principal {
	p, _ := c.Locals("principal").(*Principal)
	return p
}

// RealmKeys validates tokens of one realm.
type RealmKeys struct {
	Realm   string
	Issuer  string
	JWKSURL string
}

type jwksKey struct {
	Kid string `json:"kid"`
	Kty string `json:"kty"`
	Use string `json:"use"`
	N   string `json:"n"`
	E   string `json:"e"`
}

// keyCache holds the RSA signing keys of one realm.
type keyCache struct {
	realm   RealmKeys
	client  *http.Client
	mu      sync.RWMutex
	keys    map[string]*rsa.PublicKey
	fetched time.Time
	tried   time.Time
}

// minRefreshInterval stops a flood of tokens with unknown kids from turning
// into a flood of JWKS requests.
const minRefreshInterval = 30 * time.Second

func (k *keyCache) refresh() error {
	k.mu.Lock()
	if time.Since(k.tried) < minRefreshInterval {
		k.mu.Unlock()
		return nil
	}
	k.tried = time.Now()
	k.mu.Unlock()

	resp, err := k.client.Get(k.realm.JWKSURL)
	if err != nil {
		return fmt.Errorf("jwks fetch %s: %w", k.realm.Realm, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("jwks fetch %s: status %d", k.realm.Realm, resp.StatusCode)
	}

	var body struct {
		Keys []jwksKey `json:"keys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return fmt.Errorf("jwks decode %s: %w", k.realm.Realm, err)
	}
	keys := make(map[string]*rsa.PublicKey, len(body.Keys))
	for _, jk := range body.Keys {
		if jk.Kty != "RSA" || (jk.Use != "" && jk.Use != "sig") {
			continue
		}
		pub, err := parseRSAPublicKey(jk.N, jk.E)
		if err != nil {
			continue
		}
		keys[jk.Kid] = pub
	}

	k.mu.Lock()
	k.keys = keys
	k.fetched = time.Now()
	k.mu.Unlock()
	return nil
}

func (k *keyCache) get(kid string) (*rsa.PublicKey, bool) {
	k.mu.RLock()
	defer k.mu.RUnlock()
	pub, ok := k.keys[kid]
	return pub, ok
}

func parseRSAPublicKey(nStr, eStr string) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(nStr)
	if err != nil {
		return nil, err
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(eStr)
	if err != nil {
		return nil, err
	}
	e := new(big.Int).SetBytes(eBytes)
	if !e.IsInt64() || e.Int64() > 1<<31-1 {
		return nil, errors.New("invalid RSA exponent")
	}
	return &rsa.PublicKey{N: new(big.Int).SetBytes(nBytes), E: int(e.Int64())}, nil
}

// Validator verifies Keycloak access tokens from any configured realm.
type Validator struct {
	byIssuer       map[string]*keyCache
	allowedClients []string
}

// NewValidator builds a validator and starts an hourly key refresh. Keycloak
// being unreachable at startup is not fatal: keys are fetched on first use.
func NewValidator(realms []RealmKeys, allowedClients []string, log zerolog.Logger) *Validator {
	v := &Validator{byIssuer: map[string]*keyCache{}, allowedClients: allowedClients}
	client := &http.Client{Timeout: 10 * time.Second}
	for _, r := range realms {
		kc := &keyCache{realm: r, client: client}
		if err := kc.refresh(); err != nil {
			log.Warn().Err(err).Str("realm", r.Realm).Msg("initial JWKS fetch failed; will retry on demand")
		}
		v.byIssuer[r.Issuer] = kc
	}
	go func() {
		for range time.Tick(time.Hour) {
			for _, kc := range v.byIssuer {
				kc.mu.Lock()
				kc.tried = time.Time{}
				kc.mu.Unlock()
				if err := kc.refresh(); err != nil {
					log.Warn().Err(err).Str("realm", kc.realm.Realm).Msg("JWKS refresh failed")
				}
			}
		}
	}()
	return v
}

// Parse validates a raw token and returns its principal.
func (v *Validator) Parse(tokenStr string) (*Principal, error) {
	var realm *keyCache
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		iss, _ := t.Claims.GetIssuer()
		kc, ok := v.byIssuer[iss]
		if !ok {
			return nil, fmt.Errorf("untrusted issuer %q", iss)
		}
		realm = kc
		kid, _ := t.Header["kid"].(string)
		if pub, ok := kc.get(kid); ok {
			return pub, nil
		}
		_ = kc.refresh()
		if pub, ok := kc.get(kid); ok {
			return pub, nil
		}
		return nil, fmt.Errorf("unknown kid %q", kid)
	},
		jwt.WithValidMethods([]string{"RS256", "RS384", "RS512"}),
		jwt.WithExpirationRequired(),
		jwt.WithLeeway(30*time.Second),
	)
	if err != nil || !token.Valid {
		return nil, fmt.Errorf("invalid token: %w", err)
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, errors.New("invalid claims")
	}

	p := &Principal{Realm: realm.realm.Realm}
	p.Subject, _ = claims["sub"].(string)
	p.Username, _ = claims["preferred_username"].(string)
	p.ClientID, _ = claims["azp"].(string)
	if p.Subject == "" {
		return nil, errors.New("token has no subject")
	}
	if len(v.allowedClients) > 0 && !slices.Contains(v.allowedClients, p.ClientID) {
		return nil, fmt.Errorf("token issued to client %q is not accepted", p.ClientID)
	}
	p.Roles = extractRoles(claims)
	return p, nil
}

// extractRoles reads realm roles from the flat "roles" claim (our realm
// mapper) and from Keycloak's default realm_access.roles.
func extractRoles(claims jwt.MapClaims) []string {
	var roles []string
	add := func(v any) {
		list, _ := v.([]interface{})
		for _, r := range list {
			if s, ok := r.(string); ok && !slices.Contains(roles, s) {
				roles = append(roles, s)
			}
		}
	}
	add(claims["roles"])
	if ra, ok := claims["realm_access"].(map[string]interface{}); ok {
		add(ra["roles"])
	}
	return roles
}

// JWT returns a Fiber middleware that requires a valid bearer token and sets
// the principal (plus legacy "userID"/"username"/"roles" locals).
func JWT(v *Validator) fiber.Handler {
	return func(c *fiber.Ctx) error {
		authHeader := c.Get(fiber.HeaderAuthorization)
		if !strings.HasPrefix(authHeader, "Bearer ") {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "missing or malformed Authorization header"})
		}
		p, err := v.Parse(strings.TrimPrefix(authHeader, "Bearer "))
		if err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid token"})
		}
		c.Locals("principal", p)
		c.Locals("userID", p.Subject)
		c.Locals("username", p.Username)
		c.Locals("roles", p.Roles)
		return c.Next()
	}
}

// RequireRoles allows the request only when the principal has one of roles.
func RequireRoles(roles ...string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		p := GetPrincipal(c)
		if p == nil || !p.HasRole(roles...) {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "forbidden"})
		}
		return c.Next()
	}
}
