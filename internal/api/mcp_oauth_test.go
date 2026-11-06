package api

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/go2engle/gantry/internal/auth"
	"github.com/go2engle/gantry/internal/db"
)

func TestMCPUnauthorizedChallengeAndProtectedResourceMetadata(t *testing.T) {
	env := newTestServerEnv(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	rec := httptest.NewRecorder()
	env.server.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	challenge := rec.Header().Get("WWW-Authenticate")
	if !strings.Contains(challenge, `realm="gantry"`) || !strings.Contains(challenge, "resource_metadata=") {
		t.Fatalf("WWW-Authenticate = %q", challenge)
	}
	if !strings.Contains(challenge, "http://localhost:8080/.well-known/oauth-protected-resource/api/v1/mcp") {
		t.Fatalf("metadata URL missing from %q", challenge)
	}

	metaReq := httptest.NewRequest(http.MethodGet, "/.well-known/oauth-protected-resource/api/v1/mcp", nil)
	metaRec := httptest.NewRecorder()
	env.server.Router().ServeHTTP(metaRec, metaReq)
	if metaRec.Code != http.StatusOK {
		t.Fatalf("metadata status = %d body=%s", metaRec.Code, metaRec.Body.String())
	}
	var meta struct {
		Resource             string   `json:"resource"`
		AuthorizationServers []string `json:"authorization_servers"`
		ScopesSupported      []string `json:"scopes_supported"`
	}
	if err := json.Unmarshal(metaRec.Body.Bytes(), &meta); err != nil {
		t.Fatal(err)
	}
	if meta.Resource != "http://localhost:8080/api/v1/mcp" {
		t.Fatalf("resource = %q", meta.Resource)
	}
	if len(meta.AuthorizationServers) != 1 || meta.AuthorizationServers[0] != "http://localhost:8080" {
		t.Fatalf("authorization_servers = %#v", meta.AuthorizationServers)
	}
	if len(meta.ScopesSupported) != 1 || meta.ScopesSupported[0] != "mcp" {
		t.Fatalf("scopes = %#v", meta.ScopesSupported)
	}

	asReq := httptest.NewRequest(http.MethodGet, "/.well-known/oauth-authorization-server", nil)
	asRec := httptest.NewRecorder()
	env.server.Router().ServeHTTP(asRec, asReq)
	if asRec.Code != http.StatusOK {
		t.Fatalf("as metadata status = %d", asRec.Code)
	}
	var asMeta struct {
		Issuer                        string   `json:"issuer"`
		AuthorizationEndpoint         string   `json:"authorization_endpoint"`
		TokenEndpoint                 string   `json:"token_endpoint"`
		RegistrationEndpoint          string   `json:"registration_endpoint"`
		CodeChallengeMethodsSupported []string `json:"code_challenge_methods_supported"`
	}
	if err := json.Unmarshal(asRec.Body.Bytes(), &asMeta); err != nil {
		t.Fatal(err)
	}
	if asMeta.Issuer != "http://localhost:8080" || asMeta.TokenEndpoint != "http://localhost:8080/oauth/token" {
		t.Fatalf("as metadata = %+v", asMeta)
	}
	if len(asMeta.CodeChallengeMethodsSupported) != 1 || asMeta.CodeChallengeMethodsSupported[0] != "S256" {
		t.Fatalf("pkce methods = %#v", asMeta.CodeChallengeMethodsSupported)
	}
}

func TestOAuthRegisterRejectsPublicHTTPRedirect(t *testing.T) {
	env := newTestServerEnv(t)
	rec := postJSON(env, "/oauth/register", `{"redirect_uris":["http://evil.com/callback"],"client_name":"bad"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "invalid_redirect_uri") {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestOAuthPKCEGrantAndReuse(t *testing.T) {
	env := newTestServerEnv(t)
	_, session := env.createUser(t, "dev-user", "developer")
	clientID := registerOAuthClient(t, env, "http://localhost:8787/callback")

	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])

	code := authorizeAndGrant(t, env, session, clientID, "http://localhost:8787/callback", challenge, "state-1")
	token := exchangeCode(t, env, clientID, "http://localhost:8787/callback", code, verifier)
	if token.AccessToken == "" || token.RefreshToken == "" || token.ExpiresIn != 3600 {
		t.Fatalf("token = %+v", token)
	}

	me := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	me.Header.Set("Authorization", "Bearer "+token.AccessToken)
	meRec := httptest.NewRecorder()
	env.server.Router().ServeHTTP(meRec, me)
	if meRec.Code != http.StatusUnauthorized {
		t.Fatalf("mcp token authenticated REST: %d %s", meRec.Code, meRec.Body.String())
	}

	mcp := mcpInitialize(t, env, token.AccessToken)
	if mcp.Code != http.StatusOK {
		t.Fatalf("mcp initialize = %d %s", mcp.Code, mcp.Body.String())
	}

	reused := exchangeCodeRaw(env, clientID, "http://localhost:8787/callback", code, verifier)
	if reused.Code != http.StatusBadRequest || !strings.Contains(reused.Body.String(), "invalid_grant") {
		t.Fatalf("reuse = %d %s", reused.Code, reused.Body.String())
	}

	badCode := authorizeAndGrant(t, env, session, clientID, "http://localhost:8787/callback", challenge, "state-2")
	bad := exchangeCodeRaw(env, clientID, "http://localhost:8787/callback", badCode, verifier+"not-the-verifier-padding-pad")
	if bad.Code != http.StatusBadRequest || !strings.Contains(bad.Body.String(), "code_verifier") {
		t.Fatalf("bad verifier = %d %s", bad.Code, bad.Body.String())
	}

	refresh := httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader(url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {token.RefreshToken},
		"client_id":     {clientID},
	}.Encode()))
	refresh.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	refreshRec := httptest.NewRecorder()
	env.server.Router().ServeHTTP(refreshRec, refresh)
	if refreshRec.Code != http.StatusOK {
		t.Fatalf("refresh = %d %s", refreshRec.Code, refreshRec.Body.String())
	}
}

func TestAPIKeyStillCallsMCP(t *testing.T) {
	env := newTestServerEnv(t)
	user, _ := env.createUser(t, "key-user", "developer")
	rawKey, keyHash, prefix, err := auth.GenerateAPIKey()
	if err != nil {
		t.Fatal(err)
	}
	if err := env.db.CreateAPIKey(t.Context(), &db.APIKey{
		UserID: user.ID,
		Name:   "agent",
		Prefix: prefix,
		Role:   "developer",
	}, keyHash); err != nil {
		t.Fatal(err)
	}

	rec := mcpInitialize(t, env, rawKey)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	body, _ := io.ReadAll(rec.Body)
	if !strings.Contains(string(body), "gantry") {
		t.Fatalf("body = %s", body)
	}
}

func registerOAuthClient(t *testing.T, env *testServerEnv, redirectURI string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"client_name":   "Cursor",
		"redirect_uris": []string{redirectURI},
	})
	rec := postJSON(env, "/oauth/register", string(body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("register = %d %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		ClientID string `json:"client_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.ClientID == "" {
		t.Fatal("empty client id")
	}
	return resp.ClientID
}

func authorizeAndGrant(t *testing.T, env *testServerEnv, session, clientID, redirectURI, challenge, state string) string {
	t.Helper()
	q := url.Values{
		"response_type":         {"code"},
		"client_id":             {clientID},
		"redirect_uri":          {redirectURI},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
		"state":                 {state},
		"scope":                 {"mcp"},
		"resource":              {"http://localhost:8080/api/v1/mcp"},
	}
	req := httptest.NewRequest(http.MethodGet, "/oauth/authorize?"+q.Encode(), nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session})
	rec := httptest.NewRecorder()
	env.server.Router().ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("authorize = %d %s", rec.Code, rec.Body.String())
	}
	loc, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	rid := loc.Query().Get("rid")
	if rid == "" {
		t.Fatalf("location = %s", rec.Header().Get("Location"))
	}

	consentBody, _ := json.Marshal(map[string]string{"rid": rid, "decision": "grant"})
	consent := httptest.NewRequest(http.MethodPost, "/api/v1/oauth/consent", strings.NewReader(string(consentBody)))
	consent.Header.Set("Content-Type", "application/json")
	consent.Header.Set("Authorization", "Bearer "+session)
	consentRec := httptest.NewRecorder()
	env.server.Router().ServeHTTP(consentRec, consent)
	if consentRec.Code != http.StatusOK {
		t.Fatalf("consent = %d %s", consentRec.Code, consentRec.Body.String())
	}
	var granted struct {
		RedirectURL string `json:"redirectUrl"`
	}
	if err := json.Unmarshal(consentRec.Body.Bytes(), &granted); err != nil {
		t.Fatal(err)
	}
	target, err := url.Parse(granted.RedirectURL)
	if err != nil {
		t.Fatal(err)
	}
	if target.Query().Get("state") != state {
		t.Fatalf("state = %q", target.Query().Get("state"))
	}
	code := target.Query().Get("code")
	if code == "" {
		t.Fatalf("redirect = %s", granted.RedirectURL)
	}
	return code
}

type oauthTokenBody struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
}

func exchangeCode(t *testing.T, env *testServerEnv, clientID, redirectURI, code, verifier string) oauthTokenBody {
	t.Helper()
	rec := exchangeCodeRaw(env, clientID, redirectURI, code, verifier)
	if rec.Code != http.StatusOK {
		t.Fatalf("token = %d %s", rec.Code, rec.Body.String())
	}
	var body oauthTokenBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body
}

func exchangeCodeRaw(env *testServerEnv, clientID, redirectURI, code, verifier string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader(url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"client_id":     {clientID},
		"redirect_uri":  {redirectURI},
		"code_verifier": {verifier},
	}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	env.server.Router().ServeHTTP(rec, req)
	return rec
}

func mcpInitialize(t *testing.T, env *testServerEnv, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	env.server.Router().ServeHTTP(rec, req)
	return rec
}

func postJSON(env *testServerEnv, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	env.server.Router().ServeHTTP(rec, req)
	return rec
}
