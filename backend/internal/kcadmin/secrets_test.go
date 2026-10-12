package kcadmin_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/securepatrol/backend/internal/kcadmin"
	"github.com/securepatrol/backend/internal/kcadmin/kcfake"
)

// Secrets written by keycloak-config.sh after the API started are picked up
// without a restart.
func TestSecretsFileIsReread(t *testing.T) {
	f := kcfake.New("securepatrol-admin-api", map[string]string{"r1": "s1"})
	defer f.Close()
	path := filepath.Join(t.TempDir(), "keycloak-admin-clients")
	c := kcadmin.New(kcadmin.Options{BaseURL: f.URL, ClientID: "securepatrol-admin-api", SecretsFile: path})
	if c.Configured("r1") {
		t.Fatal("configured without a file")
	}
	if err := os.WriteFile(path, []byte("# comment\nr1=s1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// The first lookup above started the 30 s throttle; a fresh client
	// reads the file immediately.
	c = kcadmin.New(kcadmin.Options{BaseURL: f.URL, ClientID: "securepatrol-admin-api", SecretsFile: path})
	if _, err := c.ListUsers(context.Background(), "r1", "", 0, 1); err != nil {
		t.Fatalf("with file: %v", err)
	}
}
