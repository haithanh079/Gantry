// Package security provides outbound request safety helpers (SSRF mitigation).
package security

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

var blockedHostnames = map[string]bool{
	"localhost":                 true,
	"metadata.google.internal": true,
}

// ValidateOutboundURL parses raw and ensures the target is safe for server-side
// HTTP requests (no private/link-local addresses, no credentials in URL).
func ValidateOutboundURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("url scheme must be http or https")
	}
	if u.Host == "" {
		return nil, fmt.Errorf("url must include a host")
	}
	if u.User != nil {
		return nil, fmt.Errorf("url must not contain userinfo")
	}

	host := u.Hostname()
	if blockedHostnames[strings.ToLower(host)] {
		return nil, fmt.Errorf("url host %q is not allowed", host)
	}

	ips, err := net.LookupIP(host)
	if err != nil {
		return nil, fmt.Errorf("resolving host %q: %w", host, err)
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("host %q resolved to no addresses", host)
	}

	for _, ip := range ips {
		if isBlockedIP(ip) {
			return nil, fmt.Errorf("url resolves to disallowed address %s", ip.String())
		}
	}

	return u, nil
}

func isBlockedIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
		return true
	}
	// AWS/GCP metadata and other link-local services.
	if ip4 := ip.To4(); ip4 != nil && ip4[0] == 169 && ip4[1] == 254 {
		return true
	}
	return false
}
