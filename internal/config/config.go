// Package config handles Gantry server configuration.
// Configuration is loaded from environment variables with sensible defaults
// suitable for single-binary deployment.
package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// FileConfig represents configuration fields that can be set in gantry.yaml.
// Precedence: CLI flags > environment variables > config file > built-in defaults.
type FileConfig struct {
	Port          int    `yaml:"port"`
	DB            string `yaml:"db"`
	Dev           bool   `yaml:"dev"`
	AdminPassword string `yaml:"adminPassword"`
	JWTSecret     string `yaml:"jwtSecret"`
	DataDir       string `yaml:"dataDir"`
	EncryptionKey string `yaml:"encryptionKey"`
	PublicURL     string `yaml:"publicUrl"`
}

// loadFileConfig reads a YAML config file and returns a FileConfig.
// Returns nil (without error) if the file does not exist.
func loadFileConfig(path string) (*FileConfig, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading config file %s: %w", path, err)
	}
	var fc FileConfig
	if err := yaml.Unmarshal(data, &fc); err != nil {
		return nil, fmt.Errorf("parsing config file %s: %w", path, err)
	}
	return &fc, nil
}

// applyFileConfig overlays non-zero fields from fc onto cfg.
// This is called after defaults but before env-var overrides so that the
// precedence order (flags > env > file > defaults) is maintained by callers.
func applyFileConfig(cfg *Config, fc *FileConfig) {
	if fc == nil {
		return
	}
	if fc.Port > 0 {
		cfg.Port = fc.Port
	}
	if fc.DataDir != "" {
		cfg.DataDir = fc.DataDir
	}
	if fc.DB != "" {
		if strings.HasPrefix(fc.DB, "postgres://") || strings.HasPrefix(fc.DB, "postgresql://") {
			cfg.DBType = "postgres"
			cfg.DBDSN = fc.DB
		} else {
			cfg.DBType = "sqlite"
			cfg.DBDSN = fc.DB
		}
	}
	if fc.Dev {
		cfg.DevMode = true
	}
	if fc.AdminPassword != "" {
		cfg.AdminPassword = fc.AdminPassword
	}
	if fc.JWTSecret != "" {
		cfg.JWTSecret = fc.JWTSecret
		cfg.JWTSecretExplicit = true
	}
	if fc.EncryptionKey != "" {
		cfg.EncryptionKey = fc.EncryptionKey
	}
	if fc.PublicURL != "" {
		cfg.PublicURL = fc.PublicURL
	}
}

// Config holds all Gantry configuration.
type Config struct {
	Port              int      // HTTP listen port
	DBType            string   // "sqlite" or "postgres"
	DBDSN             string   // connection string or file path
	DevMode           bool     // development mode (verbose logging, CORS *, etc.)
	AdminPassword     string   // initial admin password (only used on first run)
	JWTSecret         string   // secret for signing JWT tokens
	JWTSecretExplicit bool     // true when JWTSecret was set via env/file/flag (not auto-generated)
	DataDir           string   // directory for SQLite database and other data
	EncryptionKey     string   // key for AES-256-GCM encryption of DB secrets (GANTRY_ENCRYPTION_KEY)
	InsecureAllow     bool     // skip production security validation (GANTRY_INSECURE_ALLOW)
	MetricsToken      string   // optional bearer token for /metrics (GANTRY_METRICS_TOKEN)
	DevOrigins        []string // allowed CORS origins in dev mode (GANTRY_DEV_ORIGINS)
	PublicURL         string   // external origin for OAuth issuer and MCP resource URLs
	publicURLErr      error
}

// Default returns a Config with sensible defaults for local development.
func Default() *Config {
	return &Config{
		Port:          8080,
		DBType:        "sqlite",
		DBDSN:         "", // set from DataDir in normalize()
		DevMode:       false,
		AdminPassword: "changeme",
		JWTSecret:     "", // generated if empty in normalize()
		DataDir:       "./data",
	}
}

// Load reads configuration with precedence: env vars > gantry.yaml > defaults.
// If a gantry.yaml file exists in the current directory it is applied before
// env var overrides.
func Load() *Config {
	return LoadWithFile("gantry.yaml")
}

// LoadWithFile loads configuration from the given YAML file path (if it exists),
// then overlays environment variable overrides on top.
// Precedence: env vars > config file > built-in defaults.
func LoadWithFile(configPath string) *Config {
	cfg := Default()

	// Apply config file values (lower priority than env vars).
	if fc, err := loadFileConfig(configPath); err == nil {
		applyFileConfig(cfg, fc)
	}

	applyEnv(cfg)
	cfg.normalize()
	if cfg.DevMode && len(cfg.DevOrigins) == 0 {
		cfg.DevOrigins = DefaultDevOrigins()
	}
	return cfg
}

// applyEnv overlays environment variable values onto cfg.
func applyEnv(cfg *Config) {
	if v := os.Getenv("GANTRY_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil && port > 0 && port <= 65535 {
			cfg.Port = port
		}
	}

	if v := os.Getenv("GANTRY_DATA_DIR"); v != "" {
		cfg.DataDir = v
	}

	if v := os.Getenv("GANTRY_DB"); v != "" {
		if strings.HasPrefix(v, "postgres://") || strings.HasPrefix(v, "postgresql://") {
			cfg.DBType = "postgres"
			cfg.DBDSN = v
		} else {
			cfg.DBType = "sqlite"
			cfg.DBDSN = v
		}
	}

	if v := os.Getenv("GANTRY_DEV"); v != "" {
		v = strings.ToLower(v)
		cfg.DevMode = v == "true" || v == "1" || v == "yes"
	}

	if v := os.Getenv("GANTRY_ADMIN_PASSWORD"); v != "" {
		cfg.AdminPassword = v
	}

	if v := os.Getenv("GANTRY_JWT_SECRET"); v != "" {
		cfg.JWTSecret = v
		cfg.JWTSecretExplicit = true
	}

	if v := os.Getenv("GANTRY_ENCRYPTION_KEY"); v != "" {
		cfg.EncryptionKey = v
	}

	if v := os.Getenv("GANTRY_INSECURE_ALLOW"); v != "" {
		v = strings.ToLower(v)
		cfg.InsecureAllow = v == "true" || v == "1" || v == "yes"
	}

	if v := os.Getenv("GANTRY_METRICS_TOKEN"); v != "" {
		cfg.MetricsToken = v
	}

	if v := os.Getenv("GANTRY_DEV_ORIGINS"); v != "" {
		cfg.DevOrigins = parseDevOrigins(v)
	}

	if v := os.Getenv("GANTRY_PUBLIC_URL"); v != "" {
		cfg.PublicURL = v
	}
}

func parseDevOrigins(raw string) []string {
	var origins []string
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			origins = append(origins, part)
		}
	}
	return origins
}

// DefaultDevOrigins returns the default Vite dev server origins.
func DefaultDevOrigins() []string {
	return []string{"http://localhost:5173", "http://127.0.0.1:5173"}
}

// normalize fills in derived values and generates secrets where needed.
func (c *Config) normalize() {
	// Default SQLite DSN based on data directory.
	if c.DBDSN == "" {
		c.DBDSN = filepath.Join(c.DataDir, "gantry.db")
	}

	// Generate a random JWT secret if none was provided.
	if c.JWTSecret == "" {
		c.JWTSecret = generateRandomHex(32)
	}

	if c.PublicURL != "" {
		normalized, err := NormalizePublicURL(c.PublicURL)
		if err != nil {
			c.publicURLErr = err
		} else {
			c.PublicURL = normalized
		}
	}
}

// PublicURLError reports an invalid GANTRY_PUBLIC_URL / publicUrl value.
func (c *Config) PublicURLError() error {
	return c.publicURLErr
}

// NormalizePublicURL checks that raw is an absolute http(s) origin without a path.
func NormalizePublicURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil {
		return "", fmt.Errorf("GANTRY_PUBLIC_URL must be an absolute http(s) origin")
	}
	if u.Path != "" && u.Path != "/" {
		return "", fmt.Errorf("GANTRY_PUBLIC_URL must not include a path")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("GANTRY_PUBLIC_URL must not include a query or fragment")
	}
	u.Path = ""
	u.RawQuery = ""
	u.Fragment = ""
	return strings.TrimRight(u.String(), "/"), nil
}

// DSN returns the database connection string appropriate for the configured driver.
// For SQLite it returns a URI with WAL mode and foreign keys enabled.
// For PostgreSQL it returns the DSN as-is.
func (c *Config) DSN() string {
	if c.DBType == "sqlite" {
		return fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)", c.DBDSN)
	}
	return c.DBDSN
}

// DriverName returns the database/sql driver name for the configured database type.
func (c *Config) DriverName() string {
	if c.DBType == "postgres" {
		return "postgres"
	}
	return "sqlite"
}

// generateRandomHex produces a cryptographically random hex string of n bytes.
func generateRandomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		// This should never happen on a properly functioning OS.
		panic(fmt.Sprintf("crypto/rand failed: %v", err))
	}
	return hex.EncodeToString(b)
}
