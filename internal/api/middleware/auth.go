package middleware

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/go2engle/gantry/internal/auth"
	"github.com/go2engle/gantry/internal/db"
)

type contextKey string

const claimsKey contextKey = "claims"
const effectiveRoleKey contextKey = "effectiveRole"

// RequireAuth returns middleware that validates the Bearer token in the
// Authorization header. It accepts both JWT tokens and long-lived API keys
// (tokens prefixed with "gantry_"). Claims are stored in the request context.
// The effective role (accounting for group memberships) is also computed and stored.
func RequireAuth(authSvc *auth.Service, database *db.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, ok := authenticateRequest(r, authSvc, database, false, w)
			if !ok {
				return
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireWebSocketAuth validates a WebSocket handshake using the standard
// Authorization header or the browser session cookie.
func RequireWebSocketAuth(authSvc *auth.Service, database *db.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, ok := authenticateRequest(r, authSvc, database, false, w)
			if !ok {
				return
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireRole returns middleware that checks whether the authenticated user's
// effective role (accounting for group memberships) meets the minimum required
// role according to the hierarchy: admin > platform-engineer > developer > viewer.
func RequireRole(role string) func(http.Handler) http.Handler {
	requiredLevel := auth.RoleLevel(role)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := GetClaims(r.Context())
			if claims == nil {
				http.Error(w, `{"error":"authentication required"}`, http.StatusUnauthorized)
				return
			}

			effective := GetEffectiveRole(r.Context())
			userLevel := auth.RoleLevel(effective)
			if userLevel < requiredLevel {
				http.Error(w, `{"error":"insufficient permissions"}`, http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// GetClaims extracts the auth.Claims from the request context.
// It returns nil if no claims are present.
func GetClaims(ctx context.Context) *auth.Claims {
	claims, _ := ctx.Value(claimsKey).(*auth.Claims)
	return claims
}

// GetEffectiveRole returns the effective role from the request context.
// Falls back to the claims role if no effective role is set.
func GetEffectiveRole(ctx context.Context) string {
	if role, ok := ctx.Value(effectiveRoleKey).(string); ok && role != "" {
		return role
	}
	if claims := GetClaims(ctx); claims != nil {
		return claims.Role
	}
	return ""
}

func authenticateRequest(r *http.Request, authSvc *auth.Service, database *db.DB, allowQueryToken bool, w http.ResponseWriter) (context.Context, bool) {
	token, errMsg := tokenFromRequest(r, allowQueryToken)
	if errMsg != "" {
		http.Error(w, errMsg, http.StatusUnauthorized)
		return nil, false
	}

	claims, effectiveRole, errMsg := authenticateToken(r, authSvc, database, token)
	if errMsg != "" {
		http.Error(w, errMsg, http.StatusUnauthorized)
		return nil, false
	}

	ctx := context.WithValue(r.Context(), claimsKey, claims)
	ctx = context.WithValue(ctx, effectiveRoleKey, effectiveRole)
	return ctx, true
}

func tokenFromRequest(r *http.Request, allowQueryToken bool) (string, string) {
	header := r.Header.Get("Authorization")
	if header != "" {
		parts := strings.SplitN(header, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			return "", `{"error":"invalid authorization header format"}`
		}
		return parts[1], ""
	}

	if allowQueryToken {
		if token := r.URL.Query().Get("token"); token != "" {
			return token, ""
		}
	}

	if c, err := r.Cookie(auth.SessionCookieName); err == nil && c.Value != "" {
		return c.Value, ""
	}

	return "", `{"error":"missing authentication credentials"}`
}

func authenticateToken(r *http.Request, authSvc *auth.Service, database *db.DB, token string) (*auth.Claims, string, string) {
	if strings.HasPrefix(token, auth.APIKeyPrefix) {
		keyHash := auth.HashAPIKey(token)
		apiKey, err := database.GetAPIKeyByHash(r.Context(), keyHash)
		if err != nil {
			return nil, "", `{"error":"invalid or revoked api key"}`
		}
		if apiKey.ExpiresAt != nil && apiKey.ExpiresAt.Before(time.Now().UTC()) {
			return nil, "", `{"error":"invalid or revoked api key"}`
		}

		claims := &auth.Claims{
			UserID:   apiKey.UserID,
			Username: "apikey:" + apiKey.Name,
			Role:     apiKey.Role,
		}
		// API keys must honor their stored role exactly so they can be safely
		// down-scoped relative to the owning user's account.
		return claims, apiKey.Role, ""
	}

	claims, err := authSvc.ValidateToken(token)
	if err != nil {
		return nil, "", `{"error":"invalid or expired token"}`
	}
	// MCP OAuth access tokens are bound to the MCP resource and must not
	// authenticate the REST API or the browser session.
	if claims.TokenUse == auth.MCPTokenUse {
		return nil, "", `{"error":"invalid or expired token"}`
	}

	return claims, effectiveRoleForClaims(r.Context(), database, claims), ""
}

func effectiveRoleForClaims(ctx context.Context, database *db.DB, claims *auth.Claims) string {
	effectiveRole := claims.Role
	if claims.UserID == "" {
		return effectiveRole
	}
	groups, err := database.ListUserGroups(ctx, claims.UserID)
	if err == nil && len(groups) > 0 {
		groupRoles := make([]string, len(groups))
		for i, g := range groups {
			groupRoles[i] = g.Role
		}
		effectiveRole = auth.EffectiveRole(claims.Role, groupRoles)
	}
	return effectiveRole
}
