package config

import (
	"errors"
	"os"
)

// Config holds all runtime configuration loaded from environment variables.
type Config struct {
	DatabaseURL        string
	RedisURL           string
	MinioEndpoint      string
	MinioAccessKey     string
	MinioSecretKey     string
	MinioBucketPhotos  string
	MinioBucketBackups string
	KeycloakJWKSURL    string
	KeycloakIssuer     string
	SMTPHost           string
	SMTPPort           string
	SMTPUser           string
	SMTPPassword       string
	WhatsAppAPIURL     string
	WhatsAppAPIToken   string
	LogLevel           string
	Port               string
}

// Load reads configuration from environment variables.
// Returns an error if any required variable is missing.
func Load() (*Config, error) {
	cfg := &Config{
		DatabaseURL:        os.Getenv("DATABASE_URL"),
		RedisURL:           os.Getenv("REDIS_URL"),
		MinioEndpoint:      os.Getenv("MINIO_ENDPOINT"),
		MinioAccessKey:     os.Getenv("MINIO_ACCESS_KEY"),
		MinioSecretKey:     os.Getenv("MINIO_SECRET_KEY"),
		MinioBucketPhotos:  envOrDefault("MINIO_BUCKET_PHOTOS", "photos"),
		MinioBucketBackups: envOrDefault("MINIO_BUCKET_BACKUPS", "backups"),
		KeycloakJWKSURL:    os.Getenv("KEYCLOAK_JWKS_URL"),
		KeycloakIssuer:     os.Getenv("KEYCLOAK_ISSUER"),
		SMTPHost:           os.Getenv("SMTP_HOST"),
		SMTPPort:           envOrDefault("SMTP_PORT", "587"),
		SMTPUser:           os.Getenv("SMTP_USER"),
		SMTPPassword:       os.Getenv("SMTP_PASSWORD"),
		WhatsAppAPIURL:     os.Getenv("WHATSAPP_API_URL"),
		WhatsAppAPIToken:   os.Getenv("WHATSAPP_API_TOKEN"),
		LogLevel:           envOrDefault("LOG_LEVEL", "info"),
		Port:               envOrDefault("PORT", "8080"),
	}

	if cfg.DatabaseURL == "" {
		return nil, errors.New("DATABASE_URL is required")
	}
	if cfg.KeycloakJWKSURL == "" {
		return nil, errors.New("KEYCLOAK_JWKS_URL is required")
	}
	if cfg.KeycloakIssuer == "" {
		return nil, errors.New("KEYCLOAK_ISSUER is required")
	}

	return cfg, nil
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
