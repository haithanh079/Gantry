package config

import (
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DefaultBootstrapAdminPasswordHash is the bcrypt hash of the default admin password.
// Kept in sync with db.DefaultAdminPasswordHash.
const DefaultBootstrapAdminPasswordHash = "$2a$10$eMgfxZdz20Vk.9EKPJ4oP.g99eQ1JgaHQs/JH7v2fpZZykUcN1Q8y"

// ValidateProduction checks that production deployments meet minimum security
// requirements. Call after DB migration and bootstrap password initialization.
// Skipped when DevMode or InsecureAllow is true.
func ValidateProduction(cfg *Config, bootstrapAdminPasswordHash string) error {
	if cfg.DevMode || cfg.InsecureAllow {
		return nil
	}

	var problems []string

	if bootstrapAdminPasswordHash == DefaultBootstrapAdminPasswordHash {
		problems = append(problems, "bootstrap admin account still uses the default password; set GANTRY_ADMIN_PASSWORD to a strong value before production")
	}
	if !cfg.JWTSecretExplicit {
		problems = append(problems, "GANTRY_JWT_SECRET must be set to a stable value in production (sessions invalidate on every restart when auto-generated)")
	}
	if !hasEncryptionKey(cfg) {
		problems = append(problems, "GANTRY_ENCRYPTION_KEY or a persisted data/encryption.key file is required in production")
	}

	if len(problems) == 0 {
		return nil
	}

	var b strings.Builder
	b.WriteString("production security validation failed:\n")
	for _, p := range problems {
		b.WriteString("  - ")
		b.WriteString(p)
		b.WriteByte('\n')
	}
	b.WriteString("Set GANTRY_INSECURE_ALLOW=true only for intentionally insecure dev/staging environments.")
	return fmt.Errorf("%s", strings.TrimSpace(b.String()))
}

func hasEncryptionKey(cfg *Config) bool {
	if cfg.EncryptionKey != "" {
		return true
	}
	keyPath := filepath.Join(cfg.DataDir, "encryption.key")
	data, err := os.ReadFile(keyPath)
	if err != nil {
		return false
	}
	key, err := hex.DecodeString(strings.TrimSpace(string(data)))
	return err == nil && len(key) == 32
}
