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
	// Roles are bound to the client a token was issued to (azp): web roles
	// only count for KeycloakWebClient, the device role only for
	// KeycloakTabletClient.
	KeycloakWebClient    string
	KeycloakTabletClient string
	// PlatformAdminRealms limits which realms' platform-admin users may change
	// install-wide settings; empty = any configured realm.
	PlatformAdminRealms []string

	// TrustedProxies are the proxy IPs/CIDRs whose X-Real-IP header is
	// believed for the client IP (audit log, rate limits).
	TrustedProxies []string

	// Outbound channel guards (BE-S3).
	WhatsAppAPIURLAllowlist []string // URL prefixes a WhatsApp api_url must start with
	SMTPAllowedPrivateNets  []string // CIDRs/IPs of on-prem relays allowed despite being private

	// Rate limits per minute; 0 disables a limit.
	RateLimitUserWrites   int // writes per web user
	RateLimitDeviceWrites int // writes per tablet
	RateLimitSOS          int // new SOS alerts per tablet
	RateLimitAuthFailures int // failed authentications per client IP

	CORSAllowedOrigins string
	AppPublicURL       string

	// SettingsEncryptionKey encrypts channel secrets stored in system_settings.
	SettingsEncryptionKey string

	// Field identity (D-035). Empty secrets are derived from
	// SettingsEncryptionKey with HKDF-SHA256.
	GuardTokenSecret       string        // GUARD_TOKEN_SECRET (>= 32 bytes when set)
	GuardSessionTTL        time.Duration // GUARD_SESSION_TTL, default 12h
	GuardTokenGrace        time.Duration // GUARD_TOKEN_GRACE, default 168h (= offline sync window)
	QRSigningSecret        string        // QR_SIGNING_SECRET (>= 32 bytes when set)
	RateLimitGuardSessions int           // RATE_LIMIT_GUARD_SESSIONS_PER_MIN, PIN checks per tablet

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

	// Data retention (BE-S9) and Keycloak user management (F1); see features.go.
	Retention     Retention
	KeycloakAdmin KeycloakAdmin

	MigrationsPath string

	LogLevel string
	Port     string
}

// Load reads configuration from environment variables.
// Returns an error if any required variable is missing.
func Load() (*Config, error) {
	cfg := &Config{
		DatabaseURL:             os.Getenv("DATABASE_URL"),
		RedisURL:                os.Getenv("REDIS_URL"),
		MinioEndpoint:           os.Getenv("MINIO_ENDPOINT"),
		MinioPublicEndpoint:     os.Getenv("MINIO_PUBLIC_ENDPOINT"),
		MinioUseSSL:             envBool("MINIO_USE_SSL", false),
		MinioPublicUseSSL:       envBool("MINIO_PUBLIC_USE_SSL", false),
		MinioAccessKey:          os.Getenv("MINIO_ACCESS_KEY"),
		MinioSecretKey:          os.Getenv("MINIO_SECRET_KEY"),
		MinioBucketPhotos:       envOrDefault("MINIO_BUCKET_PHOTOS", "photos"),
		MinioBucketBackups:      envOrDefault("MINIO_BUCKET_BACKUPS", "backups"),
		KeycloakURL:             strings.TrimRight(os.Getenv("KEYCLOAK_URL"), "/"),
		KeycloakPublicURL:       strings.TrimRight(os.Getenv("KEYCLOAK_PUBLIC_URL"), "/"),
		KeycloakRealms:          splitList(os.Getenv("KEYCLOAK_REALMS")),
		KeycloakAllowedClients:  splitList(envOrDefault("KEYCLOAK_ALLOWED_CLIENTS", "securepatrol-web,securepatrol-tablet")),
		KeycloakWebClient:       envOrDefault("KEYCLOAK_WEB_CLIENT", "securepatrol-web"),
		KeycloakTabletClient:    envOrDefault("KEYCLOAK_TABLET_CLIENT", "securepatrol-tablet"),
		PlatformAdminRealms:     splitList(os.Getenv("PLATFORM_ADMIN_REALMS")),
		TrustedProxies:          splitList(envOrDefault("TRUSTED_PROXIES", DefaultTrustedProxies)),
		WhatsAppAPIURLAllowlist: splitList(envOrDefault("WHATSAPP_API_URL_ALLOWLIST", "https://graph.facebook.com/")),
		SMTPAllowedPrivateNets:  splitList(os.Getenv("SMTP_ALLOWED_PRIVATE_NETS")),
		CORSAllowedOrigins:      os.Getenv("CORS_ALLOWED_ORIGINS"),
		AppPublicURL:            strings.TrimRight(os.Getenv("APP_PUBLIC_URL"), "/"),
		SettingsEncryptionKey:   os.Getenv("SETTINGS_ENCRYPTION_KEY"),
		SMTPHost:                os.Getenv("SMTP_HOST"),
		SMTPPort:                envOrDefault("SMTP_PORT", "587"),
		SMTPUser:                os.Getenv("SMTP_USER"),
		SMTPPassword:            os.Getenv("SMTP_PASSWORD"),
		SMTPFrom:                os.Getenv("SMTP_FROM"),
		WhatsAppAPIURL:          envOrDefault("WHATSAPP_API_URL", "https://graph.facebook.com/v18.0"),
		WhatsAppAPIToken:        os.Getenv("WHATSAPP_API_TOKEN"),
		WhatsAppPhoneNumberID:   os.Getenv("WHATSAPP_PHONE_NUMBER_ID"),
		WhatsAppTemplateName:    os.Getenv("WHATSAPP_TEMPLATE_NAME"),
		WhatsAppTemplateLang:    envOrDefault("WHATSAPP_TEMPLATE_LANG", "id"),
		MigrationsPath:          envOrDefault("MIGRATIONS_PATH", "/migrations"),
		LogLevel:                envOrDefault("LOG_LEVEL", "info"),
		Port:                    envOrDefault("PORT", "8080"),
	}
	if cfg.MinioPublicEndpoint == "" {
		cfg.MinioPublicEndpoint = cfg.MinioEndpoint
	}
	if cfg.KeycloakPublicURL == "" {
		cfg.KeycloakPublicURL = cfg.KeycloakURL
	}

	for _, l := range []struct {
		env string
		def int
		dst *int
	}{
		{"RATE_LIMIT_USER_WRITES_PER_MIN", 120, &cfg.RateLimitUserWrites},
		{"RATE_LIMIT_DEVICE_WRITES_PER_MIN", 600, &cfg.RateLimitDeviceWrites},
		{"RATE_LIMIT_SOS_PER_MIN", 5, &cfg.RateLimitSOS},
		{"RATE_LIMIT_AUTH_FAILURES_PER_MIN", 30, &cfg.RateLimitAuthFailures},
		{"RATE_LIMIT_GUARD_SESSIONS_PER_MIN", 20, &cfg.RateLimitGuardSessions},
	} {
		n, err := strconv.Atoi(envOrDefault(l.env, strconv.Itoa(l.def)))
		if err != nil || n < 0 {
			return nil, fmt.Errorf("%s must be a non-negative integer", l.env)
		}
		*l.dst = n
	}

	hour, err := strconv.Atoi(envOrDefault("BACKUP_HOUR", "2"))
	if err != nil || hour < 0 || hour > 23 {
		return nil, fmt.Errorf("BACKUP_HOUR must be 0-23")
	}
	cfg.BackupHour = hour
	if err := cfg.loadReliability(); err != nil {
		return nil, err
	}
	if err := cfg.loadField(); err != nil {
		return nil, err
	}
	if err := cfg.loadFeatures(); err != nil {
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

// DefaultTrustedProxies are the private ranges Docker networks use; nginx
// reaches the API from one of them.
const DefaultTrustedProxies = "10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,127.0.0.1/32,::1/128"

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

// MinFieldSecretBytes is the minimum length of GUARD_TOKEN_SECRET and
// QR_SIGNING_SECRET.
const MinFieldSecretBytes = 32

// loadField reads the guard-token and QR-signing settings (D-035).
func (c *Config) loadField() error {
	c.GuardTokenSecret = os.Getenv("GUARD_TOKEN_SECRET")
	c.QRSigningSecret = os.Getenv("QR_SIGNING_SECRET")
	for env, v := range map[string]string{"GUARD_TOKEN_SECRET": c.GuardTokenSecret, "QR_SIGNING_SECRET": c.QRSigningSecret} {
		if v != "" && len(v) < MinFieldSecretBytes {
			return fmt.Errorf("%s must be at least %d bytes (or unset to derive it from SETTINGS_ENCRYPTION_KEY)", env, MinFieldSecretBytes)
		}
	}
	var err error
	if c.GuardSessionTTL, err = envDuration("GUARD_SESSION_TTL", 12*time.Hour); err != nil {
		return err
	}
	if c.GuardSessionTTL < 5*time.Minute || c.GuardSessionTTL > 7*24*time.Hour {
		return errors.New("GUARD_SESSION_TTL must be between 5m and 168h")
	}
	if c.GuardTokenGrace, err = envDuration("GUARD_TOKEN_GRACE", 7*24*time.Hour); err != nil {
		return err
	}
	if c.GuardTokenGrace < time.Minute || c.GuardTokenGrace > 30*24*time.Hour {
		return errors.New("GUARD_TOKEN_GRACE must be between 1m and 720h")
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
