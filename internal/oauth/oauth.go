// Package oauth implements the OAuth 2.1 details Gantry needs to be an
// authorization server for MCP clients: PKCE S256, redirect checks, and the
// public origin used as the issuer.
package oauth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const (
	// Scope is the only scope MCP clients are granted.
	Scope = "mcp"

	// AccessTokenTTL is the lifetime of an MCP access token.
	AccessTokenTTL = time.Hour

	// RefreshTokenTTL is the lifetime of a refresh token.
	RefreshTokenTTL = 30 * 24 * time.Hour

	// AuthCodeTTL is the lifetime of an authorization code.
	AuthCodeTTL = 10 * time.Minute

	// PendingTTL is how long a consent request stays valid.
	PendingTTL = 10 * time.Minute
)

var codeVerifierPattern = regexp.MustCompile(`^[A-Za-z0-9._~-]{43,128}$`)

// PublicBase returns the external origin clients use to reach Gantry.
// configured wins when set. Otherwise the scheme comes from TLS or
// X-Forwarded-Proto and the host comes from the request.
func PublicBase(r *http.Request, configured string) string {
	if configured != "" {
		return strings.TrimRight(configured, "/")
	}
	scheme := "http"
	if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

// ResourceURL is the MCP protected-resource identifier.
func ResourceURL(publicBase string) string {
	return strings.TrimRight(publicBase, "/") + "/api/v1/mcp"
}

// ProtectedResourceMetadataURL is the RFC 9728 metadata URL for the MCP resource.
func ProtectedResourceMetadataURL(publicBase string) string {
	return strings.TrimRight(publicBase, "/") + "/.well-known/oauth-protected-resource/api/v1/mcp"
}

// RandomToken returns a 32-byte raw-url-base64 secret.
func RandomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// HashSecret returns the SHA-256 hex digest of a raw secret.
func HashSecret(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// VerifyCodeChallenge checks a PKCE S256 verifier against the stored challenge.
func VerifyCodeChallenge(verifier, challenge string) error {
	if !codeVerifierPattern.MatchString(verifier) {
		return errors.New("invalid code_verifier")
	}
	sum := sha256.Sum256([]byte(verifier))
	got := base64.RawURLEncoding.EncodeToString(sum[:])
	if subtle.ConstantTimeCompare([]byte(got), []byte(challenge)) != 1 {
		return errors.New("code_verifier does not match code_challenge")
	}
	return nil
}

// ValidateRedirectURI accepts https URLs and http URLs whose host is loopback.
// Fragments and userinfo are rejected. This is the open-redirect guard; callers
// still require an exact match against the URIs registered for the client.
func ValidateRedirectURI(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" || !u.IsAbs() {
		return errors.New("redirect_uri must be an absolute URL")
	}
	if u.Fragment != "" {
		return errors.New("redirect_uri must not include a fragment")
	}
	if u.User != nil {
		return errors.New("redirect_uri must not include userinfo")
	}
	switch strings.ToLower(u.Scheme) {
	case "https":
		return nil
	case "http":
		if !isLoopback(u.Hostname()) {
			return errors.New("http redirect_uri is only allowed for localhost")
		}
		return nil
	default:
		return errors.New("redirect_uri scheme must be http or https")
	}
}

func isLoopback(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// WithQuery adds parameters to a redirect URI without dropping an existing query.
func WithQuery(raw string, extra url.Values) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	q := u.Query()
	for key, values := range extra {
		for _, value := range values {
			q.Set(key, value)
		}
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}
