// Package kcfake is an in-memory fake of the parts of the Keycloak Admin
// REST API that package kcadmin uses, for unit and integration tests (no
// real Keycloak needed). Not used by the production binary.
package kcfake

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// User is a stored user.
type User struct {
	ID              string              `json:"id"`
	Username        string              `json:"username"`
	Email           string              `json:"email,omitempty"`
	EmailVerified   bool                `json:"emailVerified"`
	FirstName       string              `json:"firstName,omitempty"`
	LastName        string              `json:"lastName,omitempty"`
	Enabled         bool                `json:"enabled"`
	Totp            bool                `json:"totp"`
	Created         int64               `json:"createdTimestamp"`
	Attributes      map[string][]string `json:"attributes,omitempty"`
	RequiredActions []string            `json:"requiredActions"`

	Roles       []string `json:"-"`
	Password    string   `json:"-"`
	TempPass    bool     `json:"-"`
	OTPCredID   string   `json:"-"`
	LoggedOut   int      `json:"-"`
	EmailedActs []string `json:"-"`
}

// Realm is one realm's state.
type Realm struct {
	Secret string
	Roles  []string
	Users  map[string]*User
}

// Server is the fake Keycloak.
type Server struct {
	*httptest.Server
	ClientID string

	mu     sync.Mutex
	realms map[string]*Realm
	tokens map[string]string // token -> realm
	// Down makes every call answer 503 (Keycloak unavailable).
	Down bool
	// TokenRequests counts client-credentials grants (token caching tests).
	TokenRequests int
}

// Roles every fake realm has.
var defaultRoles = []string{"admin", "supervisor", "hr", "client-readonly", "platform-admin", "device"}

// New starts a fake with the given realm -> client secret.
func New(clientID string, realmSecrets map[string]string) *Server {
	s := &Server{ClientID: clientID, realms: map[string]*Realm{}, tokens: map[string]string{}}
	for r, sec := range realmSecrets {
		s.realms[r] = &Realm{Secret: sec, Roles: append([]string{}, defaultRoles...), Users: map[string]*User{}}
	}
	s.Server = httptest.NewServer(http.HandlerFunc(s.serve))
	return s
}

// AddUser stores a user directly (test setup) and returns it.
func (s *Server) AddUser(realm string, u User) *User {
	s.mu.Lock()
	defer s.mu.Unlock()
	if u.ID == "" {
		u.ID = uuid.NewString()
	}
	if u.Created == 0 {
		u.Created = time.Now().UnixMilli()
	}
	if u.RequiredActions == nil {
		u.RequiredActions = []string{}
	}
	if u.Totp {
		u.OTPCredID = uuid.NewString()
	}
	cp := u
	s.realms[realm].Users[u.ID] = &cp
	return &cp
}

// Get returns a copy of a stored user (nil if missing).
func (s *Server) Get(realm, id string) *User {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.realms[realm].Users[id]
	if !ok {
		return nil
	}
	cp := *u
	return &cp
}

// ByUsername returns a copy of a stored user by username (nil if missing).
func (s *Server) ByUsername(realm, username string) *User {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, u := range s.realms[realm].Users {
		if u.Username == username {
			cp := *u
			return &cp
		}
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Down {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	// /realms/{r}/protocol/openid-connect/token
	if len(parts) == 5 && parts[0] == "realms" && parts[4] == "token" {
		s.tokenEndpoint(w, r, parts[1])
		return
	}
	// /admin/realms/{r}/...
	if len(parts) < 4 || parts[0] != "admin" || parts[1] != "realms" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	realm, ok := s.realms[parts[2]]
	tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok || s.tokens[tok] != parts[2] {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	s.admin(w, r, realm, parts[3:])
}

func (s *Server) tokenEndpoint(w http.ResponseWriter, r *http.Request, realmName string) {
	realm, ok := s.realms[realmName]
	if err := r.ParseForm(); err != nil || !ok || r.PostForm.Get("grant_type") != "client_credentials" ||
		r.PostForm.Get("client_id") != s.ClientID || r.PostForm.Get("client_secret") != realm.Secret {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized_client"})
		return
	}
	s.TokenRequests++
	tok := uuid.NewString()
	s.tokens[tok] = realmName
	writeJSON(w, http.StatusOK, map[string]any{"access_token": tok, "expires_in": 300, "token_type": "Bearer"})
}

func (s *Server) admin(w http.ResponseWriter, r *http.Request, realm *Realm, p []string) {
	switch {
	case p[0] == "users" && len(p) == 1 && r.Method == http.MethodGet:
		s.listUsers(w, r, realm)
	case p[0] == "users" && len(p) == 1 && r.Method == http.MethodPost:
		s.createUser(w, r, realm)
	case p[0] == "users" && len(p) >= 2:
		u, ok := realm.Users[p[1]]
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "User not found"})
			return
		}
		s.user(w, r, realm, u, p[2:])
	case p[0] == "roles" && len(p) == 2 && r.Method == http.MethodGet:
		if !slices.Contains(realm.Roles, p[1]) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Could not find role"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"id": "role-" + p[1], "name": p[1]})
	case p[0] == "roles" && len(p) == 3 && p[2] == "users" && r.Method == http.MethodGet:
		if !slices.Contains(realm.Roles, p[1]) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Could not find role"})
			return
		}
		var out []*User
		for _, u := range realm.Users {
			if slices.Contains(u.Roles, p[1]) {
				out = append(out, u)
			}
		}
		writeJSON(w, http.StatusOK, page(sorted(out), r))
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func sorted(us []*User) []*User {
	slices.SortFunc(us, func(a, b *User) int { return strings.Compare(a.Username, b.Username) })
	return us
}

func page(us []*User, r *http.Request) []*User {
	first, _ := strconv.Atoi(r.URL.Query().Get("first"))
	max, err := strconv.Atoi(r.URL.Query().Get("max"))
	if err != nil || max <= 0 {
		max = 100
	}
	if first > len(us) {
		first = len(us)
	}
	end := first + max
	if end > len(us) {
		end = len(us)
	}
	out := us[first:end]
	if out == nil {
		out = []*User{}
	}
	return out
}

func (s *Server) listUsers(w http.ResponseWriter, r *http.Request, realm *Realm) {
	q := strings.ToLower(r.URL.Query().Get("search"))
	var out []*User
	for _, u := range realm.Users {
		if q == "" || strings.Contains(strings.ToLower(u.Username+" "+u.Email+" "+u.FirstName+" "+u.LastName), q) {
			out = append(out, u)
		}
	}
	writeJSON(w, http.StatusOK, page(sorted(out), r))
}

func conflict(realm *Realm, self, username, email string) string {
	for _, u := range realm.Users {
		if u.ID == self {
			continue
		}
		if username != "" && strings.EqualFold(u.Username, username) {
			return "User exists with same username"
		}
		if email != "" && strings.EqualFold(u.Email, email) {
			return "User exists with same email"
		}
	}
	return ""
}

func (s *Server) createUser(w http.ResponseWriter, r *http.Request, realm *Realm) {
	var in struct {
		User
		Credentials []struct {
			Type      string `json:"type"`
			Value     string `json:"value"`
			Temporary bool   `json:"temporary"`
		} `json:"credentials"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.Username == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"errorMessage": "invalid user"})
		return
	}
	if msg := conflict(realm, "", in.Username, in.Email); msg != "" {
		writeJSON(w, http.StatusConflict, map[string]string{"errorMessage": msg})
		return
	}
	u := in.User
	u.ID = uuid.NewString()
	u.Username = strings.ToLower(u.Username)
	u.Created = time.Now().UnixMilli()
	if u.RequiredActions == nil {
		u.RequiredActions = []string{}
	}
	for _, c := range in.Credentials {
		if c.Type == "password" {
			u.Password, u.TempPass = c.Value, c.Temporary
		}
	}
	realm.Users[u.ID] = &u
	w.Header().Set("Location", fmt.Sprintf("%s/admin/realms/x/users/%s", s.URL, u.ID))
	w.WriteHeader(http.StatusCreated)
}

func (s *Server) user(w http.ResponseWriter, r *http.Request, realm *Realm, u *User, p []string) {
	switch {
	case len(p) == 0 && r.Method == http.MethodGet:
		writeJSON(w, http.StatusOK, u)
	case len(p) == 0 && r.Method == http.MethodDelete:
		delete(realm.Users, u.ID)
		w.WriteHeader(http.StatusNoContent)
	case len(p) == 0 && r.Method == http.MethodPut:
		var in User
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"errorMessage": "invalid user"})
			return
		}
		if msg := conflict(realm, u.ID, "", in.Email); msg != "" {
			writeJSON(w, http.StatusConflict, map[string]string{"errorMessage": msg})
			return
		}
		u.Email, u.FirstName, u.LastName, u.Enabled = in.Email, in.FirstName, in.LastName, in.Enabled
		u.Attributes, u.RequiredActions = in.Attributes, in.RequiredActions
		w.WriteHeader(http.StatusNoContent)
	case len(p) == 2 && p[0] == "role-mappings" && p[1] == "realm":
		s.roleMappings(w, r, realm, u)
	case len(p) == 1 && p[0] == "reset-password" && r.Method == http.MethodPut:
		var c struct {
			Value     string `json:"value"`
			Temporary bool   `json:"temporary"`
		}
		_ = json.NewDecoder(r.Body).Decode(&c)
		if len(c.Value) < 10 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalidPasswordMinLengthMessage",
				"error_description": "Invalid password: minimum length 10."})
			return
		}
		u.Password, u.TempPass = c.Value, c.Temporary
		w.WriteHeader(http.StatusNoContent)
	case len(p) == 1 && p[0] == "execute-actions-email" && r.Method == http.MethodPut:
		if u.Email == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"errorMessage": "User email missing"})
			return
		}
		var acts []string
		_ = json.NewDecoder(r.Body).Decode(&acts)
		u.EmailedActs = acts
		w.WriteHeader(http.StatusNoContent)
	case len(p) == 1 && p[0] == "credentials" && r.Method == http.MethodGet:
		creds := []map[string]string{}
		if u.Password != "" {
			creds = append(creds, map[string]string{"id": "pw-" + u.ID, "type": "password"})
		}
		if u.OTPCredID != "" {
			creds = append(creds, map[string]string{"id": u.OTPCredID, "type": "otp"})
		}
		writeJSON(w, http.StatusOK, creds)
	case len(p) == 2 && p[0] == "credentials" && r.Method == http.MethodDelete:
		if p[1] != u.OTPCredID || u.OTPCredID == "" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		u.OTPCredID, u.Totp = "", false
		w.WriteHeader(http.StatusNoContent)
	case len(p) == 1 && p[0] == "logout" && r.Method == http.MethodPost:
		u.LoggedOut++
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (s *Server) roleMappings(w http.ResponseWriter, r *http.Request, realm *Realm, u *User) {
	switch r.Method {
	case http.MethodGet:
		out := []map[string]string{}
		for _, name := range u.Roles {
			out = append(out, map[string]string{"id": "role-" + name, "name": name})
		}
		writeJSON(w, http.StatusOK, out)
	case http.MethodPost, http.MethodDelete:
		var roles []struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&roles); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		for _, role := range roles {
			if !slices.Contains(realm.Roles, role.Name) {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			if r.Method == http.MethodPost && !slices.Contains(u.Roles, role.Name) {
				u.Roles = append(u.Roles, role.Name)
			}
			if r.Method == http.MethodDelete {
				u.Roles = slices.DeleteFunc(u.Roles, func(x string) bool { return x == role.Name })
			}
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}
