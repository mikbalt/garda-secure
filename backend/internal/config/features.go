package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

// Retention holds the data retention periods in days (BE-S9, UU PDP);
// 0 keeps the data forever.
type Retention struct {
	VisitorDays       int // visitor logs incl. their photos (default 365)
	SelfieDays        int // clock-in selfies: object deleted, key cleared (365)
	GPSDays           int // attendance and checkpoint-scan GPS cleared (365)
	NotificationDays  int // notification_log bodies, subjects, recipients cleared (180)
	EvidencePhotoDays int // incident, defect and checkpoint photos (0 = keep: evidence)
}

// KeycloakAdmin configures the Keycloak Admin REST client used for user and
// device-account management (F1, NFR-09).
type KeycloakAdmin struct {
	// InternalURL is Keycloak's base URL on the internal network
	// (KEYCLOAK_INTERNAL_URL, default KEYCLOAK_URL), e.g. http://keycloak:8080/auth.
	InternalURL string
	// ClientID is the confidential client with a service account in every
	// realm (KEYCLOAK_ADMIN_CLIENT_ID, default securepatrol-admin-api).
	ClientID string
	// Secrets maps realm -> client secret, from KEYCLOAK_ADMIN_CLIENT_SECRETS_FILE
	// (preferred; lines "realm=secret") and/or KEYCLOAK_ADMIN_CLIENT_SECRETS
	// ("realm1=secret1,realm2=secret2"). A realm without a secret answers
	// 503 kc_unavailable on user-management endpoints.
	Secrets map[string]string
	// SecretsFile is KEYCLOAK_ADMIN_CLIENT_SECRETS_FILE; the client re-reads
	// it for realms without a (valid) secret, so no restart is needed after
	// keycloak-config.sh wrote it.
	SecretsFile string
	// Timeout bounds every Keycloak call (KEYCLOAK_ADMIN_TIMEOUT, 10s).
	Timeout time.Duration
}

// loadFeatures reads the retention and Keycloak admin settings.
func (c *Config) loadFeatures() error {
	var err error
	for _, r := range []struct {
		env string
		def int
		dst *int
	}{
		{"RETENTION_VISITOR_DAYS", 365, &c.Retention.VisitorDays},
		{"RETENTION_SELFIE_DAYS", 365, &c.Retention.SelfieDays},
		{"RETENTION_GPS_DAYS", 365, &c.Retention.GPSDays},
		{"RETENTION_NOTIFICATION_DAYS", 180, &c.Retention.NotificationDays},
		{"RETENTION_EVIDENCE_PHOTO_DAYS", 0, &c.Retention.EvidencePhotoDays},
	} {
		n, err := envInt(r.env, r.def)
		if err != nil || n < 0 || n > 36500 {
			return fmt.Errorf("%s must be between 0 and 36500 days", r.env)
		}
		*r.dst = n
	}

	c.KeycloakAdmin.InternalURL = strings.TrimRight(envOrDefault("KEYCLOAK_INTERNAL_URL", c.KeycloakURL), "/")
	c.KeycloakAdmin.ClientID = envOrDefault("KEYCLOAK_ADMIN_CLIENT_ID", "securepatrol-admin-api")
	if c.KeycloakAdmin.Timeout, err = envDuration("KEYCLOAK_ADMIN_TIMEOUT", 10*time.Second); err != nil {
		return err
	}
	c.KeycloakAdmin.Secrets = map[string]string{}
	if path := os.Getenv("KEYCLOAK_ADMIN_CLIENT_SECRETS_FILE"); path != "" {
		c.KeycloakAdmin.SecretsFile = path
		data, err := os.ReadFile(path) // #nosec G304 G703 -- operator-configured secrets file
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("KEYCLOAK_ADMIN_CLIENT_SECRETS_FILE: %w", err)
		}
		if err := parseSecrets(string(data), "\n", c.KeycloakAdmin.Secrets); err != nil {
			return fmt.Errorf("KEYCLOAK_ADMIN_CLIENT_SECRETS_FILE: %w", err)
		}
	}
	if err := parseSecrets(os.Getenv("KEYCLOAK_ADMIN_CLIENT_SECRETS"), ",", c.KeycloakAdmin.Secrets); err != nil {
		return fmt.Errorf("KEYCLOAK_ADMIN_CLIENT_SECRETS: %w", err)
	}
	return nil
}

// parseSecrets reads "realm=secret" entries separated by sep (blank lines
// and # comments are ignored). Later entries win.
func parseSecrets(s, sep string, into map[string]string) error {
	for _, line := range strings.Split(s, sep) {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		realm, secret, ok := strings.Cut(line, "=")
		realm, secret = strings.TrimSpace(realm), strings.TrimSpace(secret)
		if !ok || realm == "" || secret == "" {
			return fmt.Errorf("entry %q is not realm=secret", redactEntry(line))
		}
		into[realm] = secret
	}
	return nil
}

// redactEntry keeps only the realm part of a malformed entry for errors.
func redactEntry(line string) string {
	if realm, _, ok := strings.Cut(line, "="); ok {
		return realm + "=***"
	}
	if len(line) > 3 {
		return line[:3] + "***"
	}
	return "***"
}
