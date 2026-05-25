package config

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestValidateProductionSkipsInDevMode(t *testing.T) {
	cfg := &Config{DevMode: true}
	if err := ValidateProduction(cfg, DefaultBootstrapAdminPasswordHash); err != nil {
		t.Fatalf("expected no error in dev mode, got %v", err)
	}
}

func TestValidateProductionSkipsWithInsecureAllow(t *testing.T) {
	cfg := &Config{InsecureAllow: true}
	if err := ValidateProduction(cfg, DefaultBootstrapAdminPasswordHash); err != nil {
		t.Fatalf("expected no error with insecure allow, got %v", err)
	}
}

func TestValidateProductionFailsOnDefaultAdminPassword(t *testing.T) {
	cfg := &Config{
		JWTSecret:         "stable-secret",
		JWTSecretExplicit: true,
		EncryptionKey:     "my-encryption-key",
	}
	err := ValidateProduction(cfg, DefaultBootstrapAdminPasswordHash)
	if err == nil {
		t.Fatal("expected validation error for default admin password")
	}
}

func TestValidateProductionFailsWithoutJWTSecret(t *testing.T) {
	dir := t.TempDir()
	key := make([]byte, 32)
	_ = os.WriteFile(filepath.Join(dir, "encryption.key"), []byte(hex.EncodeToString(key)), 0o600)

	cfg := &Config{
		DataDir:           dir,
		JWTSecret:         "auto",
		JWTSecretExplicit: false,
	}
	err := ValidateProduction(cfg, "custom-hash")
	if err == nil {
		t.Fatal("expected validation error for auto JWT secret")
	}
}

func TestValidateProductionPassesWithRequiredSecrets(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{
		DataDir:           dir,
		JWTSecret:         "stable-secret",
		JWTSecretExplicit: true,
		EncryptionKey:     "my-encryption-key",
	}
	if err := ValidateProduction(cfg, "custom-hash"); err != nil {
		t.Fatalf("expected pass, got %v", err)
	}
}

func TestValidateProductionPassesWithPersistedEncryptionKeyFile(t *testing.T) {
	dir := t.TempDir()
	key := make([]byte, 32)
	if err := os.WriteFile(filepath.Join(dir, "encryption.key"), []byte(hex.EncodeToString(key)), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := &Config{
		DataDir:           dir,
		JWTSecret:         "stable-secret",
		JWTSecretExplicit: true,
	}
	if err := ValidateProduction(cfg, "custom-hash"); err != nil {
		t.Fatalf("expected pass, got %v", err)
	}
}

func TestParseDevOrigins(t *testing.T) {
	got := parseDevOrigins(" http://a.test ,http://b.test ")
	if len(got) != 2 || got[0] != "http://a.test" || got[1] != "http://b.test" {
		t.Fatalf("unexpected origins: %#v", got)
	}
}
