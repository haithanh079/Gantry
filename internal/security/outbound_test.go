package security

import (
	"strings"
	"testing"
)

func TestValidateOutboundURLRejectsPrivateIPs(t *testing.T) {
	cases := []string{
		"http://127.0.0.1/health",
		"http://localhost/health",
		"http://10.0.0.1/internal",
		"http://192.168.1.1/",
		"http://[::1]/",
		"http://metadata.google.internal/",
	}
	for _, raw := range cases {
		if _, err := ValidateOutboundURL(raw); err == nil {
			t.Fatalf("expected error for %q", raw)
		}
	}
}

func TestValidateOutboundURLRejectsUserinfo(t *testing.T) {
	if _, err := ValidateOutboundURL("http://user:pass@example.com/"); err == nil {
		t.Fatal("expected error for userinfo in url")
	}
}

func TestValidateOutboundURLRejectsNonHTTP(t *testing.T) {
	if _, err := ValidateOutboundURL("file:///etc/passwd"); err == nil {
		t.Fatal("expected error for file scheme")
	}
}

func TestValidateOutboundURLAllowsPublicHost(t *testing.T) {
	u, err := ValidateOutboundURL("https://example.com/health")
	if err != nil {
		t.Fatalf("expected public host to be allowed, got %v", err)
	}
	if u.Hostname() != "example.com" {
		t.Fatalf("unexpected host: %s", u.Hostname())
	}
}

func TestValidateOutboundURLErrorMessages(t *testing.T) {
	_, err := ValidateOutboundURL("http://127.0.0.1/")
	if err == nil || !strings.Contains(err.Error(), "disallowed") {
		t.Fatalf("expected disallowed address error, got %v", err)
	}
}
