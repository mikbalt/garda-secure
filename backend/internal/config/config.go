package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all runtime configuration loaded from environment variables.
type Config struct {
	DatabaseURL string
	RedisURL    string

	MinioEndpoint       string
	MinioPublicEndpoint string // host:port browsers use for pre-signed URLs
	MinioUseSSL         bool
	MinioPublicUseSSL   bool
	MinioAccessKey      string
	MinioSecretKey      string
	MinioBucketPhotos   string
	MinioBucketBackups  string

	// Keycloak: one realm per client. Tokens are validated against
	// <KeycloakPublicURL>/realms/<realm> (issuer) using keys fetched from
	// <KeycloakURL>/realms/<realm>/protocol/openid-connect/certs.
	KeycloakURL            string
	KeycloakPublicURL      string
	KeycloakRealms         []string
	KeycloakAllowedClients []string

	CORSAllowedOrigins string
	AppPublicURL       string

	// SettingsEncryptionKey encrypts channel secrets stored in system_settings.
	SettingsEncryptionKey string

	// Channel defaults; admins can override them from the web UI.
	SMTPHost              string
	SMTPPort              string
	SMTPUser              string
	SMTPPassword          string
	SMTPFrom              string
	WhatsAppAPIURL        string
	WhatsAppAPIToken      string
	WhatsAppPhoneNumberID string
	WhatsAppTemplateName  string
	WhatsAppTemplateLang  string

	BackupHour int // hour (0-23) in BackupLocation for the daily pg_dump
	// BackupLocation is BACKUP_TZ (default Asia/Jakarta), independent of the
	// container's TZ.
	BackupLocation *time.Location
	// BackupTimeout bounds each pg_dump (BACKUP_TIMEOUT, default 1h).
	BackupTimeout time.Duration
	// BackupKeycloakDatabaseURL, when set, is dumped into a separate object
	// next to the application database (BACKUP_KEYCLOAK_DATABASE_URL).
	BackupKeycloakDatabaseURL string

	// Database pool (BE-R6).
	DBMaxConns         int32         // DB_MAX_CONNS, default 20
	DBStatementTimeout time.Duration // DB_STATEMENT_TIMEOUT, default 30s

	// Worker (BE-R2, BE-O1, BE-R6).
	NotifyWorkers         int           // NOTIFY_WORKERS, default 4 (min 2: one is reserved for SOS)
	WorkerHTTPAddr        string        // WORKER_HTTP_ADDR, default :9091 (/health, /metrics)
	WorkerShutdownTimeout time.Duration // WORKER_SHUTDOWN_TIMEOUT, default 40s

	MigrationsPath string

	LogLevel string
	Port     string
}

// Load reads configuration from environment variables.
// Returns an error if any required variable is missing.
func Load() (*Config, error) {
	cfg := &Config{
		DatabaseURL:            os.Getenv("DATABASE_URL"),
		RedisURL:               os.Getenv("REDIS_URL"),
		MinioEndpoint:          os.Getenv("MINIO_ENDPOINT"),
		MinioPublicEndpoint:    os.Getenv("MINIO_PUBLIC_ENDPOINT"),
		MinioUseSSL:            envBool("MINIO_USE_SSL", false),
		MinioPublicUseSSL:      envBool("MINIO_PUBLIC_USE_SSL", false),
		MinioAccessKey:         os.Getenv("MINIO_ACCESS_KEY"),
		MinioSecretKey:         os.Getenv("MINIO_SECRET_KEY"),
		MinioBucketPhotos:      envOrDefault("MINIO_BUCKET_PHOTOS", "photos"),
		MinioBucketBackups:     envOrDefault("MINIO_BUCKET_BACKUPS", "backups"),
		KeycloakURL:            strings.TrimRight(os.Getenv("KEYCLOAK_URL"), "/"),
		KeycloakPublicURL:      strings.TrimRight(os.Getenv("KEYCLOAK_PUBLIC_URL"), "/"),
		KeycloakRealms:         splitList(os.Getenv("KEYCLOAK_REALMS")),
		KeycloakAllowedClients: splitList(envOrDefault("KEYCLOAK_ALLOWED_CLIENTS", "securepatrol-web,securepatrol-tablet")),
		CORSAllowedOrigins:     os.Getenv("CORS_ALLOWED_ORIGINS"),
		AppPublicURL:           strings.TrimRight(os.Getenv("APP_PUBLIC_URL"), "/"),
		SettingsEncryptionKey:  os.Getenv("SETTINGS_ENCRYPTION_KEY"),
		SMTPHost:               os.Getenv("SMTP_HOST"),
		SMTPPort:               envOrDefault("SMTP_PORT", "587"),
		SMTPUser:               os.Getenv("SMTP_USER"),
		SMTPPassword:           os.Getenv("SMTP_PASSWORD"),
		SMTPFrom:               os.Getenv("SMTP_FROM"),
		WhatsAppAPIURL:         envOrDefault("WHATSAPP_API_URL", "https://graph.facebook.com/v18.0"),
		WhatsAppAPIToken:       os.Getenv("WHATSAPP_API_TOKEN"),
		WhatsAppPhoneNumberID:  os.Getenv("WHATSAPP_PHONE_NUMBER_ID"),
		WhatsAppTemplateName:   os.Getenv("WHATSAPP_TEMPLATE_NAME"),
		WhatsAppTemplateLang:   envOrDefault("WHATSAPP_TEMPLATE_LANG", "id"),
		MigrationsPath:         envOrDefault("MIGRATIONS_PATH", "/migrations"),
		LogLevel:               envOrDefault("LOG_LEVEL", "info"),
		Port:                   envOrDefault("PORT", "8080"),
	}
	if cfg.MinioPublicEndpoint == "" {
		cfg.MinioPublicEndpoint = cfg.MinioEndpoint
	}
	if cfg.KeycloakPublicURL == "" {
		cfg.KeycloakPublicURL = cfg.KeycloakURL
	}

	hour, err := strconv.Atoi(envOrDefault("BACKUP_HOUR", "2"))
	if err != nil || hour < 0 || hour > 23 {
		return nil, fmt.Errorf("BACKUP_HOUR must be 0-23")
	}
	cfg.BackupHour = hour
	if err := cfg.loadReliability(); err != nil {
		return nil, err
	}

	if cfg.DatabaseURL == "" {
		return nil, errors.New("DATABASE_URL is required")
	}
	if cfg.KeycloakURL == "" {
		return nil, errors.New("KEYCLOAK_URL is required")
	}
	if len(cfg.KeycloakRealms) == 0 {
		return nil, errors.New("KEYCLOAK_REALMS is required (comma-separated realm names)")
	}
	if cfg.SettingsEncryptionKey == "" {
		return nil, errors.New("SETTINGS_ENCRYPTION_KEY is required")
	}

	return cfg, nil
}

// IssuerFor returns the token issuer URL of a realm.
func (c *Config) IssuerFor(realm string) string {
	return c.KeycloakPublicURL + "/realms/" + realm
}

// JWKSURLFor returns the internal JWKS URL of a realm.
func (c *Config) JWKSURLFor(realm string) string {
	return c.KeycloakURL + "/realms/" + realm + "/protocol/openid-connect/certs"
}

// loadReliability reads the backup, pool and worker settings.
func (c *Config) loadReliability() error {
	var err error
	if c.BackupLocation, err = time.LoadLocation(envOrDefault("BACKUP_TZ", "Asia/Jakarta")); err != nil {
		return fmt.Errorf("BACKUP_TZ: %w", err)
	}
	if c.BackupTimeout, err = envDuration("BACKUP_TIMEOUT", time.Hour); err != nil {
		return err
	}
	c.BackupKeycloakDatabaseURL = os.Getenv("BACKUP_KEYCLOAK_DATABASE_URL")

	maxConns, err := envInt("DB_MAX_CONNS", 20)
	if err != nil || maxConns < 2 || maxConns > 1000 {
		return errors.New("DB_MAX_CONNS must be between 2 and 1000")
	}
	c.DBMaxConns = int32(maxConns) // #nosec G115 -- range checked above
	if c.DBStatementTimeout, err = envDuration("DB_STATEMENT_TIMEOUT", 30*time.Second); err != nil {
		return err
	}

	if c.NotifyWorkers, err = envInt("NOTIFY_WORKERS", 4); err != nil || c.NotifyWorkers < 2 || c.NotifyWorkers > 64 {
		return errors.New("NOTIFY_WORKERS must be between 2 and 64")
	}
	c.WorkerHTTPAddr = envOrDefault("WORKER_HTTP_ADDR", ":9091")
	if c.WorkerShutdownTimeout, err = envDuration("WORKER_SHUTDOWN_TIMEOUT", 40*time.Second); err != nil {
		return err
	}
	return nil
}

func envInt(key string, def int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer", key)
	}
	return n, nil
}

func envDuration(key string, def time.Duration) (time.Duration, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("%s must be a duration like 30s or 1h", key)
	}
	return d, nil
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envBool(key string, def bool) bool {
	v, err := strconv.ParseBool(os.Getenv(key))
	if err != nil {
		return def
	}
	return v
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
