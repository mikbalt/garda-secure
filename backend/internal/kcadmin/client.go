// Package kcadmin is a small Keycloak Admin REST client for user and
// device-account management from the web app (audit F1, NFR-09).
//
// Each realm has a confidential client (default securepatrol-admin-api) whose
// service account holds only realm-management view-users, query-users,
// manage-users and view-realm. Tokens are fetched with the client
// credentials grant and cached per realm until shortly before they expire.
// Secrets and tokens never appear in errors or logs.
package kcadmin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Errors returned by the client.
var (
	// ErrUnavailable: Keycloak cannot be reached, answered 5xx, or the
	// realm has no admin client secret configured.
	ErrUnavailable = errors.New("keycloak unavailable")
	ErrNotFound    = errors.New("keycloak: not found")
	ErrUsername    = errors.New("keycloak: username already exists")
	ErrEmail       = errors.New("keycloak: email already exists")
)

// BadRequestError is a 400 answer of Keycloak (e.g. password policy).
type BadRequestError struct{ Message string }

func (e *BadRequestError) Error() string { return "keycloak: " + e.Message }

// Options configures a client.
type Options struct {
	BaseURL  string            // e.g. http://keycloak:8080/auth
	ClientID string            // confidential client in every realm
	Secrets  map[string]string // realm -> client secret
	Timeout  time.Duration     // per request
}

type token struct {
	access string
	exp    time.Time
}

// Client talks to the Keycloak Admin REST API.
type Client struct {
	base     string
	clientID string
	secrets  map[string]string
	http     *http.Client

	mu     sync.Mutex
	tokens map[string]token
}

// New creates a client.
func New(o Options) *Client {
	if o.Timeout <= 0 {
		o.Timeout = 10 * time.Second
	}
	return &Client{
		base: strings.TrimRight(o.BaseURL, "/"), clientID: o.ClientID, secrets: o.Secrets,
		http: &http.Client{
			Timeout: o.Timeout,
			// The bearer token must only go to the configured Keycloak.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		tokens: map[string]token{},
	}
}

// Configured reports whether user management is set up for a realm.
func (c *Client) Configured(realm string) bool {
	return c != nil && c.base != "" && c.secrets[realm] != ""
}

func (c *Client) token(ctx context.Context, realm string) (string, error) {
	c.mu.Lock()
	t, ok := c.tokens[realm]
	c.mu.Unlock()
	if ok && time.Now().Before(t.exp) {
		return t.access, nil
	}
	secret := c.secrets[realm]
	if c.base == "" || secret == "" {
		return "", fmt.Errorf("%w: no admin client secret for realm %q", ErrUnavailable, realm)
	}
	form := url.Values{"grant_type": {"client_credentials"}, "client_id": {c.clientID}, "client_secret": {secret}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.base+"/realms/"+url.PathEscape(realm)+"/protocol/openid-connect/token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: token request: %v", ErrUnavailable, redactURLError(err))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("%w: token request for realm %q answered %d", ErrUnavailable, realm, resp.StatusCode)
	}
	var body struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body); err != nil || body.AccessToken == "" {
		return "", fmt.Errorf("%w: invalid token response", ErrUnavailable)
	}
	ttl := time.Duration(body.ExpiresIn) * time.Second
	if ttl <= 0 {
		ttl = time.Minute
	}
	ttl -= 30 * time.Second // refresh before it expires
	if ttl < 5*time.Second {
		ttl = 5 * time.Second
	}
	c.mu.Lock()
	c.tokens[realm] = token{access: body.AccessToken, exp: time.Now().Add(ttl)}
	c.mu.Unlock()
	return body.AccessToken, nil
}

func (c *Client) dropToken(realm string) {
	c.mu.Lock()
	delete(c.tokens, realm)
	c.mu.Unlock()
}

// do calls {base}/admin/realms/{realm}{path}. out may be nil. It returns the
// response headers (e.g. Location).
func (c *Client) do(ctx context.Context, realm, method, path string, in, out any) (http.Header, error) {
	var payload []byte
	if in != nil {
		var err error
		if payload, err = json.Marshal(in); err != nil {
			return nil, err
		}
	}
	for attempt := 0; ; attempt++ {
		tok, err := c.token(ctx, realm)
		if err != nil {
			return nil, err
		}
		var body io.Reader
		if payload != nil {
			body = bytes.NewReader(payload)
		}
		req, err := http.NewRequestWithContext(ctx, method, c.base+"/admin/realms/"+url.PathEscape(realm)+path, body)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+tok)
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := c.http.Do(req)
		if err != nil {
			return nil, fmt.Errorf("%w: %s %s: %v", ErrUnavailable, method, path, redactURLError(err))
		}
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		_ = resp.Body.Close()
		switch {
		case resp.StatusCode == http.StatusUnauthorized && attempt == 0:
			c.dropToken(realm) // token revoked or realm keys rotated: fetch a new one once
			continue
		case resp.StatusCode >= 200 && resp.StatusCode < 300:
			if out != nil && len(raw) > 0 {
				if err := json.Unmarshal(raw, out); err != nil {
					return nil, fmt.Errorf("%w: %s %s: invalid response", ErrUnavailable, method, path)
				}
			}
			return resp.Header, nil
		case resp.StatusCode == http.StatusNotFound:
			return nil, ErrNotFound
		case resp.StatusCode == http.StatusConflict:
			if strings.Contains(strings.ToLower(errorMessage(raw)), "email") {
				return nil, ErrEmail
			}
			return nil, ErrUsername
		case resp.StatusCode == http.StatusBadRequest:
			return nil, &BadRequestError{Message: errorMessage(raw)}
		default:
			return nil, fmt.Errorf("%w: %s %s answered %d", ErrUnavailable, method, path, resp.StatusCode)
		}
	}
}

// errorMessage extracts Keycloak's error text ({"errorMessage"} or {"error"}).
func errorMessage(raw []byte) string {
	var e struct {
		ErrorMessage string `json:"errorMessage"`
		Error        string `json:"error"`
		Description  string `json:"error_description"`
	}
	_ = json.Unmarshal(raw, &e)
	for _, s := range []string{e.ErrorMessage, e.Description, e.Error} {
		if s != "" {
			if len(s) > 200 {
				s = s[:200]
			}
			return s
		}
	}
	return "invalid request"
}

// redactURLError drops the URL (never contains secrets, but keeps logs short)
// from transport errors.
func redactURLError(err error) string {
	var ue *url.Error
	if errors.As(err, &ue) {
		return ue.Op + ": " + ue.Err.Error()
	}
	return err.Error()
}

// ── Users ────────────────────────────────────────────────────────────────────

// User is the part of Keycloak's UserRepresentation the API uses.
type User struct {
	ID               string              `json:"id,omitempty"`
	Username         string              `json:"username,omitempty"`
	Email            string              `json:"email,omitempty"`
	EmailVerified    bool                `json:"emailVerified,omitempty"`
	FirstName        string              `json:"firstName,omitempty"`
	LastName         string              `json:"lastName,omitempty"`
	Enabled          bool                `json:"enabled"`
	Totp             bool                `json:"totp,omitempty"`
	CreatedTimestamp int64               `json:"createdTimestamp,omitempty"`
	Attributes       map[string][]string `json:"attributes,omitempty"`
	RequiredActions  []string            `json:"requiredActions,omitempty"`
	Credentials      []Credential        `json:"credentials,omitempty"`
	// Raw keeps every field Keycloak returned, so an update sends back what
	// this struct does not model.
	Raw map[string]json.RawMessage `json:"-"`
}

// Credential is a credential representation (password on create, or an
// existing credential when listing).
type Credential struct {
	ID        string `json:"id,omitempty"`
	Type      string `json:"type"`
	Value     string `json:"value,omitempty"`
	Temporary bool   `json:"temporary,omitempty"`
}

// Role is a realm role.
type Role struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ListUsers returns one page of the realm's users (search matches username,
// e-mail, first and last name).
func (c *Client) ListUsers(ctx context.Context, realm, search string, first, max int) ([]User, error) {
	q := url.Values{"briefRepresentation": {"false"}, "first": {strconv.Itoa(first)}, "max": {strconv.Itoa(max)}}
	if search != "" {
		q.Set("search", search)
	}
	var users []User
	_, err := c.do(ctx, realm, http.MethodGet, "/users?"+q.Encode(), nil, &users)
	return users, err
}

// GetUser returns a user by ID, keeping the raw representation for updates.
func (c *Client) GetUser(ctx context.Context, realm, id string) (*User, error) {
	var raw map[string]json.RawMessage
	if _, err := c.do(ctx, realm, http.MethodGet, "/users/"+url.PathEscape(id), nil, &raw); err != nil {
		return nil, err
	}
	b, _ := json.Marshal(raw)
	var u User
	if err := json.Unmarshal(b, &u); err != nil {
		return nil, fmt.Errorf("%w: invalid user", ErrUnavailable)
	}
	u.Raw = raw
	return &u, nil
}

// CreateUser creates a user and returns its ID.
func (c *Client) CreateUser(ctx context.Context, realm string, u User) (string, error) {
	h, err := c.do(ctx, realm, http.MethodPost, "/users", u, nil)
	if err != nil {
		return "", err
	}
	loc := h.Get("Location")
	id := loc[strings.LastIndex(loc, "/")+1:]
	if id == "" {
		return "", fmt.Errorf("%w: user created without location", ErrUnavailable)
	}
	return id, nil
}

// DeleteUser removes a user (rollback of a half-created user).
func (c *Client) DeleteUser(ctx context.Context, realm, id string) error {
	_, err := c.do(ctx, realm, http.MethodDelete, "/users/"+url.PathEscape(id), nil, nil)
	return err
}

// UpdateUser writes the modelled fields of u onto its raw representation
// (other fields Keycloak returned are preserved) and saves it.
func (c *Client) UpdateUser(ctx context.Context, realm string, u *User) error {
	rep := map[string]any{}
	for k, v := range u.Raw {
		rep[k] = v
	}
	rep["email"], rep["firstName"], rep["lastName"], rep["enabled"] = u.Email, u.FirstName, u.LastName, u.Enabled
	rep["attributes"], rep["requiredActions"] = u.Attributes, u.RequiredActions
	if u.Attributes == nil {
		rep["attributes"] = map[string][]string{}
	}
	if u.RequiredActions == nil {
		rep["requiredActions"] = []string{}
	}
	if u.Email != "" {
		rep["emailVerified"] = u.EmailVerified
	}
	_, err := c.do(ctx, realm, http.MethodPut, "/users/"+url.PathEscape(u.ID), rep, nil)
	return err
}

// RealmRoles returns the realm roles directly mapped to a user.
func (c *Client) RealmRoles(ctx context.Context, realm, userID string) ([]Role, error) {
	var roles []Role
	_, err := c.do(ctx, realm, http.MethodGet, "/users/"+url.PathEscape(userID)+"/role-mappings/realm", nil, &roles)
	return roles, err
}

// Role looks a realm role up by name.
func (c *Client) Role(ctx context.Context, realm, name string) (*Role, error) {
	var r Role
	if _, err := c.do(ctx, realm, http.MethodGet, "/roles/"+url.PathEscape(name), nil, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// AddRealmRoles maps realm roles to a user.
func (c *Client) AddRealmRoles(ctx context.Context, realm, userID string, roles []Role) error {
	if len(roles) == 0 {
		return nil
	}
	_, err := c.do(ctx, realm, http.MethodPost, "/users/"+url.PathEscape(userID)+"/role-mappings/realm", roles, nil)
	return err
}

// RemoveRealmRoles unmaps realm roles from a user.
func (c *Client) RemoveRealmRoles(ctx context.Context, realm, userID string, roles []Role) error {
	if len(roles) == 0 {
		return nil
	}
	_, err := c.do(ctx, realm, http.MethodDelete, "/users/"+url.PathEscape(userID)+"/role-mappings/realm", roles, nil)
	return err
}

// RoleMembers returns the IDs of the users holding a realm role (directly).
func (c *Client) RoleMembers(ctx context.Context, realm, role string) ([]User, error) {
	const page = 500
	var all []User
	for first := 0; first < 50_000; first += page {
		var users []User
		q := url.Values{"first": {strconv.Itoa(first)}, "max": {strconv.Itoa(page)}, "briefRepresentation": {"false"}}
		_, err := c.do(ctx, realm, http.MethodGet, "/roles/"+url.PathEscape(role)+"/users?"+q.Encode(), nil, &users)
		if errors.Is(err, ErrNotFound) {
			return nil, nil // role does not exist in this realm
		}
		if err != nil {
			return nil, err
		}
		all = append(all, users...)
		if len(users) < page {
			break
		}
	}
	return all, nil
}

// ResetPassword sets a password (temporary = must change at next login).
func (c *Client) ResetPassword(ctx context.Context, realm, userID, password string, temporary bool) error {
	_, err := c.do(ctx, realm, http.MethodPut, "/users/"+url.PathEscape(userID)+"/reset-password",
		Credential{Type: "password", Value: password, Temporary: temporary}, nil)
	return err
}

// ExecuteActionsEmail e-mails the user a link to perform required actions
// (e.g. UPDATE_PASSWORD); needs the realm's SMTP settings.
func (c *Client) ExecuteActionsEmail(ctx context.Context, realm, userID string, actions []string, lifespan time.Duration) error {
	q := url.Values{"lifespan": {strconv.Itoa(int(lifespan.Seconds()))}}
	_, err := c.do(ctx, realm, http.MethodPut, "/users/"+url.PathEscape(userID)+"/execute-actions-email?"+q.Encode(), actions, nil)
	return err
}

// Credentials lists a user's credentials (type password, otp, ...).
func (c *Client) Credentials(ctx context.Context, realm, userID string) ([]Credential, error) {
	var creds []Credential
	_, err := c.do(ctx, realm, http.MethodGet, "/users/"+url.PathEscape(userID)+"/credentials", nil, &creds)
	return creds, err
}

// DeleteCredential removes one credential.
func (c *Client) DeleteCredential(ctx context.Context, realm, userID, credID string) error {
	_, err := c.do(ctx, realm, http.MethodDelete,
		"/users/"+url.PathEscape(userID)+"/credentials/"+url.PathEscape(credID), nil, nil)
	return err
}

// Logout ends every session of a user.
func (c *Client) Logout(ctx context.Context, realm, userID string) error {
	_, err := c.do(ctx, realm, http.MethodPost, "/users/"+url.PathEscape(userID)+"/logout", nil, nil)
	return err
}
