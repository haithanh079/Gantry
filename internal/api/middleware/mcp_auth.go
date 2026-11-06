package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/go2engle/gantry/internal/auth"
	"github.com/go2engle/gantry/internal/db"
	"github.com/go2engle/gantry/internal/oauth"
)

// RequireMCPAuth authenticates the MCP endpoint.
// It accepts API keys, browser session JWTs, and MCP OAuth access tokens.
// A missing or invalid credential is a 401 with a WWW-Authenticate challenge
// so MCP clients can start the authorization-code grant.
func RequireMCPAuth(authSvc *auth.Service, database *db.DB, publicURL string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			base := oauth.PublicBase(r, publicURL)
			metadataURL := oauth.ProtectedResourceMetadataURL(base)
			resource := oauth.ResourceURL(base)

			token, errMsg := tokenFromRequest(r, false)
			if errMsg != "" {
				writeMCPUnauthorized(w, metadataURL, "missing authentication credentials")
				return
			}

			claims, effective, errMsg := authenticateMCPToken(r, authSvc, database, token, base, resource)
			if errMsg != "" {
				if strings.Contains(errMsg, "insufficient scope") {
					writeMCPForbidden(w, metadataURL, "insufficient scope")
					return
				}
				writeMCPUnauthorized(w, metadataURL, plainAuthError(errMsg))
				return
			}

			ctx := context.WithValue(r.Context(), claimsKey, claims)
			ctx = context.WithValue(ctx, effectiveRoleKey, effective)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func authenticateMCPToken(r *http.Request, authSvc *auth.Service, database *db.DB, token, issuer, resource string) (*auth.Claims, string, string) {
	if strings.HasPrefix(token, auth.APIKeyPrefix) {
		return authenticateToken(r, authSvc, database, token)
	}

	claims, err := authSvc.ValidateToken(token)
	if err != nil {
		return nil, "", `{"error":"invalid or expired token"}`
	}

	switch claims.TokenUse {
	case auth.MCPTokenUse:
		if claims.Issuer != issuer || !claims.HasAudience(resource) {
			return nil, "", `{"error":"invalid or expired token"}`
		}
		if claims.Scope != "" && !scopeContains(claims.Scope, auth.MCPScope) {
			return nil, "", `{"error":"insufficient scope"}`
		}
	case "":
		// Browser session JWT.
	default:
		return nil, "", `{"error":"invalid or expired token"}`
	}

	return claims, effectiveRoleForClaims(r.Context(), database, claims), ""
}

func scopeContains(scope, want string) bool {
	for _, part := range strings.Fields(scope) {
		if part == want {
			return true
		}
	}
	return false
}

func plainAuthError(msg string) string {
	var body struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(msg), &body); err == nil && body.Error != "" {
		return body.Error
	}
	return msg
}

func writeMCPUnauthorized(w http.ResponseWriter, metadataURL, msg string) {
	w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer realm="gantry", resource_metadata=%q`, metadataURL))
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func writeMCPForbidden(w http.ResponseWriter, metadataURL, msg string) {
	w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer realm="gantry", resource_metadata=%q, scope=%q, error=%q`, metadataURL, auth.MCPScope, "insufficient_scope"))
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
