package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// OAuthClient is a dynamically registered public OAuth client.
type OAuthClient struct {
	ID           string
	Name         string
	RedirectURIs []string
	CreatedAt    time.Time
}

// OAuthPending is an authorization request waiting for the user to grant or deny it.
type OAuthPending struct {
	ID            string
	ClientID      string
	RedirectURI   string
	CodeChallenge string
	Resource      string
	Scope         string
	State         string
	ExpiresAt     time.Time
	CreatedAt     time.Time
}

// OAuthAuthCode is a one-time authorization code. Only the hash is stored.
type OAuthAuthCode struct {
	CodeHash      string
	ClientID      string
	UserID        string
	RedirectURI   string
	CodeChallenge string
	Resource      string
	Scope         string
	ExpiresAt     time.Time
	Used          bool
}

// OAuthRefreshToken is a rotating refresh token. Only the hash is stored.
type OAuthRefreshToken struct {
	TokenHash string
	ClientID  string
	UserID    string
	Scope     string
	Resource  string
	ExpiresAt time.Time
	Revoked   bool
}

// CreateOAuthClient inserts a dynamically registered client.
func (d *DB) CreateOAuthClient(ctx context.Context, c *OAuthClient) error {
	raw, err := json.Marshal(c.RedirectURIs)
	if err != nil {
		return fmt.Errorf("encoding redirect uris: %w", err)
	}
	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now().UTC()
	}
	_, err = d.exec(ctx,
		`INSERT INTO oauth_clients (id, name, redirect_uris, created_at) VALUES (?, ?, ?, ?)`,
		c.ID, c.Name, string(raw), c.CreatedAt)
	if err != nil {
		return fmt.Errorf("inserting oauth client: %w", err)
	}
	return nil
}

// GetOAuthClient loads a client by id.
func (d *DB) GetOAuthClient(ctx context.Context, id string) (*OAuthClient, error) {
	row := d.queryRow(ctx,
		`SELECT id, name, redirect_uris, created_at FROM oauth_clients WHERE id = ?`, id)
	c := &OAuthClient{}
	var raw string
	if err := row.Scan(&c.ID, &c.Name, &raw, &c.CreatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, sql.ErrNoRows
		}
		return nil, fmt.Errorf("querying oauth client: %w", err)
	}
	if err := json.Unmarshal([]byte(raw), &c.RedirectURIs); err != nil {
		return nil, fmt.Errorf("decoding redirect uris: %w", err)
	}
	return c, nil
}

// CreateOAuthPending stores a consent request.
func (d *DB) CreateOAuthPending(ctx context.Context, p *OAuthPending) error {
	if p.CreatedAt.IsZero() {
		p.CreatedAt = time.Now().UTC()
	}
	_, err := d.exec(ctx,
		`INSERT INTO oauth_pending (id, client_id, redirect_uri, code_challenge, resource, scope, state, expires_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.ClientID, p.RedirectURI, p.CodeChallenge, p.Resource, p.Scope, p.State, p.ExpiresAt, p.CreatedAt)
	if err != nil {
		return fmt.Errorf("inserting oauth pending request: %w", err)
	}
	return nil
}

// GetOAuthPending loads a consent request.
func (d *DB) GetOAuthPending(ctx context.Context, id string) (*OAuthPending, error) {
	row := d.queryRow(ctx,
		`SELECT id, client_id, redirect_uri, code_challenge, resource, scope, state, expires_at, created_at
		 FROM oauth_pending WHERE id = ?`, id)
	p := &OAuthPending{}
	if err := row.Scan(&p.ID, &p.ClientID, &p.RedirectURI, &p.CodeChallenge, &p.Resource, &p.Scope, &p.State, &p.ExpiresAt, &p.CreatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, sql.ErrNoRows
		}
		return nil, fmt.Errorf("querying oauth pending request: %w", err)
	}
	return p, nil
}

// DeleteOAuthPending removes a consent request.
func (d *DB) DeleteOAuthPending(ctx context.Context, id string) error {
	_, err := d.exec(ctx, `DELETE FROM oauth_pending WHERE id = ?`, id)
	return err
}

// CreateOAuthAuthCode stores the hash of a one-time authorization code.
func (d *DB) CreateOAuthAuthCode(ctx context.Context, c *OAuthAuthCode) error {
	_, err := d.exec(ctx,
		`INSERT INTO oauth_auth_codes (code_hash, client_id, user_id, redirect_uri, code_challenge, resource, scope, expires_at, used)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.CodeHash, c.ClientID, c.UserID, c.RedirectURI, c.CodeChallenge, c.Resource, c.Scope, c.ExpiresAt, boolToInt(c.Used))
	if err != nil {
		return fmt.Errorf("inserting oauth auth code: %w", err)
	}
	return nil
}

// GetOAuthAuthCode loads an authorization code by its hash.
func (d *DB) GetOAuthAuthCode(ctx context.Context, codeHash string) (*OAuthAuthCode, error) {
	row := d.queryRow(ctx,
		`SELECT code_hash, client_id, user_id, redirect_uri, code_challenge, resource, scope, expires_at, used
		 FROM oauth_auth_codes WHERE code_hash = ?`, codeHash)
	c := &OAuthAuthCode{}
	var used int
	if err := row.Scan(&c.CodeHash, &c.ClientID, &c.UserID, &c.RedirectURI, &c.CodeChallenge, &c.Resource, &c.Scope, &c.ExpiresAt, &used); err != nil {
		if err == sql.ErrNoRows {
			return nil, sql.ErrNoRows
		}
		return nil, fmt.Errorf("querying oauth auth code: %w", err)
	}
	c.Used = used != 0
	return c, nil
}

// ConsumeOAuthAuthCode marks a code used. It returns an error if the code was already used.
func (d *DB) ConsumeOAuthAuthCode(ctx context.Context, codeHash string) error {
	result, err := d.exec(ctx,
		`UPDATE oauth_auth_codes SET used = 1 WHERE code_hash = ? AND used = 0`, codeHash)
	if err != nil {
		return fmt.Errorf("consuming oauth auth code: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking oauth auth code rows: %w", err)
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// CreateOAuthRefreshToken stores the hash of a refresh token.
func (d *DB) CreateOAuthRefreshToken(ctx context.Context, t *OAuthRefreshToken) error {
	_, err := d.exec(ctx,
		`INSERT INTO oauth_refresh_tokens (token_hash, client_id, user_id, scope, resource, expires_at, revoked)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		t.TokenHash, t.ClientID, t.UserID, t.Scope, t.Resource, t.ExpiresAt, boolToInt(t.Revoked))
	if err != nil {
		return fmt.Errorf("inserting oauth refresh token: %w", err)
	}
	return nil
}

// GetOAuthRefreshToken loads a refresh token by its hash.
func (d *DB) GetOAuthRefreshToken(ctx context.Context, tokenHash string) (*OAuthRefreshToken, error) {
	row := d.queryRow(ctx,
		`SELECT token_hash, client_id, user_id, scope, resource, expires_at, revoked
		 FROM oauth_refresh_tokens WHERE token_hash = ?`, tokenHash)
	t := &OAuthRefreshToken{}
	var revoked int
	if err := row.Scan(&t.TokenHash, &t.ClientID, &t.UserID, &t.Scope, &t.Resource, &t.ExpiresAt, &revoked); err != nil {
		if err == sql.ErrNoRows {
			return nil, sql.ErrNoRows
		}
		return nil, fmt.Errorf("querying oauth refresh token: %w", err)
	}
	t.Revoked = revoked != 0
	return t, nil
}

// RotateOAuthRefreshToken revokes oldHash and inserts next in one transaction.
func (d *DB) RotateOAuthRefreshToken(ctx context.Context, oldHash string, next *OAuthRefreshToken) error {
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning refresh rotation: %w", err)
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(ctx, d.rebind(
		`UPDATE oauth_refresh_tokens SET revoked = 1 WHERE token_hash = ? AND revoked = 0`), oldHash)
	if err != nil {
		return fmt.Errorf("revoking oauth refresh token: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking revoked refresh token: %w", err)
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	_, err = tx.ExecContext(ctx, d.rebind(
		`INSERT INTO oauth_refresh_tokens (token_hash, client_id, user_id, scope, resource, expires_at, revoked)
		 VALUES (?, ?, ?, ?, ?, ?, 0)`),
		next.TokenHash, next.ClientID, next.UserID, next.Scope, next.Resource, next.ExpiresAt)
	if err != nil {
		return fmt.Errorf("inserting rotated refresh token: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing refresh rotation: %w", err)
	}
	return nil
}
