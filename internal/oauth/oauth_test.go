package oauth

import (
	"crypto/sha256"
	"encoding/base64"
	"testing"
)

func TestVerifyCodeChallengeRFC7636(t *testing.T) {
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	if err := VerifyCodeChallenge(verifier, challenge); err != nil {
		t.Fatal(err)
	}
	if err := VerifyCodeChallenge(verifier, challenge+"x"); err == nil {
		t.Fatal("expected verifier mismatch")
	}
	if err := VerifyCodeChallenge("short", challenge); err == nil {
		t.Fatal("expected short verifier to fail")
	}
}

func TestValidateRedirectURI(t *testing.T) {
	allowed := []string{
		"http://localhost:8787/callback",
		"http://127.0.0.1:8787/callback",
		"http://[::1]:8787/callback",
		"https://www.cursor.com/agents/mcp/oauth/callback",
	}
	for _, uri := range allowed {
		if err := ValidateRedirectURI(uri); err != nil {
			t.Errorf("%s: %v", uri, err)
		}
	}
	rejected := []string{
		"http://evil.com/callback",
		"http://localhost.evil.com/callback",
		"javascript:alert(1)",
		"http://user:pass@localhost/callback",
		"https://example.com/callback#frag",
		"/relative",
	}
	for _, uri := range rejected {
		if err := ValidateRedirectURI(uri); err == nil {
			t.Errorf("expected reject for %s", uri)
		}
	}
}
