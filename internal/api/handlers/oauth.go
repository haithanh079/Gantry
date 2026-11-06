package handlers

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"

	mcpauth "github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/oauthex"

	"github.com/go2engle/gantry/internal/api/middleware"
	"github.com/go2engle/gantry/internal/auth"
	"github.com/go2engle/gantry/internal/db"
	"github.com/go2engle/gantry/internal/oauth"
)

type oauthRegisterRequest struct {
	RedirectURIs            []string `json:"redirect_uris"`
	ClientName              string   `json:"client_name"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
	GrantTypes              []string `json:"grant_types"`
	ResponseTypes           []string `json:"response_types"`
	Scope                   string   `json:"scope"`
}

type oauthRegisterResponse struct {
	ClientID                string   `json:"client_id"`
	ClientIDIssuedAt        int64    `json:"client_id_issued_at"`
	ClientName              string   `json:"client_name,omitempty"`
	RedirectURIs            []string `json:"redirect_uris"`
	GrantTypes              []string `json:"grant_types"`
	ResponseTypes           []string `json:"response_types"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
	Scope                   string   `json:"scope"`
}

type oauthTokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"`
	Scope        string `json:"scope"`
}

type oauthConsentRequest struct {
	RID      string `json:"rid"`
	Decision string `json:"decision"`
}

// OAuthProtectedResource serves RFC 9728 metadata for the MCP resource.
func (h *Handlers) OAuthProtectedResource(w http.ResponseWriter, r *http.Request) {
	base := h.publicBase(r)
	meta := &oauthex.ProtectedResourceMetadata{
		Resource:               oauth.ResourceURL(base),
		AuthorizationServers:   []string{base},
		ScopesSupported:        []string{oauth.Scope},
		BearerMethodsSupported: []string{"header"},
		ResourceName:           "Gantry",
	}
	mcpauth.ProtectedResourceMetadataHandler(meta).ServeHTTP(w, r)
}

// OAuthAuthorizationServer serves RFC 8414 metadata.
func (h *Handlers) OAuthAuthorizationServer(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		h.OAuthOptions(w, r)
		return
	}
	base := h.publicBase(r)
	setOAuthCORS(w)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"issuer":                                base,
		"authorization_endpoint":                base + "/oauth/authorize",
		"token_endpoint":                        base + "/oauth/token",
		"registration_endpoint":                 base + "/oauth/register",
		"scopes_supported":                      []string{oauth.Scope},
		"response_types_supported":              []string{"code"},
		"grant_types_supported":                 []string{"authorization_code", "refresh_token"},
		"token_endpoint_auth_methods_supported": []string{"none"},
		"code_challenge_methods_supported":      []string{"S256"},
	})
}

// OAuthOptions answers CORS preflight for the public OAuth endpoints.
func (h *Handlers) OAuthOptions(w http.ResponseWriter, r *http.Request) {
	setOAuthCORS(w)
	w.WriteHeader(http.StatusNoContent)
}

// OAuthRegister implements dynamic client registration for public clients.
func (h *Handlers) OAuthRegister(w http.ResponseWriter, r *http.Request) {
	setOAuthCORS(w)
	var req oauthRegisterRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	if err := dec.Decode(&req); err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_client_metadata", "invalid JSON body")
		return
	}

	if req.TokenEndpointAuthMethod != "" && req.TokenEndpointAuthMethod != "none" {
		writeOAuthError(w, http.StatusBadRequest, "invalid_client_metadata", "only public clients (token_endpoint_auth_method=none) are supported")
		return
	}
	if err := supportedList(req.GrantTypes, "authorization_code", "refresh_token"); err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_client_metadata", "unsupported grant_types")
		return
	}
	if err := supportedList(req.ResponseTypes, "code"); err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_client_metadata", "unsupported response_types")
		return
	}
	if strings.TrimSpace(req.Scope) != "" {
		if _, err := normalizeOAuthScope(req.Scope); err != nil {
			writeOAuthError(w, http.StatusBadRequest, "invalid_client_metadata", err.Error())
			return
		}
	}
	if len(req.RedirectURIs) == 0 {
		writeOAuthError(w, http.StatusBadRequest, "invalid_redirect_uri", "redirect_uris is required")
		return
	}
	for _, uri := range req.RedirectURIs {
		if err := oauth.ValidateRedirectURI(uri); err != nil {
			writeOAuthError(w, http.StatusBadRequest, "invalid_redirect_uri", err.Error())
			return
		}
	}

	id, err := newOAuthClientID()
	if err != nil {
		writeOAuthError(w, http.StatusInternalServerError, "server_error", "failed to create client")
		return
	}
	client := &db.OAuthClient{
		ID:           id,
		Name:         sanitizeClientName(req.ClientName),
		RedirectURIs: req.RedirectURIs,
		CreatedAt:    time.Now().UTC(),
	}
	if err := h.DB.CreateOAuthClient(r.Context(), client); err != nil {
		writeOAuthError(w, http.StatusInternalServerError, "server_error", "failed to store client")
		return
	}

	writeOAuthJSON(w, http.StatusCreated, oauthRegisterResponse{
		ClientID:                client.ID,
		ClientIDIssuedAt:        client.CreatedAt.Unix(),
		ClientName:              client.Name,
		RedirectURIs:            client.RedirectURIs,
		GrantTypes:              []string{"authorization_code", "refresh_token"},
		ResponseTypes:           []string{"code"},
		TokenEndpointAuthMethod: "none",
		Scope:                   oauth.Scope,
	})
}

// OAuthAuthorize validates an authorization-code request and sends the browser
// to the Gantry consent page (after login, if needed).
func (h *Handlers) OAuthAuthorize(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	clientID := q.Get("client_id")
	redirectURI := q.Get("redirect_uri")
	state := q.Get("state")
	responseType := q.Get("response_type")
	challenge := q.Get("code_challenge")
	method := q.Get("code_challenge_method")

	client, err := h.DB.GetOAuthClient(r.Context(), clientID)
	if err != nil || client == nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_client", "unknown client_id")
		return
	}
	if oauth.ValidateRedirectURI(redirectURI) != nil || !clientAllowsRedirect(client, redirectURI) {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "redirect_uri is not registered for this client")
		return
	}

	fail := func(code, desc string) {
		h.redirectOAuthError(w, r, redirectURI, state, code, desc)
	}
	if responseType != "code" {
		fail("unsupported_response_type", "response_type must be code")
		return
	}
	if method != "S256" || challenge == "" {
		fail("invalid_request", "PKCE S256 code_challenge is required")
		return
	}
	scope, err := normalizeOAuthScope(q.Get("scope"))
	if err != nil {
		fail("invalid_scope", err.Error())
		return
	}
	resource, err := normalizeOAuthResource(q.Get("resource"), oauth.ResourceURL(h.publicBase(r)))
	if err != nil {
		fail("invalid_target", err.Error())
		return
	}

	rid, err := randomHex(32)
	if err != nil {
		fail("server_error", "failed to start authorization")
		return
	}
	pending := &db.OAuthPending{
		ID:            rid,
		ClientID:      client.ID,
		RedirectURI:   redirectURI,
		CodeChallenge: challenge,
		Resource:      resource,
		Scope:         scope,
		State:         state,
		ExpiresAt:     time.Now().UTC().Add(oauth.PendingTTL),
	}
	if err := h.DB.CreateOAuthPending(r.Context(), pending); err != nil {
		fail("server_error", "failed to start authorization")
		return
	}

	next := "/oauth/consent?rid=" + url.QueryEscape(rid)
	if _, ok := h.browserSession(r); !ok {
		http.Redirect(w, r, "/login?next="+url.QueryEscape(next), http.StatusFound)
		return
	}
	http.Redirect(w, r, next, http.StatusFound)
}

// OAuthPending describes a consent request to the logged-in user.
func (h *Handlers) OAuthPending(w http.ResponseWriter, r *http.Request) {
	pending, client, err := h.loadPending(r, r.URL.Query().Get("rid"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"rid":         pending.ID,
		"clientId":    client.ID,
		"clientName":  displayClientName(client.Name),
		"scope":       pending.Scope,
		"redirectURI": pending.RedirectURI,
	})
}

// OAuthConsent records a grant or denial and returns the client redirect URL.
func (h *Handlers) OAuthConsent(w http.ResponseWriter, r *http.Request) {
	var req oauthConsentRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	pending, _, err := h.loadPending(r, req.RID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	decision := strings.ToLower(strings.TrimSpace(req.Decision))
	if decision == "deny" {
		_ = h.DB.DeleteOAuthPending(r.Context(), pending.ID)
		h.writeOAuthAudit(r, claims, "oauth.denied", pending.ClientID)
		target, err := oauth.WithQuery(pending.RedirectURI, oauthErrorQuery("access_denied", "the user denied the request", pending.State))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid redirect_uri")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"redirectUrl": target})
		return
	}
	if decision != "grant" {
		writeError(w, http.StatusBadRequest, "decision must be grant or deny")
		return
	}

	rawCode, err := oauth.RandomToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create authorization code")
		return
	}
	if err := h.DB.CreateOAuthAuthCode(r.Context(), &db.OAuthAuthCode{
		CodeHash:      oauth.HashSecret(rawCode),
		ClientID:      pending.ClientID,
		UserID:        claims.UserID,
		RedirectURI:   pending.RedirectURI,
		CodeChallenge: pending.CodeChallenge,
		Resource:      pending.Resource,
		Scope:         pending.Scope,
		ExpiresAt:     time.Now().UTC().Add(oauth.AuthCodeTTL),
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create authorization code")
		return
	}
	_ = h.DB.DeleteOAuthPending(r.Context(), pending.ID)
	h.writeOAuthAudit(r, claims, "oauth.granted", pending.ClientID)

	q := url.Values{"code": {rawCode}}
	if pending.State != "" {
		q.Set("state", pending.State)
	}
	target, err := oauth.WithQuery(pending.RedirectURI, q)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid redirect_uri")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"redirectUrl": target})
}

// OAuthToken exchanges an authorization code or refresh token for an access token.
func (h *Handlers) OAuthToken(w http.ResponseWriter, r *http.Request) {
	setOAuthCORS(w)
	form, err := readOAuthForm(w, r)
	if err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "invalid request body")
		return
	}
	switch form.Get("grant_type") {
	case "authorization_code":
		h.oauthAuthorizationCodeGrant(w, r, form)
	case "refresh_token":
		h.oauthRefreshGrant(w, r, form)
	default:
		writeOAuthError(w, http.StatusBadRequest, "unsupported_grant_type", "grant_type must be authorization_code or refresh_token")
	}
}

func (h *Handlers) oauthAuthorizationCodeGrant(w http.ResponseWriter, r *http.Request, form url.Values) {
	rawCode := form.Get("code")
	clientID := form.Get("client_id")
	redirectURI := form.Get("redirect_uri")
	verifier := form.Get("code_verifier")
	if rawCode == "" || clientID == "" || redirectURI == "" || verifier == "" {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "code, client_id, redirect_uri, and code_verifier are required")
		return
	}

	code, err := h.DB.GetOAuthAuthCode(r.Context(), oauth.HashSecret(rawCode))
	if err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "invalid authorization code")
		return
	}
	if code.Used || time.Now().UTC().After(code.ExpiresAt) || code.ClientID != clientID || code.RedirectURI != redirectURI {
		_ = h.DB.ConsumeOAuthAuthCode(r.Context(), code.CodeHash)
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "invalid authorization code")
		return
	}
	resource, err := normalizeOAuthResource(form.Get("resource"), code.Resource)
	if err != nil || resource != code.Resource {
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "resource does not match the authorization code")
		return
	}

	pkceErr := oauth.VerifyCodeChallenge(verifier, code.CodeChallenge)
	if consumeErr := h.DB.ConsumeOAuthAuthCode(r.Context(), code.CodeHash); consumeErr != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "authorization code already used")
		return
	}
	if pkceErr != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", pkceErr.Error())
		return
	}

	h.issueOAuthTokens(w, r, code.ClientID, code.UserID, code.Scope, code.Resource, false, "")
}

func (h *Handlers) oauthRefreshGrant(w http.ResponseWriter, r *http.Request, form url.Values) {
	raw := form.Get("refresh_token")
	clientID := form.Get("client_id")
	if raw == "" || clientID == "" {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "refresh_token and client_id are required")
		return
	}
	current, err := h.DB.GetOAuthRefreshToken(r.Context(), oauth.HashSecret(raw))
	if err != nil || current.Revoked || time.Now().UTC().After(current.ExpiresAt) || current.ClientID != clientID {
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "invalid refresh token")
		return
	}
	if form.Get("resource") != "" {
		resource, err := normalizeOAuthResource(form.Get("resource"), current.Resource)
		if err != nil || resource != current.Resource {
			writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "resource does not match the refresh token")
			return
		}
	}
	h.issueOAuthTokens(w, r, current.ClientID, current.UserID, current.Scope, current.Resource, true, current.TokenHash)
}

func (h *Handlers) issueOAuthTokens(w http.ResponseWriter, r *http.Request, clientID, userID, scope, resource string, rotating bool, oldRefreshHash string) {
	user, err := h.DB.GetUserByID(r.Context(), userID)
	if err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "user no longer exists")
		return
	}
	access, err := h.Auth.GenerateMCPAccessToken(&auth.User{
		ID:       user.ID,
		Username: user.Username,
		Role:     user.Role,
	}, h.publicBase(r), resource, oauth.AccessTokenTTL)
	if err != nil {
		writeOAuthError(w, http.StatusInternalServerError, "server_error", "failed to issue access token")
		return
	}
	rawRefresh, err := oauth.RandomToken()
	if err != nil {
		writeOAuthError(w, http.StatusInternalServerError, "server_error", "failed to issue refresh token")
		return
	}
	next := &db.OAuthRefreshToken{
		TokenHash: oauth.HashSecret(rawRefresh),
		ClientID:  clientID,
		UserID:    user.ID,
		Scope:     scope,
		Resource:  resource,
		ExpiresAt: time.Now().UTC().Add(oauth.RefreshTokenTTL),
	}
	if rotating {
		if err := h.DB.RotateOAuthRefreshToken(r.Context(), oldRefreshHash, next); err != nil {
			writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "invalid refresh token")
			return
		}
	} else if err := h.DB.CreateOAuthRefreshToken(r.Context(), next); err != nil {
		writeOAuthError(w, http.StatusInternalServerError, "server_error", "failed to store refresh token")
		return
	}

	h.writeOAuthAudit(r, &auth.Claims{UserID: user.ID, Username: user.Username}, "oauth.token_issued", clientID)
	writeOAuthJSON(w, http.StatusOK, oauthTokenResponse{
		AccessToken:  access,
		TokenType:    "Bearer",
		ExpiresIn:    int(oauth.AccessTokenTTL.Seconds()),
		RefreshToken: rawRefresh,
		Scope:        scope,
	})
}

func (h *Handlers) loadPending(r *http.Request, rid string) (*db.OAuthPending, *db.OAuthClient, error) {
	if rid == "" {
		return nil, nil, errors.New("missing authorization request")
	}
	pending, err := h.DB.GetOAuthPending(r.Context(), rid)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil, errors.New("authorization request not found or already used")
		}
		return nil, nil, errors.New("failed to load authorization request")
	}
	if time.Now().UTC().After(pending.ExpiresAt) {
		_ = h.DB.DeleteOAuthPending(r.Context(), pending.ID)
		return nil, nil, errors.New("authorization request expired")
	}
	client, err := h.DB.GetOAuthClient(r.Context(), pending.ClientID)
	if err != nil {
		return nil, nil, errors.New("oauth client not found")
	}
	return pending, client, nil
}

func (h *Handlers) browserSession(r *http.Request) (*auth.Claims, bool) {
	c, err := r.Cookie(auth.SessionCookieName)
	if err != nil || c.Value == "" {
		return nil, false
	}
	claims, err := h.Auth.ValidateToken(c.Value)
	if err != nil || claims.TokenUse == auth.MCPTokenUse {
		return nil, false
	}
	return claims, true
}

func (h *Handlers) redirectOAuthError(w http.ResponseWriter, r *http.Request, redirectURI, state, code, desc string) {
	target, err := oauth.WithQuery(redirectURI, oauthErrorQuery(code, desc, state))
	if err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "invalid redirect_uri")
		return
	}
	http.Redirect(w, r, target, http.StatusFound)
}

func (h *Handlers) writeOAuthAudit(r *http.Request, claims *auth.Claims, action, clientID string) {
	userID := ""
	userName := ""
	if claims != nil {
		userID = claims.UserID
		userName = claims.Username
	}
	_ = h.DB.CreateAuditEntry(r.Context(), &db.AuditEntry{
		UserID:       userID,
		UserName:     userName,
		Action:       action,
		ResourceType: "oauth_client",
		ResourceName: clientID,
		Source:       "oauth",
		IPAddress:    clientIP(r),
	})
}

func oauthErrorQuery(code, desc, state string) url.Values {
	q := url.Values{"error": {code}}
	if desc != "" {
		q.Set("error_description", desc)
	}
	if state != "" {
		q.Set("state", state)
	}
	return q
}

func clientAllowsRedirect(client *db.OAuthClient, redirectURI string) bool {
	for _, uri := range client.RedirectURIs {
		if uri == redirectURI {
			return true
		}
	}
	return false
}

func normalizeOAuthScope(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return oauth.Scope, nil
	}
	for _, part := range strings.Fields(raw) {
		if part != oauth.Scope {
			return "", errors.New("unsupported scope")
		}
	}
	return oauth.Scope, nil
}

func normalizeOAuthResource(raw, expected string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return expected, nil
	}
	if strings.TrimRight(raw, "/") != strings.TrimRight(expected, "/") {
		return "", errors.New("unsupported resource")
	}
	return expected, nil
}

func supportedList(got []string, allowed ...string) error {
	allow := map[string]bool{}
	for _, item := range allowed {
		allow[item] = true
	}
	for _, item := range got {
		if !allow[item] {
			return errors.New("unsupported value")
		}
	}
	return nil
}

func sanitizeClientName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "MCP client"
	}
	var b strings.Builder
	for _, r := range name {
		if unicode.IsControl(r) {
			continue
		}
		b.WriteRune(r)
		if b.Len() >= 200 {
			break
		}
	}
	cleaned := strings.TrimSpace(b.String())
	if cleaned == "" {
		return "MCP client"
	}
	return cleaned
}

func displayClientName(name string) string {
	if strings.TrimSpace(name) == "" {
		return "MCP client"
	}
	return name
}

func newOAuthClientID() (string, error) {
	id, err := randomHex(16)
	if err != nil {
		return "", err
	}
	return "gmc_" + id, nil
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func readOAuthForm(w http.ResponseWriter, r *http.Request) (url.Values, error) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	ct := r.Header.Get("Content-Type")
	if strings.Contains(ct, "application/json") {
		var raw map[string]any
		if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
			return nil, err
		}
		vals := url.Values{}
		for key, value := range raw {
			if s, ok := value.(string); ok {
				vals.Set(key, s)
			}
		}
		return vals, nil
	}
	if err := r.ParseForm(); err != nil {
		return nil, err
	}
	return r.PostForm, nil
}

func setOAuthCORS(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Accept")
}

func writeOAuthJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeOAuthError(w http.ResponseWriter, status int, code, desc string) {
	setOAuthCORS(w)
	writeOAuthJSON(w, status, map[string]string{
		"error":             code,
		"error_description": desc,
	})
}
