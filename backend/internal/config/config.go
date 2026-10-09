package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
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

	BackupHour int // local server hour for the daily pg_dump

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
