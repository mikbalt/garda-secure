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
	"strings"

	"github.com/securepatrol/backend/internal/config"
	"github.com/securepatrol/backend/internal/repository"
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
}

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
	db  repository.DBTX
	cfg *config.Config
	gcm cipher.AEAD
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
	return &Store{db: db, cfg: cfg, gcm: gcm}, nil
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
	return Channels{
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
	}, nil
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

// Save stores channel settings. Secrets equal to the placeholder keep their
// stored value; an empty secret clears it (falling back to the env default).
func (s *Store) Save(ctx context.Context, in Channels, actor string) error {
	stored, err := s.load(ctx)
	if err != nil {
		return err
	}
	if in.SMTP.Password == secretPlaceholder {
		in.SMTP.Password = stored.SMTP.Password
	}
	if in.WhatsApp.Token == secretPlaceholder {
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
