package middleware

import (
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
)

type jwksKey struct {
	Kid string `json:"kid"`
	Kty string `json:"kty"`
	Alg string `json:"alg"`
	Use string `json:"use"`
	N   string `json:"n"`
	E   string `json:"e"`
}

type jwksResponse struct {
	Keys []jwksKey `json:"keys"`
}

// jwtKeyCache caches RSA public keys from the Keycloak JWKS endpoint.
type jwtKeyCache struct {
	mu       sync.RWMutex
	keys     map[string]*rsa.PublicKey
	jwksURL  string
	issuer   string
	fetchedAt time.Time
}

func newJWTKeyCache(jwksURL, issuer string) (*jwtKeyCache, error) {
	c := &jwtKeyCache{jwksURL: jwksURL, issuer: issuer}
	if err := c.refresh(); err != nil {
		return nil, err
	}
	// Background refresh every hour.
	go func() {
		for range time.Tick(time.Hour) {
			_ = c.refresh()
		}
	}()
	return c, nil
}

func (c *jwtKeyCache) refresh() error {
	resp, err := http.Get(c.jwksURL) //nolint:noctx
	if err != nil {
		return fmt.Errorf("jwks fetch: %w", err)
	}
	defer resp.Body.Close()

	var jwks jwksResponse
	if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		return fmt.Errorf("jwks decode: %w", err)
	}

	keys := make(map[string]*rsa.PublicKey, len(jwks.Keys))
	for _, k := range jwks.Keys {
		if k.Kty != "RSA" {
			continue
		}
		pub, err := parseRSAPublicKey(k.N, k.E)
		if err != nil {
			continue
		}
		keys[k.Kid] = pub
	}

	c.mu.Lock()
	c.keys = keys
	c.fetchedAt = time.Now()
	c.mu.Unlock()
	return nil
}

func (c *jwtKeyCache) get(kid string) (*rsa.PublicKey, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	k, ok := c.keys[kid]
	return k, ok
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
	n := new(big.Int).SetBytes(nBytes)
	e := new(big.Int).SetBytes(eBytes)
	return &rsa.PublicKey{N: n, E: int(e.Int64())}, nil
}

// JWT returns a Fiber middleware that validates Keycloak-issued RS256 JWTs.
// It sets c.Locals: "userID" (string), "username" (string), "roles" ([]string).
func JWT(jwksURL, issuer string) (fiber.Handler, error) {
	cache, err := newJWTKeyCache(jwksURL, issuer)
	if err != nil {
		return nil, fmt.Errorf("init JWT key cache: %w", err)
	}

	return func(c *fiber.Ctx) error {
		authHeader := c.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "missing or malformed Authorization header",
			})
		}
		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")

		token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
			}
			kid, _ := t.Header["kid"].(string)
			pub, ok := cache.get(kid)
			if !ok {
				// Try a refresh once in case the key rotated.
				_ = cache.refresh()
				pub, ok = cache.get(kid)
				if !ok {
					return nil, fmt.Errorf("unknown kid: %s", kid)
				}
			}
			return pub, nil
		},
			jwt.WithIssuer(issuer),
			jwt.WithExpirationRequired(),
		)

		if err != nil || !token.Valid {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid token"})
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid claims"})
		}

		sub, _ := claims["sub"].(string)
		username, _ := claims["preferred_username"].(string)

		var roles []string
		if ra, ok := claims["realm_access"].(map[string]interface{}); ok {
			if rr, ok := ra["roles"].([]interface{}); ok {
				for _, r := range rr {
					if s, ok := r.(string); ok {
						roles = append(roles, s)
					}
				}
			}
		}

		c.Locals("userID", sub)
		c.Locals("username", username)
		c.Locals("roles", roles)

		return c.Next()
	}, nil
}
