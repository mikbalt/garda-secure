// Package settings stores operator-editable system settings (notification
// channel credentials) in the database, encrypting secret fields.
package settings

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/securepatrol/backend/internal/config"
	"github.com/securepatrol/backend/internal/netguard"
	"github.com/securepatrol/backend/internal/repository"
	"github.com/securepatrol/backend/internal/service"
)

const channelsKey = "channels"

// secretPlaceholder is returned to clients instead of a stored secret; sending
// it back unchanged keeps the stored value.
const secretPlaceholder = "********"

// SMTP holds e-mail delivery settings.
type SMTP struct {
	Host     string `json:"host"`
	Port     string `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"`
	From     string `json:"from"`
	// Guard restricts which addresses the relay may resolve to at dial
	// time; filled in by Store.Channels.
	Guard netguard.Policy `json:"-"`
}

// SMTPPorts are the ports an SMTP relay may use (no SSRF to other services).
var SMTPPorts = []string{"25", "465", "587", "2525"}

// WhatsApp holds WhatsApp Business (Cloud API) settings.
type WhatsApp struct {
	APIURL        string `json:"api_url"`
	Token         string `json:"token"`
	PhoneNumberID string `json:"phone_number_id"`
	TemplateName  string `json:"template_name"` // approved template with one body parameter
	TemplateLang  string `json:"template_lang"`
}

// Channels groups the settings of every notification channel.
type Channels struct {
	SMTP     SMTP     `json:"smtp"`
	WhatsApp WhatsApp `json:"whatsapp"`
}

// Store reads and writes settings.
type Store struct {
	db        repository.DBTX
	cfg       *config.Config
	gcm       cipher.AEAD
	smtpGuard netguard.Policy
	waPrefix  []string
}

// NewStore builds a store whose secrets are encrypted with a key derived from
// SETTINGS_ENCRYPTION_KEY.
func NewStore(db repository.DBTX, cfg *config.Config) (*Store, error) {
	key := sha256.Sum256([]byte(cfg.SettingsEncryptionKey))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	guard, err := netguard.NewPolicy(cfg.SMTPAllowedPrivateNets)
	if err != nil {
		return nil, fmt.Errorf("SMTP_ALLOWED_PRIVATE_NETS: %w", err)
	}
	st := &Store{db: db, cfg: cfg, gcm: gcm, smtpGuard: guard}
	for _, p := range cfg.WhatsAppAPIURLAllowlist {
		st.waPrefix = append(st.waPrefix, normalizePrefix(p))
	}
	if len(st.waPrefix) == 0 {
		st.waPrefix = []string{"https://graph.facebook.com/"}
	}
	return st, nil
}

// Channels returns the effective channel settings: values saved from the web
// UI, falling back field by field to environment defaults.
func (s *Store) Channels(ctx context.Context) (Channels, error) {
	stored, err := s.load(ctx)
	if err != nil {
		return Channels{}, err
	}
	env := Channels{
		SMTP: SMTP{
			Host: s.cfg.SMTPHost, Port: s.cfg.SMTPPort, User: s.cfg.SMTPUser,
			Password: s.cfg.SMTPPassword, From: s.cfg.SMTPFrom,
		},
		WhatsApp: WhatsApp{
			APIURL: s.cfg.WhatsAppAPIURL, Token: s.cfg.WhatsAppAPIToken,
			PhoneNumberID: s.cfg.WhatsAppPhoneNumberID, TemplateName: s.cfg.WhatsAppTemplateName,
			TemplateLang: s.cfg.WhatsAppTemplateLang,
		},
	}
	out := Channels{
		SMTP: SMTP{
			Host:     pick(stored.SMTP.Host, env.SMTP.Host),
			Port:     pick(stored.SMTP.Port, env.SMTP.Port),
			User:     pick(stored.SMTP.User, env.SMTP.User),
			Password: pick(stored.SMTP.Password, env.SMTP.Password),
			From:     pick(stored.SMTP.From, env.SMTP.From),
		},
		WhatsApp: WhatsApp{
			APIURL:        pick(stored.WhatsApp.APIURL, env.WhatsApp.APIURL),
			Token:         pick(stored.WhatsApp.Token, env.WhatsApp.Token),
			PhoneNumberID: pick(stored.WhatsApp.PhoneNumberID, env.WhatsApp.PhoneNumberID),
			TemplateName:  pick(stored.WhatsApp.TemplateName, env.WhatsApp.TemplateName),
			TemplateLang:  pick(stored.WhatsApp.TemplateLang, env.WhatsApp.TemplateLang),
		},
	}
	// The relay set by the operator in the environment is trusted (it may
	// be an on-prem relay on a private network); anything entered in the UI
	// must resolve to a public or allow-listed address.
	out.SMTP.Guard = s.smtpGuard
	if out.SMTP.Host != "" && strings.EqualFold(out.SMTP.Host, s.cfg.SMTPHost) {
		out.SMTP.Guard = netguard.AllowAll
	}
	return out, nil
}

// Masked returns the effective settings with secrets replaced by a placeholder.
func (s *Store) Masked(ctx context.Context) (Channels, error) {
	c, err := s.Channels(ctx)
	if err != nil {
		return c, err
	}
	c.SMTP.Password = mask(c.SMTP.Password)
	c.WhatsApp.Token = mask(c.WhatsApp.Token)
	return c, nil
}

// Save validates and stores channel settings. Secrets equal to the
// placeholder keep their stored value — but only while nothing that decides
// where the secret is sent (SMTP host/port/user, WhatsApp API URL/phone
// number ID) changes; otherwise the secret must be entered again, so a stored
// password can never be redirected to another server. An empty secret clears
// it (falling back to the env default).
func (s *Store) Save(ctx context.Context, in Channels, actor string) error {
	stored, err := s.load(ctx)
	if err != nil {
		return err
	}
	current, err := s.Channels(ctx)
	if err != nil {
		return err
	}
	in = trimChannels(in)
	if err := s.validate(ctx, in); err != nil {
		return err
	}
	if in.SMTP.Password == secretPlaceholder {
		if !strings.EqualFold(pick(in.SMTP.Host, s.cfg.SMTPHost), current.SMTP.Host) ||
			pick(in.SMTP.Port, s.cfg.SMTPPort) != current.SMTP.Port ||
			pick(in.SMTP.User, s.cfg.SMTPUser) != current.SMTP.User {
			return service.BadRequest("secret_required",
				"re-enter the SMTP password when changing host, port or user").WithField("smtp.password")
		}
		in.SMTP.Password = stored.SMTP.Password
	}
	if in.WhatsApp.Token == secretPlaceholder {
		if strings.TrimRight(pick(in.WhatsApp.APIURL, s.cfg.WhatsAppAPIURL), "/") != strings.TrimRight(current.WhatsApp.APIURL, "/") ||
			pick(in.WhatsApp.PhoneNumberID, s.cfg.WhatsAppPhoneNumberID) != current.WhatsApp.PhoneNumberID {
			return service.BadRequest("secret_required",
				"re-enter the WhatsApp token when changing the API URL or phone number ID").WithField("whatsapp.token")
		}
		in.WhatsApp.Token = stored.WhatsApp.Token
	}
	if in.SMTP.Password, err = s.encrypt(in.SMTP.Password); err != nil {
		return err
	}
	if in.WhatsApp.Token, err = s.encrypt(in.WhatsApp.Token); err != nil {
		return err
	}
	raw, err := json.Marshal(in)
	if err != nil {
		return err
	}
	return repository.PutSetting(ctx, s.db, channelsKey, raw, actor)
}

func trimChannels(c Channels) Channels {
	t := strings.TrimSpace
	c.SMTP.Host, c.SMTP.Port, c.SMTP.User, c.SMTP.From = t(c.SMTP.Host), t(c.SMTP.Port), t(c.SMTP.User), t(c.SMTP.From)
	c.WhatsApp.APIURL, c.WhatsApp.PhoneNumberID = t(c.WhatsApp.APIURL), t(c.WhatsApp.PhoneNumberID)
	c.WhatsApp.TemplateName, c.WhatsApp.TemplateLang = t(c.WhatsApp.TemplateName), t(c.WhatsApp.TemplateLang)
	return c
}

// validate checks lengths and that the destinations are allowed.
func (s *Store) validate(ctx context.Context, c Channels) error {
	if err := service.CheckLengths(
		service.S("smtp.host", c.SMTP.Host, 253), service.S("smtp.port", c.SMTP.Port, 5),
		service.S("smtp.user", c.SMTP.User, 254), service.S("smtp.password", c.SMTP.Password, 1024),
		service.S("smtp.from", c.SMTP.From, 320), service.S("whatsapp.api_url", c.WhatsApp.APIURL, 500),
		service.S("whatsapp.token", c.WhatsApp.Token, 4096), service.S("whatsapp.phone_number_id", c.WhatsApp.PhoneNumberID, 64),
		service.S("whatsapp.template_name", c.WhatsApp.TemplateName, 512), service.S("whatsapp.template_lang", c.WhatsApp.TemplateLang, 16),
	); err != nil {
		return err
	}
	if c.SMTP.Port != "" && !slices.Contains(SMTPPorts, c.SMTP.Port) {
		return service.BadRequest("invalid_smtp_port", "SMTP port must be one of %s", strings.Join(SMTPPorts, ", ")).WithField("smtp.port")
	}
	if h := c.SMTP.Host; h != "" && !strings.EqualFold(h, s.cfg.SMTPHost) {
		if strings.ContainsAny(h, "/:@ ") && net.ParseIP(h) == nil {
			return service.BadRequest("invalid_smtp_host", "SMTP host must be a host name or IP address").WithField("smtp.host")
		}
		if err := s.smtpGuard.CheckHost(ctx, h); err != nil {
			if errors.Is(err, netguard.ErrBlocked) {
				return service.BadRequest("smtp_host_not_allowed",
					"SMTP host resolves to an internal address; ask the operator to allow-list it (SMTP_ALLOWED_PRIVATE_NETS)").WithField("smtp.host")
			}
			return service.BadRequest("smtp_host_unresolvable", "SMTP host cannot be resolved").WithField("smtp.host")
		}
	}
	if c.WhatsApp.APIURL != "" && !s.AllowedWhatsAppURL(c.WhatsApp.APIURL) {
		return service.BadRequest("whatsapp_url_not_allowed",
			"WhatsApp API URL must start with one of: %s", strings.Join(s.waPrefix, ", ")).WithField("whatsapp.api_url")
	}
	if id := c.WhatsApp.PhoneNumberID; id != "" {
		if _, err := strconv.ParseUint(id, 10, 64); err != nil {
			return service.BadRequest("invalid_phone_number_id", "phone number ID must be numeric").WithField("whatsapp.phone_number_id")
		}
	}
	return nil
}

// AllowedWhatsAppURL reports whether raw is an http(s) URL under one of the
// allow-listed prefixes (WHATSAPP_API_URL_ALLOWLIST).
func (s *Store) AllowedWhatsAppURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.User != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") ||
		u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	candidate := strings.ToLower(strings.TrimRight(raw, "/") + "/")
	for _, p := range s.waPrefix {
		if strings.HasPrefix(candidate, strings.ToLower(p)) {
			return true
		}
	}
	return false
}

// normalizePrefix makes "https://host" match only that host, not
// "https://host.evil.example".
func normalizePrefix(p string) string {
	p = strings.TrimSpace(p)
	if !strings.HasSuffix(p, "/") {
		p += "/"
	}
	return p
}

// load reads the stored settings and decrypts their secrets.
func (s *Store) load(ctx context.Context) (Channels, error) {
	var c Channels
	raw, err := repository.GetSetting(ctx, s.db, channelsKey)
	if errors.Is(err, repository.ErrNotFound) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	if err := json.Unmarshal(raw, &c); err != nil {
		return c, fmt.Errorf("decode channel settings: %w", err)
	}
	if c.SMTP.Password, err = s.decrypt(c.SMTP.Password); err != nil {
		return c, err
	}
	if c.WhatsApp.Token, err = s.decrypt(c.WhatsApp.Token); err != nil {
		return c, err
	}
	return c, nil
}

const encPrefix = "enc:v1:"

func (s *Store) encrypt(plain string) (string, error) {
	if plain == "" {
		return "", nil
	}
	nonce := make([]byte, s.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := s.gcm.Seal(nonce, nonce, []byte(plain), nil)
	return encPrefix + base64.StdEncoding.EncodeToString(sealed), nil
}

func (s *Store) decrypt(v string) (string, error) {
	if !strings.HasPrefix(v, encPrefix) {
		return v, nil
	}
	data, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(v, encPrefix))
	if err != nil || len(data) < s.gcm.NonceSize() {
		return "", errors.New("corrupt encrypted setting")
	}
	plain, err := s.gcm.Open(nil, data[:s.gcm.NonceSize()], data[s.gcm.NonceSize():], nil)
	if err != nil {
		return "", errors.New("cannot decrypt setting: SETTINGS_ENCRYPTION_KEY changed?")
	}
	return string(plain), nil
}

func pick(v, def string) string {
	if v != "" {
		return v
	}
	return def
}

func mask(v string) string {
	if v == "" {
		return ""
	}
	return secretPlaceholder
}
