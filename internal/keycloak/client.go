// Package keycloak provides a client for interacting with the Keycloak Admin REST API.
// This is a custom implementation that works with raw JSON to support all Keycloak versions
// without being limited by struct definitions.
package keycloak

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-resty/resty/v2"
)

// Client provides methods to interact with the Keycloak Admin REST API
type Client struct {
	baseURL      string
	realm        string
	username     string
	password     string
	clientID     string
	clientSecret string

	caCert             string
	insecureSkipVerify bool

	httpClient  *resty.Client
	token       *TokenResponse
	tokenExpiry time.Time
	tokenMutex  sync.RWMutex
	log         logr.Logger
}

// Config holds Keycloak client configuration
type Config struct {
	BaseURL      string
	Realm        string // defaults to "master"
	Username     string
	Password     string
	ClientID     string // optional, for client credentials
	ClientSecret string // optional, for client credentials

	// CACert is a PEM-encoded CA bundle used to verify the Keycloak server
	// certificate. Ignored when InsecureSkipVerify is true.
	CACert string
	// InsecureSkipVerify disables TLS verification. Do not use in production.
	InsecureSkipVerify bool
}

// TokenResponse represents an OAuth2 token response
type TokenResponse struct {
	AccessToken      string `json:"access_token"`
	ExpiresIn        int    `json:"expires_in"`
	RefreshToken     string `json:"refresh_token"`
	RefreshExpiresIn int    `json:"refresh_expires_in"`
	TokenType        string `json:"token_type"`
}

// NewClient creates a new Keycloak client
func NewClient(cfg Config, log logr.Logger) *Client {
	cfg = normalizeConfig(cfg)

	httpClient := resty.New().
		SetTimeout(30 * time.Second).
		SetRetryCount(0) // We handle retries ourselves

	if tlsCfg, ok := buildTLSConfig(cfg, log); ok {
		httpClient.SetTLSClientConfig(tlsCfg)
	}

	return &Client{
		baseURL:            cfg.BaseURL,
		realm:              cfg.Realm,
		username:           cfg.Username,
		password:           cfg.Password,
		clientID:           cfg.ClientID,
		clientSecret:       cfg.ClientSecret,
		caCert:             cfg.CACert,
		insecureSkipVerify: cfg.InsecureSkipVerify,
		httpClient:         httpClient,
		log:                log.WithName("keycloak-client"),
	}
}

func normalizeConfig(cfg Config) Config {
	cfg.BaseURL = strings.TrimSuffix(cfg.BaseURL, "/")
	if cfg.Realm == "" {
		cfg.Realm = "master"
	}
	return cfg
}

// buildTLSConfig returns a *tls.Config when the cfg requests TLS customisation,
// or (nil, false) when the resty defaults are appropriate. An unparseable
// CACert is logged and dropped rather than failing client construction; the
// next HTTPS call will surface the verification error.
func buildTLSConfig(cfg Config, log logr.Logger) (*tls.Config, bool) {
	if cfg.InsecureSkipVerify {
		return &tls.Config{InsecureSkipVerify: true}, true //nolint:gosec // opt-in via spec.tls.insecureSkipVerify
	}
	if cfg.CACert == "" {
		return nil, false
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM([]byte(cfg.CACert)) {
		log.WithName("keycloak-client").Info("failed to parse CACert as PEM; falling back to system roots")
		return nil, false
	}
	return &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}, true
}

// getToken gets a valid token, refreshing if necessary
func (c *Client) getToken(ctx context.Context) (string, error) {
	c.tokenMutex.RLock()
	if c.token != nil && c.isTokenValid() {
		defer c.tokenMutex.RUnlock()
		return c.token.AccessToken, nil
	}
	c.tokenMutex.RUnlock()

	c.tokenMutex.Lock()
	defer c.tokenMutex.Unlock()

	// Double-check after acquiring write lock
	if c.token != nil && c.isTokenValid() {
		return c.token.AccessToken, nil
	}

	// Prepare token request
	tokenURL := fmt.Sprintf("%s/realms/%s/protocol/openid-connect/token", c.baseURL, c.realm)

	formData := map[string]string{}

	if c.clientID != "" && c.clientSecret != "" {
		// Client credentials grant
		formData["grant_type"] = "client_credentials"
		formData["client_id"] = c.clientID
		formData["client_secret"] = c.clientSecret
	} else {
		// Password grant
		formData["grant_type"] = "password"
		formData["client_id"] = "admin-cli"
		formData["username"] = c.username
		formData["password"] = c.password
	}

	var token TokenResponse
	resp, err := c.httpClient.R().
		SetContext(ctx).
		SetFormData(formData).
		SetResult(&token).
		Post(tokenURL)

	if err != nil {
		return "", fmt.Errorf("failed to authenticate with Keycloak: %w", err)
	}

	if resp.IsError() {
		return "", fmt.Errorf("failed to authenticate with Keycloak: %s: %s", resp.Status(), string(resp.Body()))
	}

	c.token = &token
	c.tokenExpiry = time.Now().Add(time.Duration(token.ExpiresIn) * time.Second)

	return token.AccessToken, nil
}

// isTokenValid checks if the current token is still valid
func (c *Client) isTokenValid() bool {
	if c.token == nil {
		return false
	}
	// Add a buffer of 30 seconds before expiration
	return time.Now().Add(30 * time.Second).Before(c.tokenExpiry)
}

// Ping checks if the Keycloak server is accessible
func (c *Client) Ping(ctx context.Context) error {
	_, err := c.getToken(ctx)
	return err
}

// request creates an authenticated request
func (c *Client) request(ctx context.Context) (*resty.Request, error) {
	token, err := c.getToken(ctx)
	if err != nil {
		return nil, err
	}

	return c.httpClient.R().
		SetContext(ctx).
		SetHeader("Content-Type", "application/json").
		SetAuthToken(token), nil
}

// ============================================================================
// Generic CRUD Operations
// ============================================================================

// Create creates a resource and returns its ID (from Location header)
func (c *Client) Create(ctx context.Context, path string, body interface{}) (string, error) {
	req, err := c.request(ctx)
	if err != nil {
		return "", err
	}

	resp, err := req.SetBody(body).Post(c.baseURL + path)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}

	if resp.IsError() {
		return "", fmt.Errorf("%s: %s", resp.Status(), string(resp.Body()))
	}

	// Extract ID from Location header
	location := resp.Header().Get("Location")
	if location != "" {
		parts := strings.Split(location, "/")
		if len(parts) > 0 {
			return parts[len(parts)-1], nil
		}
	}

	return "", nil
}

// Get retrieves a resource
func (c *Client) Get(ctx context.Context, path string, result interface{}) error {
	req, err := c.request(ctx)
	if err != nil {
		return err
	}

	resp, err := req.SetResult(result).Get(c.baseURL + path)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}

	if resp.IsError() {
		return fmt.Errorf("%s: %s", resp.Status(), string(resp.Body()))
	}

	return nil
}

// Update updates a resource
func (c *Client) Update(ctx context.Context, path string, body interface{}) error {
	req, err := c.request(ctx)
	if err != nil {
		return err
	}

	resp, err := req.SetBody(body).Put(c.baseURL + path)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}

	if resp.IsError() {
		return fmt.Errorf("%s: %s", resp.Status(), string(resp.Body()))
	}

	return nil
}

// put is like Update but decodes the response body into result. Use when the
// PUT endpoint returns a useful body (e.g. management/permissions, which
// returns a ManagementPermissionReference on toggle).
func (c *Client) put(ctx context.Context, path string, body interface{}, result interface{}) error {
	req, err := c.request(ctx)
	if err != nil {
		return err
	}

	resp, err := req.SetBody(body).SetResult(result).Put(c.baseURL + path)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}

	if resp.IsError() {
		return fmt.Errorf("%s: %s", resp.Status(), string(resp.Body()))
	}

	return nil
}

// Delete deletes a resource
func (c *Client) Delete(ctx context.Context, path string) error {
	req, err := c.request(ctx)
	if err != nil {
		return err
	}

	resp, err := req.Delete(c.baseURL + path)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}

	if resp.IsError() {
		return fmt.Errorf("%s: %s", resp.Status(), string(resp.Body()))
	}

	return nil
}

// List retrieves a list of resources with optional query parameters
func (c *Client) List(ctx context.Context, path string, params map[string]string, result interface{}) error {
	req, err := c.request(ctx)
	if err != nil {
		return err
	}

	if params != nil {
		req.SetQueryParams(params)
	}

	resp, err := req.SetResult(result).Get(c.baseURL + path)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}

	if resp.IsError() {
		return fmt.Errorf("%s: %s", resp.Status(), string(resp.Body()))
	}

	return nil
}

// listAll retrieves all resources using offset-based pagination.
// The Keycloak Admin API defaults to returning at most 100 results.
// This helper pages through all results using the "first" and "max" query
// parameters and returns the full list.
func listAll[T any](ctx context.Context, c *Client, path string, params map[string]string) ([]T, error) {
	const pageSize = 100
	if params == nil {
		params = make(map[string]string)
	}

	var all []T
	for offset := 0; ; offset += pageSize {
		params["first"] = fmt.Sprintf("%d", offset)
		params["max"] = fmt.Sprintf("%d", pageSize)

		var page []T
		if err := c.List(ctx, path, params, &page); err != nil {
			return nil, err
		}
		all = append(all, page...)
		if len(page) < pageSize {
			break
		}
	}
	return all, nil
}

// Post performs a POST request (for non-CRUD operations)
func (c *Client) Post(ctx context.Context, path string, body interface{}, result interface{}) error {
	req, err := c.request(ctx)
	if err != nil {
		return err
	}

	if body != nil {
		req.SetBody(body)
	}
	if result != nil {
		req.SetResult(result)
	}

	resp, err := req.Post(c.baseURL + path)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}

	if resp.IsError() {
		return fmt.Errorf("%s: %s", resp.Status(), string(resp.Body()))
	}

	return nil
}

// ============================================================================
// Server Info
// ============================================================================

// ServerInfo represents Keycloak server information
type ServerInfo struct {
	SystemInfo struct {
		Version string `json:"version"`
	} `json:"systemInfo"`
}

// GetServerInfo returns Keycloak server information
func (c *Client) GetServerInfo(ctx context.Context) (*ServerInfo, error) {
	var info ServerInfo
	if err := c.Get(ctx, "/admin/serverinfo", &info); err != nil {
		return nil, err
	}
	return &info, nil
}

// ============================================================================
// Realm Operations
// ============================================================================

// RealmRepresentation represents a Keycloak realm (minimal fields we need)
type RealmRepresentation struct {
	ID                   *string `json:"id,omitempty"`
	Realm                *string `json:"realm,omitempty"`
	Enabled              *bool   `json:"enabled,omitempty"`
	DisplayName          *string `json:"displayName,omitempty"`
	OrganizationsEnabled *bool   `json:"organizationsEnabled,omitempty"`
}

// CreateRealm creates a new realm
func (c *Client) CreateRealm(ctx context.Context, realm json.RawMessage) error {
	cfg := DefaultRetryConfig()
	return WithRetryVoid(ctx, cfg, "CreateRealm", func() error {
		return c.Update(ctx, "/admin/realms", realm) // POST to /admin/realms uses PUT-like semantics
	})
}

// CreateRealmFromDefinition creates a realm from raw JSON definition
func (c *Client) CreateRealmFromDefinition(ctx context.Context, definition json.RawMessage) error {
	cfg := DefaultRetryConfig()
	return WithRetryVoid(ctx, cfg, "CreateRealm", func() error {
		req, err := c.request(ctx)
		if err != nil {
			return err
		}
		resp, err := req.SetBody(definition).Post(c.baseURL + "/admin/realms")
		if err != nil {
			return fmt.Errorf("request failed: %w", err)
		}
		if resp.IsError() {
			return fmt.Errorf("%s: %s", resp.Status(), string(resp.Body()))
		}
		return nil
	})
}

// GetRealm gets a realm by name
func (c *Client) GetRealm(ctx context.Context, realmName string) (*RealmRepresentation, error) {
	var realm RealmRepresentation
	if err := c.Get(ctx, "/admin/realms/"+url.PathEscape(realmName), &realm); err != nil {
		return nil, err
	}
	return &realm, nil
}

// UpdateRealm updates a realm from raw JSON definition
func (c *Client) UpdateRealm(ctx context.Context, realmName string, definition json.RawMessage) error {
	cfg := DefaultRetryConfig()
	return WithRetryVoid(ctx, cfg, "UpdateRealm", func() error {
		return c.Update(ctx, "/admin/realms/"+url.PathEscape(realmName), definition)
	})
}

// DeleteRealm deletes a realm
func (c *Client) DeleteRealm(ctx context.Context, realmName string) error {
	return c.Delete(ctx, "/admin/realms/"+url.PathEscape(realmName))
}

// ============================================================================
// Client Operations
// ============================================================================

// ClientRepresentation represents a Keycloak client (minimal fields we need)
type ClientRepresentation struct {
	ID                     *string `json:"id,omitempty"`
	ClientID               *string `json:"clientId,omitempty"`
	Name                   *string `json:"name,omitempty"`
	Enabled                *bool   `json:"enabled,omitempty"`
	Secret                 *string `json:"secret,omitempty"`
	ServiceAccountsEnabled *bool   `json:"serviceAccountsEnabled,omitempty"`
}

// CreateClient creates a new client
func (c *Client) CreateClient(ctx context.Context, realmName string, clientDef json.RawMessage) (string, error) {
	cfg := DefaultRetryConfig()
	return WithRetry(ctx, cfg, "CreateClient", func() (string, error) {
		return c.Create(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/clients", clientDef)
	})
}

// GetClient gets a client by internal ID
func (c *Client) GetClient(ctx context.Context, realmName, clientID string) (*ClientRepresentation, error) {
	var client ClientRepresentation
	if err := c.Get(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/clients/"+url.PathEscape(clientID), &client); err != nil {
		return nil, err
	}
	return &client, nil
}

// GetClients gets all clients in a realm with optional filtering
func (c *Client) GetClients(ctx context.Context, realmName string, params map[string]string) ([]ClientRepresentation, error) {
	return listAll[ClientRepresentation](ctx, c, "/admin/realms/"+url.PathEscape(realmName)+"/clients", params)
}

// GetClientByClientID finds a client by its clientId field
func (c *Client) GetClientByClientID(ctx context.Context, realmName, clientID string) (*ClientRepresentation, error) {
	clients, err := c.GetClients(ctx, realmName, map[string]string{"clientId": clientID})
	if err != nil {
		return nil, err
	}
	if len(clients) == 0 {
		return nil, fmt.Errorf("client not found: %s", clientID)
	}
	return &clients[0], nil
}

// UpdateClient updates a client
func (c *Client) UpdateClient(ctx context.Context, realmName, clientID string, clientDef json.RawMessage) error {
	cfg := DefaultRetryConfig()
	return WithRetryVoid(ctx, cfg, "UpdateClient", func() error {
		return c.Update(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/clients/"+url.PathEscape(clientID), clientDef)
	})
}

// DeleteClient deletes a client
func (c *Client) DeleteClient(ctx context.Context, realmName, clientID string) error {
	return c.Delete(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/clients/"+url.PathEscape(clientID))
}

// GetClientSecret gets the client secret
func (c *Client) GetClientSecret(ctx context.Context, realmName, clientID string) (string, error) {
	var result struct {
		Value string `json:"value"`
	}
	if err := c.Get(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/clients/"+url.PathEscape(clientID)+"/client-secret", &result); err != nil {
		return "", err
	}
	return result.Value, nil
}

// RegenerateClientSecret regenerates the client secret
func (c *Client) RegenerateClientSecret(ctx context.Context, realmName, clientID string) (string, error) {
	var result struct {
		Value string `json:"value"`
	}
	if err := c.Post(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/clients/"+url.PathEscape(clientID)+"/client-secret", nil, &result); err != nil {
		return "", err
	}
	return result.Value, nil
}

// GetClientServiceAccount gets the service account user for a client
func (c *Client) GetClientServiceAccount(ctx context.Context, realmName, clientID string) (*UserRepresentation, error) {
	var user UserRepresentation
	if err := c.Get(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/clients/"+url.PathEscape(clientID)+"/service-account-user", &user); err != nil {
		return nil, err
	}
	return &user, nil
}

// ============================================================================
// User Operations
// ============================================================================

// UserRepresentation represents a Keycloak user (minimal fields we need)
type UserRepresentation struct {
	ID            *string `json:"id,omitempty"`
	Username      *string `json:"username,omitempty"`
	Email         *string `json:"email,omitempty"`
	Enabled       *bool   `json:"enabled,omitempty"`
	FirstName     *string `json:"firstName,omitempty"`
	LastName      *string `json:"lastName,omitempty"`
	EmailVerified *bool   `json:"emailVerified,omitempty"`
}

// CreateUser creates a new user
func (c *Client) CreateUser(ctx context.Context, realmName string, userDef json.RawMessage) (string, error) {
	cfg := DefaultRetryConfig()
	return WithRetry(ctx, cfg, "CreateUser", func() (string, error) {
		return c.Create(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/users", userDef)
	})
}

// GetUser gets a user by ID
func (c *Client) GetUser(ctx context.Context, realmName, userID string) (*UserRepresentation, error) {
	var user UserRepresentation
	if err := c.Get(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/users/"+url.PathEscape(userID), &user); err != nil {
		return nil, err
	}
	return &user, nil
}

// GetUsers gets users with optional filtering
func (c *Client) GetUsers(ctx context.Context, realmName string, params map[string]string) ([]UserRepresentation, error) {
	return listAll[UserRepresentation](ctx, c, "/admin/realms/"+url.PathEscape(realmName)+"/users", params)
}

// GetUserByUsername finds a user by username
func (c *Client) GetUserByUsername(ctx context.Context, realmName, username string) (*UserRepresentation, error) {
	users, err := c.GetUsers(ctx, realmName, map[string]string{"username": username, "exact": "true"})
	if err != nil {
		return nil, err
	}
	if len(users) == 0 {
		return nil, fmt.Errorf("user not found: %s", username)
	}
	return &users[0], nil
}

// UpdateUser updates a user
func (c *Client) UpdateUser(ctx context.Context, realmName, userID string, userDef json.RawMessage) error {
	cfg := DefaultRetryConfig()
	return WithRetryVoid(ctx, cfg, "UpdateUser", func() error {
		return c.Update(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/users/"+url.PathEscape(userID), userDef)
	})
}

// DeleteUser deletes a user
func (c *Client) DeleteUser(ctx context.Context, realmName, userID string) error {
	return c.Delete(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/users/"+url.PathEscape(userID))
}

// SetPassword sets a user's password
func (c *Client) SetPassword(ctx context.Context, realmName, userID, password string, temporary bool) error {
	cfg := DefaultRetryConfig()
	return WithRetryVoid(ctx, cfg, "SetPassword", func() error {
		body := map[string]interface{}{
			"type":      "password",
			"value":     password,
			"temporary": temporary,
		}
		return c.Update(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/users/"+url.PathEscape(userID)+"/reset-password", body)
	})
}

// ============================================================================
// Group Operations
// ============================================================================

// GroupRepresentation represents a Keycloak group (minimal fields we need)
type GroupRepresentation struct {
	ID            *string               `json:"id,omitempty"`
	Name          *string               `json:"name,omitempty"`
	Path          *string               `json:"path,omitempty"`
	SubGroupCount *int                  `json:"subGroupCount,omitempty"`
	SubGroups     []GroupRepresentation `json:"subGroups,omitempty"`
}

// CreateGroup creates a new group
func (c *Client) CreateGroup(ctx context.Context, realmName string, groupDef json.RawMessage) (string, error) {
	return c.Create(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/groups", groupDef)
}

// CreateChildGroup creates a child group
func (c *Client) CreateChildGroup(ctx context.Context, realmName, parentID string, groupDef json.RawMessage) (string, error) {
	return c.Create(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/groups/"+url.PathEscape(parentID)+"/children", groupDef)
}

// GetGroup gets a group by ID
func (c *Client) GetGroup(ctx context.Context, realmName, groupID string) (*GroupRepresentation, error) {
	var group GroupRepresentation
	if err := c.Get(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/groups/"+url.PathEscape(groupID), &group); err != nil {
		return nil, err
	}
	return &group, nil
}

// GetGroups gets all groups in a realm
func (c *Client) GetGroups(ctx context.Context, realmName string, params map[string]string) ([]GroupRepresentation, error) {
	return listAll[GroupRepresentation](ctx, c, "/admin/realms/"+url.PathEscape(realmName)+"/groups", params)
}

// GetGroupChildren returns direct children of a group.
// Accepts pagination params (first, max, briefRepresentation, search, exact).
// Required since Keycloak 23+, where /groups no longer inlines nested subGroups.
func (c *Client) GetGroupChildren(ctx context.Context, realmName, groupID string, params map[string]string) ([]GroupRepresentation, error) {
	var groups []GroupRepresentation
	if err := c.List(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/groups/"+url.PathEscape(groupID)+"/children", params, &groups); err != nil {
		return nil, err
	}
	return groups, nil
}

// GetGroupByName finds a group by name
func (c *Client) GetGroupByName(ctx context.Context, realmName, name string) (*GroupRepresentation, error) {
	groups, err := c.GetGroups(ctx, realmName, map[string]string{"search": name, "exact": "true"})
	if err != nil {
		return nil, err
	}
	// Search through results for exact match
	for i := range groups {
		if groups[i].Name != nil && *groups[i].Name == name {
			return &groups[i], nil
		}
	}
	return nil, fmt.Errorf("group not found: %s", name)
}

// UpdateGroup updates a group
func (c *Client) UpdateGroup(ctx context.Context, realmName, groupID string, groupDef json.RawMessage) error {
	return c.Update(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/groups/"+url.PathEscape(groupID), groupDef)
}

// DeleteGroup deletes a group
func (c *Client) DeleteGroup(ctx context.Context, realmName, groupID string) error {
	return c.Delete(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/groups/"+url.PathEscape(groupID))
}

// ============================================================================
// Client Scope Operations
// ============================================================================

// ClientScopeRepresentation represents a Keycloak client scope (minimal fields we need)
type ClientScopeRepresentation struct {
	ID          *string `json:"id,omitempty"`
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
	Protocol    *string `json:"protocol,omitempty"`
}

// CreateClientScope creates a new client scope
func (c *Client) CreateClientScope(ctx context.Context, realmName string, scopeDef json.RawMessage) (string, error) {
	return c.Create(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/client-scopes", scopeDef)
}

// GetClientScope gets a client scope by ID
func (c *Client) GetClientScope(ctx context.Context, realmName, scopeID string) (*ClientScopeRepresentation, error) {
	var scope ClientScopeRepresentation
	if err := c.Get(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/client-scopes/"+url.PathEscape(scopeID), &scope); err != nil {
		return nil, err
	}
	return &scope, nil
}

// GetClientScopes gets all client scopes in a realm
func (c *Client) GetClientScopes(ctx context.Context, realmName string) ([]ClientScopeRepresentation, error) {
	var scopes []ClientScopeRepresentation
	if err := c.List(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/client-scopes", nil, &scopes); err != nil {
		return nil, err
	}
	return scopes, nil
}

// GetClientScopeByName finds a client scope by name
func (c *Client) GetClientScopeByName(ctx context.Context, realmName, name string) (*ClientScopeRepresentation, error) {
	scopes, err := c.GetClientScopes(ctx, realmName)
	if err != nil {
		return nil, err
	}
	for i := range scopes {
		if scopes[i].Name != nil && *scopes[i].Name == name {
			return &scopes[i], nil
		}
	}
	return nil, fmt.Errorf("client scope not found: %s", name)
}

// UpdateClientScope updates a client scope
func (c *Client) UpdateClientScope(ctx context.Context, realmName, scopeID string, scopeDef json.RawMessage) error {
	return c.Update(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/client-scopes/"+url.PathEscape(scopeID), scopeDef)
}

// DeleteClientScope deletes a client scope
func (c *Client) DeleteClientScope(ctx context.Context, realmName, scopeID string) error {
	return c.Delete(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/client-scopes/"+url.PathEscape(scopeID))
}

// ============================================================================
// Client Scope Assignment Operations (per-client)
// ============================================================================

// GetClientDefaultScopes gets the default client scopes assigned to a client
func (c *Client) GetClientDefaultScopes(ctx context.Context, realmName, clientUUID string) ([]ClientScopeRepresentation, error) {
	var scopes []ClientScopeRepresentation
	if err := c.List(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/clients/"+url.PathEscape(clientUUID)+"/default-client-scopes", nil, &scopes); err != nil {
		return nil, err
	}
	return scopes, nil
}

// AddClientDefaultScope adds a default client scope to a client
func (c *Client) AddClientDefaultScope(ctx context.Context, realmName, clientUUID, scopeID string) error {
	return c.Update(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/clients/"+url.PathEscape(clientUUID)+"/default-client-scopes/"+url.PathEscape(scopeID), nil)
}

// RemoveClientDefaultScope removes a default client scope from a client
func (c *Client) RemoveClientDefaultScope(ctx context.Context, realmName, clientUUID, scopeID string) error {
	return c.Delete(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/clients/"+url.PathEscape(clientUUID)+"/default-client-scopes/"+url.PathEscape(scopeID))
}

// GetClientOptionalScopes gets the optional client scopes assigned to a client
func (c *Client) GetClientOptionalScopes(ctx context.Context, realmName, clientUUID string) ([]ClientScopeRepresentation, error) {
	var scopes []ClientScopeRepresentation
	if err := c.List(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/clients/"+url.PathEscape(clientUUID)+"/optional-client-scopes", nil, &scopes); err != nil {
		return nil, err
	}
	return scopes, nil
}

// AddClientOptionalScope adds an optional client scope to a client
func (c *Client) AddClientOptionalScope(ctx context.Context, realmName, clientUUID, scopeID string) error {
	return c.Update(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/clients/"+url.PathEscape(clientUUID)+"/optional-client-scopes/"+url.PathEscape(scopeID), nil)
}

// RemoveClientOptionalScope removes an optional client scope from a client
func (c *Client) RemoveClientOptionalScope(ctx context.Context, realmName, clientUUID, scopeID string) error {
	return c.Delete(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/clients/"+url.PathEscape(clientUUID)+"/optional-client-scopes/"+url.PathEscape(scopeID))
}

// ============================================================================
// Identity Provider Operations
// ============================================================================

// IdentityProviderRepresentation represents a Keycloak identity provider (minimal fields we need)
type IdentityProviderRepresentation struct {
	Alias       *string `json:"alias,omitempty"`
	DisplayName *string `json:"displayName,omitempty"`
	ProviderId  *string `json:"providerId,omitempty"`
	Enabled     *bool   `json:"enabled,omitempty"`
}

// CreateIdentityProvider creates a new identity provider
func (c *Client) CreateIdentityProvider(ctx context.Context, realmName string, idpDef json.RawMessage) (string, error) {
	return c.Create(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/identity-provider/instances", idpDef)
}

// GetIdentityProvider gets an identity provider by alias
func (c *Client) GetIdentityProvider(ctx context.Context, realmName, alias string) (*IdentityProviderRepresentation, error) {
	var idp IdentityProviderRepresentation
	if err := c.Get(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/identity-provider/instances/"+url.PathEscape(alias), &idp); err != nil {
		return nil, err
	}
	return &idp, nil
}

// UpdateIdentityProvider updates an identity provider
func (c *Client) UpdateIdentityProvider(ctx context.Context, realmName, alias string, idpDef json.RawMessage) error {
	return c.Update(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/identity-provider/instances/"+url.PathEscape(alias), idpDef)
}

// DeleteIdentityProvider deletes an identity provider
func (c *Client) DeleteIdentityProvider(ctx context.Context, realmName, alias string) error {
	return c.Delete(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/identity-provider/instances/"+url.PathEscape(alias))
}

// ============================================================================
// Identity Provider — Fine-grained Authz / Token-Exchange Permission
// ============================================================================

// ManagementPermissionReference is what Keycloak returns when toggling
// fine-grained-authz on a resource (here: an IdP). When `Enabled==true`,
// `ScopePermissions` carries the per-scope permission IDs that Keycloak
// auto-created under the realm-management authz resource server — for IdPs
// the key we care about is "token-exchange".
type ManagementPermissionReference struct {
	Enabled          bool              `json:"enabled"`
	Resource         string            `json:"resource,omitempty"`
	ScopePermissions map[string]string `json:"scopePermissions,omitempty"`
}

// SetIdentityProviderPermissionsEnabled toggles fine-grained-authz on an IdP.
// Returns the resulting reference (carries the auto-created scope-permission
// IDs in ScopePermissions, e.g. ScopePermissions["token-exchange"]).
func (c *Client) SetIdentityProviderPermissionsEnabled(ctx context.Context, realmName, alias string, enabled bool) (*ManagementPermissionReference, error) {
	path := "/admin/realms/" + url.PathEscape(realmName) + "/identity-provider/instances/" + url.PathEscape(alias) + "/management/permissions"
	body := map[string]bool{"enabled": enabled}
	var ref ManagementPermissionReference
	cfg := DefaultRetryConfig()
	err := WithRetryVoid(ctx, cfg, "SetIdentityProviderPermissionsEnabled", func() error {
		return c.put(ctx, path, body, &ref)
	})
	if err != nil {
		return nil, err
	}
	return &ref, nil
}

// GetIdentityProviderPermissions reads the current state of fine-grained
// authz on an IdP (Enabled flag + scope-permission IDs).
func (c *Client) GetIdentityProviderPermissions(ctx context.Context, realmName, alias string) (*ManagementPermissionReference, error) {
	path := "/admin/realms/" + url.PathEscape(realmName) + "/identity-provider/instances/" + url.PathEscape(alias) + "/management/permissions"
	var ref ManagementPermissionReference
	if err := c.Get(ctx, path, &ref); err != nil {
		return nil, err
	}
	return &ref, nil
}

// ClientPolicyRepresentation is the body shape Keycloak expects for
// Client-type authz policies (carries the list of allowed-client UUIDs).
type ClientPolicyRepresentation struct {
	ID               string   `json:"id,omitempty"`
	Name             string   `json:"name,omitempty"`
	Description      string   `json:"description,omitempty"`
	Type             string   `json:"type,omitempty"`
	Logic            string   `json:"logic,omitempty"`            // POSITIVE | NEGATIVE
	DecisionStrategy string   `json:"decisionStrategy,omitempty"` // UNANIMOUS | AFFIRMATIVE | CONSENSUS
	Clients          []string `json:"clients,omitempty"`          // client UUIDs (NOT clientId text)
}

// SearchClientPolicyByName looks up a Client-type authz policy by name within
// a resource server (a Keycloak client with `authorizationServicesEnabled`,
// e.g. realm-management). Returns nil with no error when not found.
//
// Uses the search hit's ID to do a second, type-specific GET (`/policy/client/{id}`)
// because the generic `/policy?name=…` listing returns a flattened shape with
// `config.clients` as a JSON-encoded string, whereas the type-specific GET
// returns a proper PolicyRepresentation with top-level `clients` — which is the
// shape this lib's struct uses (and that POST/PUT also accept).
func (c *Client) SearchClientPolicyByName(ctx context.Context, realmName, resourceServerUUID, name string) (*ClientPolicyRepresentation, error) {
	listPath := "/admin/realms/" + url.PathEscape(realmName) + "/clients/" + url.PathEscape(resourceServerUUID) + "/authz/resource-server/policy"
	params := map[string]string{
		"name":  name,
		"exact": "true",
	}
	// Hits from the generic search expose only id+name+type+meta — enough to dispatch
	// the per-type GET below.
	var hits []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Type string `json:"type"`
	}
	if err := c.List(ctx, listPath, params, &hits); err != nil {
		return nil, err
	}
	for _, h := range hits {
		if h.Name != name {
			continue
		}
		clientPath := "/admin/realms/" + url.PathEscape(realmName) + "/clients/" + url.PathEscape(resourceServerUUID) + "/authz/resource-server/policy/client/" + url.PathEscape(h.ID)
		var full ClientPolicyRepresentation
		if err := c.Get(ctx, clientPath, &full); err != nil {
			return nil, err
		}
		return &full, nil
	}
	return nil, nil
}

// CreateClientPolicy creates a Client-type authz policy in a resource server.
// Returns the new policy's Keycloak ID.
//
// Keycloak's POST to `/authz/resource-server/policy/client` does not set a
// `Location` header on the 201 response (unlike most other admin endpoints) —
// the new ID lives in the response body. We therefore parse the body directly
// instead of relying on the generic `Client.Create` (which infers IDs from
// Location and would return an empty string here, breaking any downstream
// caller that needs the ID — e.g. binding the policy to a scope permission).
func (c *Client) CreateClientPolicy(ctx context.Context, realmName, resourceServerUUID string, policy ClientPolicyRepresentation) (string, error) {
	path := "/admin/realms/" + url.PathEscape(realmName) + "/clients/" + url.PathEscape(resourceServerUUID) + "/authz/resource-server/policy/client"
	cfg := DefaultRetryConfig()
	return WithRetry(ctx, cfg, "CreateClientPolicy", func() (string, error) {
		var created ClientPolicyRepresentation
		if err := c.Post(ctx, path, policy, &created); err != nil {
			return "", err
		}
		if created.ID == "" {
			return "", fmt.Errorf("Keycloak created the policy but returned an empty id in the body")
		}
		return created.ID, nil
	})
}

// UpdateClientPolicy updates an existing Client-type authz policy.
func (c *Client) UpdateClientPolicy(ctx context.Context, realmName, resourceServerUUID, policyID string, policy ClientPolicyRepresentation) error {
	path := "/admin/realms/" + url.PathEscape(realmName) + "/clients/" + url.PathEscape(resourceServerUUID) + "/authz/resource-server/policy/client/" + url.PathEscape(policyID)
	cfg := DefaultRetryConfig()
	return WithRetryVoid(ctx, cfg, "UpdateClientPolicy", func() error {
		body, err := json.Marshal(policy)
		if err != nil {
			return err
		}
		return c.Update(ctx, path, body)
	})
}

// DeletePolicy deletes any authz policy by ID (the type-agnostic endpoint).
func (c *Client) DeletePolicy(ctx context.Context, realmName, resourceServerUUID, policyID string) error {
	return c.Delete(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/clients/"+url.PathEscape(resourceServerUUID)+"/authz/resource-server/policy/"+url.PathEscape(policyID))
}

// ScopePermissionRepresentation is the body shape for a scope-based
// authorization permission (what Keycloak auto-creates as
// `token-exchange.permission.idp.<id>` when IdP permissions are enabled).
type ScopePermissionRepresentation struct {
	ID               string   `json:"id,omitempty"`
	Name             string   `json:"name,omitempty"`
	Description      string   `json:"description,omitempty"`
	Type             string   `json:"type,omitempty"` // typically "scope"
	Logic            string   `json:"logic,omitempty"`
	DecisionStrategy string   `json:"decisionStrategy,omitempty"`
	Resources        []string `json:"resources,omitempty"`
	Scopes           []string `json:"scopes,omitempty"`
	Policies         []string `json:"policies,omitempty"` // policy IDs bound to this permission
}

// GetScopePermission reads a scope permission's full body from a resource server.
func (c *Client) GetScopePermission(ctx context.Context, realmName, resourceServerUUID, permissionID string) (*ScopePermissionRepresentation, error) {
	path := "/admin/realms/" + url.PathEscape(realmName) + "/clients/" + url.PathEscape(resourceServerUUID) + "/authz/resource-server/permission/scope/" + url.PathEscape(permissionID)
	var perm ScopePermissionRepresentation
	if err := c.Get(ctx, path, &perm); err != nil {
		return nil, err
	}
	return &perm, nil
}

// UpdateScopePermission updates a scope permission. Use this to bind policies
// (the auto-created `token-exchange.permission.idp.<id>` ships without
// policies; the operator PUTs back the same body with Policies populated).
func (c *Client) UpdateScopePermission(ctx context.Context, realmName, resourceServerUUID, permissionID string, perm ScopePermissionRepresentation) error {
	path := "/admin/realms/" + url.PathEscape(realmName) + "/clients/" + url.PathEscape(resourceServerUUID) + "/authz/resource-server/permission/scope/" + url.PathEscape(permissionID)
	cfg := DefaultRetryConfig()
	return WithRetryVoid(ctx, cfg, "UpdateScopePermission", func() error {
		body, err := json.Marshal(perm)
		if err != nil {
			return err
		}
		return c.Update(ctx, path, body)
	})
}

// ============================================================================
// Identity Provider Mapper Operations
// ============================================================================

// IdentityProviderMapperRepresentation represents a Keycloak identity provider mapper
// (minimal fields we need)
type IdentityProviderMapperRepresentation struct {
	ID                     *string           `json:"id,omitempty"`
	Name                   *string           `json:"name,omitempty"`
	IdentityProviderAlias  *string           `json:"identityProviderAlias,omitempty"`
	IdentityProviderMapper *string           `json:"identityProviderMapper,omitempty"`
	Config                 map[string]string `json:"config,omitempty"`
}

// idpMappersPath builds the IdP-mappers REST path for a given realm and IdP alias.
func idpMappersPath(realmName, alias string) string {
	return "/admin/realms/" + url.PathEscape(realmName) + "/identity-provider/instances/" + url.PathEscape(alias) + "/mappers"
}

// CreateIdentityProviderMapper creates a mapper on an identity provider
func (c *Client) CreateIdentityProviderMapper(ctx context.Context, realmName, alias string, mapperDef json.RawMessage) (string, error) {
	cfg := DefaultRetryConfig()
	return WithRetry(ctx, cfg, "CreateIdentityProviderMapper", func() (string, error) {
		return c.Create(ctx, idpMappersPath(realmName, alias), mapperDef)
	})
}

// GetIdentityProviderMappers gets all mappers for an identity provider
func (c *Client) GetIdentityProviderMappers(ctx context.Context, realmName, alias string) ([]IdentityProviderMapperRepresentation, error) {
	var mappers []IdentityProviderMapperRepresentation
	if err := c.List(ctx, idpMappersPath(realmName, alias), nil, &mappers); err != nil {
		return nil, err
	}
	return mappers, nil
}

// GetIdentityProviderMappersRaw gets all mappers for an identity provider as raw JSON
func (c *Client) GetIdentityProviderMappersRaw(ctx context.Context, realmName, alias string) ([]json.RawMessage, error) {
	return c.ListRaw(ctx, idpMappersPath(realmName, alias), nil)
}

// GetIdentityProviderMapper gets a single mapper on an identity provider by ID
func (c *Client) GetIdentityProviderMapper(ctx context.Context, realmName, alias, mapperID string) (*IdentityProviderMapperRepresentation, error) {
	var mapper IdentityProviderMapperRepresentation
	if err := c.Get(ctx, idpMappersPath(realmName, alias)+"/"+url.PathEscape(mapperID), &mapper); err != nil {
		return nil, err
	}
	return &mapper, nil
}

// GetIdentityProviderMapperByName finds a mapper by name on an identity provider
func (c *Client) GetIdentityProviderMapperByName(ctx context.Context, realmName, alias, name string) (*IdentityProviderMapperRepresentation, error) {
	mappers, err := c.GetIdentityProviderMappers(ctx, realmName, alias)
	if err != nil {
		return nil, err
	}
	for i := range mappers {
		if mappers[i].Name != nil && *mappers[i].Name == name {
			return &mappers[i], nil
		}
	}
	return nil, fmt.Errorf("identity provider mapper not found: %s", name)
}

// UpdateIdentityProviderMapper updates a mapper on an identity provider
func (c *Client) UpdateIdentityProviderMapper(ctx context.Context, realmName, alias, mapperID string, mapperDef json.RawMessage) error {
	cfg := DefaultRetryConfig()
	return WithRetryVoid(ctx, cfg, "UpdateIdentityProviderMapper", func() error {
		return c.Update(ctx, idpMappersPath(realmName, alias)+"/"+url.PathEscape(mapperID), mapperDef)
	})
}

// DeleteIdentityProviderMapper deletes a mapper from an identity provider
func (c *Client) DeleteIdentityProviderMapper(ctx context.Context, realmName, alias, mapperID string) error {
	return c.Delete(ctx, idpMappersPath(realmName, alias)+"/"+url.PathEscape(mapperID))
}

// ============================================================================
// Role Operations
// ============================================================================

// RoleRepresentation represents a Keycloak role (minimal fields we need)
type RoleRepresentation struct {
	ID          *string `json:"id,omitempty"`
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
	Composite   *bool   `json:"composite,omitempty"`
	ClientRole  *bool   `json:"clientRole,omitempty"`
	ContainerID *string `json:"containerId,omitempty"`
}

// CreateRealmRole creates a new realm role
func (c *Client) CreateRealmRole(ctx context.Context, realmName string, roleDef json.RawMessage) (string, error) {
	return c.Create(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/roles", roleDef)
}

// GetRealmRole gets a realm role by name
func (c *Client) GetRealmRole(ctx context.Context, realmName, roleName string) (*RoleRepresentation, error) {
	var role RoleRepresentation
	if err := c.Get(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/roles/"+url.PathEscape(roleName), &role); err != nil {
		return nil, err
	}
	return &role, nil
}

// UpdateRealmRole updates a realm role
func (c *Client) UpdateRealmRole(ctx context.Context, realmName, roleName string, roleDef json.RawMessage) error {
	return c.Update(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/roles/"+url.PathEscape(roleName), roleDef)
}

// DeleteRealmRole deletes a realm role
func (c *Client) DeleteRealmRole(ctx context.Context, realmName, roleName string) error {
	return c.Delete(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/roles/"+url.PathEscape(roleName))
}

// CreateClientRole creates a client role
func (c *Client) CreateClientRole(ctx context.Context, realmName, clientID string, roleDef json.RawMessage) (string, error) {
	cfg := DefaultRetryConfig()
	return WithRetry(ctx, cfg, "CreateClientRole", func() (string, error) {
		return c.Create(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/clients/"+url.PathEscape(clientID)+"/roles", roleDef)
	})
}

// GetClientRole gets a client role by name
func (c *Client) GetClientRole(ctx context.Context, realmName, clientID, roleName string) (*RoleRepresentation, error) {
	var role RoleRepresentation
	if err := c.Get(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/clients/"+url.PathEscape(clientID)+"/roles/"+url.PathEscape(roleName), &role); err != nil {
		return nil, err
	}
	return &role, nil
}

// UpdateClientRole updates a client role
func (c *Client) UpdateClientRole(ctx context.Context, realmName, clientID, roleName string, roleDef json.RawMessage) error {
	cfg := DefaultRetryConfig()
	return WithRetryVoid(ctx, cfg, "UpdateClientRole", func() error {
		return c.Update(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/clients/"+url.PathEscape(clientID)+"/roles/"+url.PathEscape(roleName), roleDef)
	})
}

// DeleteClientRole deletes a client role
func (c *Client) DeleteClientRole(ctx context.Context, realmName, clientID, roleName string) error {
	return c.Delete(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/clients/"+url.PathEscape(clientID)+"/roles/"+url.PathEscape(roleName))
}

func realmRoleCompositesPath(realmName, roleName string) string {
	return "/admin/realms/" + url.PathEscape(realmName) + "/roles/" + url.PathEscape(roleName) + "/composites"
}

func clientRoleCompositesPath(realmName, clientID, roleName string) string {
	return "/admin/realms/" + url.PathEscape(realmName) + "/clients/" + url.PathEscape(clientID) + "/roles/" + url.PathEscape(roleName) + "/composites"
}

// GetRealmRoleComposites lists composite members of a realm role.
func (c *Client) GetRealmRoleComposites(ctx context.Context, realmName, roleName string) ([]RoleRepresentation, error) {
	var roles []RoleRepresentation
	if err := c.Get(ctx, realmRoleCompositesPath(realmName, roleName), &roles); err != nil {
		return nil, err
	}
	return roles, nil
}

// AddRealmRoleComposites adds composite members to a realm role.
func (c *Client) AddRealmRoleComposites(ctx context.Context, realmName, roleName string, roles []RoleRepresentation) error {
	if len(roles) == 0 {
		return nil
	}
	cfg := DefaultRetryConfig()
	return WithRetryVoid(ctx, cfg, "AddRealmRoleComposites", func() error {
		return c.Post(ctx, realmRoleCompositesPath(realmName, roleName), roles, nil)
	})
}

// RemoveRealmRoleComposites removes composite members from a realm role.
func (c *Client) RemoveRealmRoleComposites(ctx context.Context, realmName, roleName string, roles []RoleRepresentation) error {
	if len(roles) == 0 {
		return nil
	}
	req, err := c.request(ctx)
	if err != nil {
		return err
	}
	resp, err := req.SetBody(roles).Delete(c.baseURL + realmRoleCompositesPath(realmName, roleName))
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	if resp.IsError() {
		return fmt.Errorf("%s: %s", resp.Status(), string(resp.Body()))
	}
	return nil
}

// GetClientRoleComposites lists composite members of a client role.
func (c *Client) GetClientRoleComposites(ctx context.Context, realmName, clientID, roleName string) ([]RoleRepresentation, error) {
	var roles []RoleRepresentation
	if err := c.Get(ctx, clientRoleCompositesPath(realmName, clientID, roleName), &roles); err != nil {
		return nil, err
	}
	return roles, nil
}

// AddClientRoleComposites adds composite members to a client role.
func (c *Client) AddClientRoleComposites(ctx context.Context, realmName, clientID, roleName string, roles []RoleRepresentation) error {
	if len(roles) == 0 {
		return nil
	}
	cfg := DefaultRetryConfig()
	return WithRetryVoid(ctx, cfg, "AddClientRoleComposites", func() error {
		return c.Post(ctx, clientRoleCompositesPath(realmName, clientID, roleName), roles, nil)
	})
}

// RemoveClientRoleComposites removes composite members from a client role.
func (c *Client) RemoveClientRoleComposites(ctx context.Context, realmName, clientID, roleName string, roles []RoleRepresentation) error {
	if len(roles) == 0 {
		return nil
	}
	req, err := c.request(ctx)
	if err != nil {
		return err
	}
	resp, err := req.SetBody(roles).Delete(c.baseURL + clientRoleCompositesPath(realmName, clientID, roleName))
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	if resp.IsError() {
		return fmt.Errorf("%s: %s", resp.Status(), string(resp.Body()))
	}
	return nil
}

// ============================================================================
// Role Mapping Operations
// ============================================================================

// AddRealmRolesToUser adds realm roles to a user
func (c *Client) AddRealmRolesToUser(ctx context.Context, realmName, userID string, roles []RoleRepresentation) error {
	cfg := DefaultRetryConfig()
	return WithRetryVoid(ctx, cfg, "AddRealmRolesToUser", func() error {
		return c.Post(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/users/"+url.PathEscape(userID)+"/role-mappings/realm", roles, nil)
	})
}

// DeleteRealmRolesFromUser removes realm roles from a user
func (c *Client) DeleteRealmRolesFromUser(ctx context.Context, realmName, userID string, roles []RoleRepresentation) error {
	req, err := c.request(ctx)
	if err != nil {
		return err
	}
	resp, err := req.SetBody(roles).Delete(c.baseURL + "/admin/realms/" + url.PathEscape(realmName) + "/users/" + url.PathEscape(userID) + "/role-mappings/realm")
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	if resp.IsError() {
		return fmt.Errorf("%s: %s", resp.Status(), string(resp.Body()))
	}
	return nil
}

// AddClientRolesToUser adds client roles to a user
func (c *Client) AddClientRolesToUser(ctx context.Context, realmName, clientID, userID string, roles []RoleRepresentation) error {
	cfg := DefaultRetryConfig()
	return WithRetryVoid(ctx, cfg, "AddClientRolesToUser", func() error {
		return c.Post(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/users/"+url.PathEscape(userID)+"/role-mappings/clients/"+url.PathEscape(clientID), roles, nil)
	})
}

// DeleteClientRolesFromUser removes client roles from a user
func (c *Client) DeleteClientRolesFromUser(ctx context.Context, realmName, clientID, userID string, roles []RoleRepresentation) error {
	req, err := c.request(ctx)
	if err != nil {
		return err
	}
	resp, err := req.SetBody(roles).Delete(c.baseURL + "/admin/realms/" + url.PathEscape(realmName) + "/users/" + url.PathEscape(userID) + "/role-mappings/clients/" + url.PathEscape(clientID))
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	if resp.IsError() {
		return fmt.Errorf("%s: %s", resp.Status(), string(resp.Body()))
	}
	return nil
}

// AddRealmRolesToGroup adds realm roles to a group
func (c *Client) AddRealmRolesToGroup(ctx context.Context, realmName, groupID string, roles []RoleRepresentation) error {
	cfg := DefaultRetryConfig()
	return WithRetryVoid(ctx, cfg, "AddRealmRolesToGroup", func() error {
		return c.Post(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/groups/"+url.PathEscape(groupID)+"/role-mappings/realm", roles, nil)
	})
}

// DeleteRealmRolesFromGroup removes realm roles from a group
func (c *Client) DeleteRealmRolesFromGroup(ctx context.Context, realmName, groupID string, roles []RoleRepresentation) error {
	req, err := c.request(ctx)
	if err != nil {
		return err
	}
	resp, err := req.SetBody(roles).Delete(c.baseURL + "/admin/realms/" + url.PathEscape(realmName) + "/groups/" + url.PathEscape(groupID) + "/role-mappings/realm")
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	if resp.IsError() {
		return fmt.Errorf("%s: %s", resp.Status(), string(resp.Body()))
	}
	return nil
}

// AddClientRolesToGroup adds client roles to a group
func (c *Client) AddClientRolesToGroup(ctx context.Context, realmName, clientID, groupID string, roles []RoleRepresentation) error {
	cfg := DefaultRetryConfig()
	return WithRetryVoid(ctx, cfg, "AddClientRolesToGroup", func() error {
		return c.Post(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/groups/"+url.PathEscape(groupID)+"/role-mappings/clients/"+url.PathEscape(clientID), roles, nil)
	})
}

// DeleteClientRolesFromGroup removes client roles from a group
func (c *Client) DeleteClientRolesFromGroup(ctx context.Context, realmName, clientID, groupID string, roles []RoleRepresentation) error {
	req, err := c.request(ctx)
	if err != nil {
		return err
	}
	resp, err := req.SetBody(roles).Delete(c.baseURL + "/admin/realms/" + url.PathEscape(realmName) + "/groups/" + url.PathEscape(groupID) + "/role-mappings/clients/" + url.PathEscape(clientID))
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	if resp.IsError() {
		return fmt.Errorf("%s: %s", resp.Status(), string(resp.Body()))
	}
	return nil
}

// ============================================================================
// Protocol Mapper Operations
// ============================================================================

// ProtocolMapperRepresentation represents a protocol mapper (minimal fields we need)
type ProtocolMapperRepresentation struct {
	ID              *string           `json:"id,omitempty"`
	Name            *string           `json:"name,omitempty"`
	Protocol        *string           `json:"protocol,omitempty"`
	ProtocolMapper  *string           `json:"protocolMapper,omitempty"`
	ConsentRequired *bool             `json:"consentRequired,omitempty"`
	Config          map[string]string `json:"config,omitempty"`
}

// CreateClientProtocolMapper creates a protocol mapper for a client
func (c *Client) CreateClientProtocolMapper(ctx context.Context, realmName, clientID string, mapperDef json.RawMessage) (string, error) {
	cfg := DefaultRetryConfig()
	return WithRetry(ctx, cfg, "CreateClientProtocolMapper", func() (string, error) {
		return c.Create(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/clients/"+url.PathEscape(clientID)+"/protocol-mappers/models", mapperDef)
	})
}

// GetClientProtocolMapper gets a protocol mapper by ID
func (c *Client) GetClientProtocolMapper(ctx context.Context, realmName, clientID, mapperID string) (*ProtocolMapperRepresentation, error) {
	var mapper ProtocolMapperRepresentation
	if err := c.Get(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/clients/"+url.PathEscape(clientID)+"/protocol-mappers/models/"+url.PathEscape(mapperID), &mapper); err != nil {
		return nil, err
	}
	return &mapper, nil
}

// GetClientProtocolMappers gets all protocol mappers for a client
func (c *Client) GetClientProtocolMappers(ctx context.Context, realmName, clientID string) ([]ProtocolMapperRepresentation, error) {
	var mappers []ProtocolMapperRepresentation
	if err := c.List(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/clients/"+url.PathEscape(clientID)+"/protocol-mappers/models", nil, &mappers); err != nil {
		return nil, err
	}
	return mappers, nil
}

// GetClientProtocolMapperByName finds a protocol mapper by name
func (c *Client) GetClientProtocolMapperByName(ctx context.Context, realmName, clientID, name string) (*ProtocolMapperRepresentation, error) {
	mappers, err := c.GetClientProtocolMappers(ctx, realmName, clientID)
	if err != nil {
		return nil, err
	}
	for i := range mappers {
		if mappers[i].Name != nil && *mappers[i].Name == name {
			return &mappers[i], nil
		}
	}
	return nil, fmt.Errorf("protocol mapper not found: %s", name)
}

// UpdateClientProtocolMapper updates a protocol mapper
func (c *Client) UpdateClientProtocolMapper(ctx context.Context, realmName, clientID, mapperID string, mapperDef json.RawMessage) error {
	cfg := DefaultRetryConfig()
	return WithRetryVoid(ctx, cfg, "UpdateClientProtocolMapper", func() error {
		return c.Update(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/clients/"+url.PathEscape(clientID)+"/protocol-mappers/models/"+url.PathEscape(mapperID), mapperDef)
	})
}

// DeleteClientProtocolMapper deletes a protocol mapper
func (c *Client) DeleteClientProtocolMapper(ctx context.Context, realmName, clientID, mapperID string) error {
	return c.Delete(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/clients/"+url.PathEscape(clientID)+"/protocol-mappers/models/"+url.PathEscape(mapperID))
}

// CreateClientScopeProtocolMapper creates a protocol mapper for a client scope
func (c *Client) CreateClientScopeProtocolMapper(ctx context.Context, realmName, scopeID string, mapperDef json.RawMessage) (string, error) {
	cfg := DefaultRetryConfig()
	return WithRetry(ctx, cfg, "CreateClientScopeProtocolMapper", func() (string, error) {
		return c.Create(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/client-scopes/"+url.PathEscape(scopeID)+"/protocol-mappers/models", mapperDef)
	})
}

// GetClientScopeProtocolMappers gets all protocol mappers for a client scope
func (c *Client) GetClientScopeProtocolMappers(ctx context.Context, realmName, scopeID string) ([]ProtocolMapperRepresentation, error) {
	var mappers []ProtocolMapperRepresentation
	if err := c.List(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/client-scopes/"+url.PathEscape(scopeID)+"/protocol-mappers/models", nil, &mappers); err != nil {
		return nil, err
	}
	return mappers, nil
}

// GetClientScopeProtocolMapperByName finds a protocol mapper by name in a client scope
func (c *Client) GetClientScopeProtocolMapperByName(ctx context.Context, realmName, scopeID, name string) (*ProtocolMapperRepresentation, error) {
	mappers, err := c.GetClientScopeProtocolMappers(ctx, realmName, scopeID)
	if err != nil {
		return nil, err
	}
	for i := range mappers {
		if mappers[i].Name != nil && *mappers[i].Name == name {
			return &mappers[i], nil
		}
	}
	return nil, fmt.Errorf("protocol mapper not found: %s", name)
}

// UpdateClientScopeProtocolMapper updates a protocol mapper in a client scope
func (c *Client) UpdateClientScopeProtocolMapper(ctx context.Context, realmName, scopeID, mapperID string, mapperDef json.RawMessage) error {
	cfg := DefaultRetryConfig()
	return WithRetryVoid(ctx, cfg, "UpdateClientScopeProtocolMapper", func() error {
		return c.Update(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/client-scopes/"+url.PathEscape(scopeID)+"/protocol-mappers/models/"+url.PathEscape(mapperID), mapperDef)
	})
}

// DeleteClientScopeProtocolMapper deletes a protocol mapper from a client scope
func (c *Client) DeleteClientScopeProtocolMapper(ctx context.Context, realmName, scopeID, mapperID string) error {
	return c.Delete(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/client-scopes/"+url.PathEscape(scopeID)+"/protocol-mappers/models/"+url.PathEscape(mapperID))
}

// ============================================================================
// Component Operations
// ============================================================================

// ComponentRepresentation represents a Keycloak component (minimal fields we need)
type ComponentRepresentation struct {
	ID           *string `json:"id,omitempty"`
	Name         *string `json:"name,omitempty"`
	ProviderID   *string `json:"providerId,omitempty"`
	ProviderType *string `json:"providerType,omitempty"`
	ParentID     *string `json:"parentId,omitempty"`
}

// CreateComponent creates a component
func (c *Client) CreateComponent(ctx context.Context, realmName string, componentDef json.RawMessage) (string, error) {
	cfg := DefaultRetryConfig()
	return WithRetry(ctx, cfg, "CreateComponent", func() (string, error) {
		return c.Create(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/components", componentDef)
	})
}

// GetComponent gets a component by ID
func (c *Client) GetComponent(ctx context.Context, realmName, componentID string) (*ComponentRepresentation, error) {
	var component ComponentRepresentation
	if err := c.Get(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/components/"+url.PathEscape(componentID), &component); err != nil {
		return nil, err
	}
	return &component, nil
}

// GetComponents gets components with optional filtering
func (c *Client) GetComponents(ctx context.Context, realmName string, params map[string]string) ([]ComponentRepresentation, error) {
	var components []ComponentRepresentation
	if err := c.List(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/components", params, &components); err != nil {
		return nil, err
	}
	return components, nil
}

// GetComponentByName finds a component by name and type
func (c *Client) GetComponentByName(ctx context.Context, realmName, name, providerType string) (*ComponentRepresentation, error) {
	params := map[string]string{"name": name}
	if providerType != "" {
		params["type"] = providerType
	}
	components, err := c.GetComponents(ctx, realmName, params)
	if err != nil {
		return nil, err
	}
	if len(components) == 0 {
		return nil, fmt.Errorf("component not found: %s", name)
	}
	return &components[0], nil
}

// UpdateComponent updates a component
func (c *Client) UpdateComponent(ctx context.Context, realmName, componentID string, componentDef json.RawMessage) error {
	cfg := DefaultRetryConfig()
	return WithRetryVoid(ctx, cfg, "UpdateComponent", func() error {
		return c.Update(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/components/"+url.PathEscape(componentID), componentDef)
	})
}

// DeleteComponent deletes a component
func (c *Client) DeleteComponent(ctx context.Context, realmName, componentID string) error {
	return c.Delete(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/components/"+url.PathEscape(componentID))
}

// ============================================================================
// Organization Operations (Keycloak 26+)
// ============================================================================

// OrganizationRepresentation represents a Keycloak organization
type OrganizationRepresentation struct {
	ID          string               `json:"id,omitempty"`
	Name        string               `json:"name,omitempty"`
	Alias       string               `json:"alias,omitempty"`
	Description string               `json:"description,omitempty"`
	Enabled     *bool                `json:"enabled,omitempty"`
	Domains     []OrganizationDomain `json:"domains,omitempty"`
	Attributes  map[string][]string  `json:"attributes,omitempty"`
}

// OrganizationDomain represents a domain associated with an organization
type OrganizationDomain struct {
	Name     string `json:"name,omitempty"`
	Verified bool   `json:"verified,omitempty"`
}

// GetOrganizations gets all organizations in a realm
func (c *Client) GetOrganizations(ctx context.Context, realmName string) ([]OrganizationRepresentation, error) {
	return listAll[OrganizationRepresentation](ctx, c, "/admin/realms/"+url.PathEscape(realmName)+"/organizations", nil)
}

// GetOrganization gets an organization by ID
func (c *Client) GetOrganization(ctx context.Context, realmName, orgID string) (*OrganizationRepresentation, error) {
	var org OrganizationRepresentation
	if err := c.Get(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/organizations/"+url.PathEscape(orgID), &org); err != nil {
		return nil, err
	}
	return &org, nil
}

// CreateOrganization creates a new organization
func (c *Client) CreateOrganization(ctx context.Context, realmName string, org OrganizationRepresentation) (string, error) {
	return c.Create(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/organizations", org)
}

// UpdateOrganization updates an existing organization
func (c *Client) UpdateOrganization(ctx context.Context, realmName string, org OrganizationRepresentation) error {
	if org.ID == "" {
		return fmt.Errorf("organization ID is required for update")
	}
	return c.Update(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/organizations/"+url.PathEscape(org.ID), org)
}

// DeleteOrganization deletes an organization
func (c *Client) DeleteOrganization(ctx context.Context, realmName, orgID string) error {
	return c.Delete(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/organizations/"+url.PathEscape(orgID))
}

// ============================================================================
// Required Action Operations
// ============================================================================

// RequiredActionProviderRepresentation represents a Keycloak required action provider
type RequiredActionProviderRepresentation struct {
	Alias         *string           `json:"alias,omitempty"`
	Name          *string           `json:"name,omitempty"`
	ProviderID    *string           `json:"providerId,omitempty"`
	Enabled       *bool             `json:"enabled,omitempty"`
	DefaultAction *bool             `json:"defaultAction,omitempty"`
	Priority      *int32            `json:"priority,omitempty"`
	Config        map[string]string `json:"config,omitempty"`
}

// GetRequiredActions lists all required action providers in a realm
func (c *Client) GetRequiredActions(ctx context.Context, realmName string) ([]RequiredActionProviderRepresentation, error) {
	var actions []RequiredActionProviderRepresentation
	if err := c.List(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/authentication/required-actions", nil, &actions); err != nil {
		return nil, err
	}
	return actions, nil
}

// GetRequiredAction gets a required action by alias
func (c *Client) GetRequiredAction(ctx context.Context, realmName, alias string) (*RequiredActionProviderRepresentation, error) {
	var action RequiredActionProviderRepresentation
	if err := c.Get(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/authentication/required-actions/"+url.PathEscape(alias), &action); err != nil {
		return nil, err
	}
	return &action, nil
}

// UpdateRequiredAction updates a required action
func (c *Client) UpdateRequiredAction(ctx context.Context, realmName, alias string, action json.RawMessage) error {
	cfg := DefaultRetryConfig()
	return WithRetryVoid(ctx, cfg, "UpdateRequiredAction", func() error {
		return c.Update(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/authentication/required-actions/"+url.PathEscape(alias), action)
	})
}

// RegisterRequiredAction registers a new required action provider
func (c *Client) RegisterRequiredAction(ctx context.Context, realmName string, action json.RawMessage) error {
	cfg := DefaultRetryConfig()
	return WithRetryVoid(ctx, cfg, "RegisterRequiredAction", func() error {
		return c.Post(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/authentication/register-required-action", action, nil)
	})
}

// DeleteRequiredAction deletes (unregisters) a required action
func (c *Client) DeleteRequiredAction(ctx context.Context, realmName, alias string) error {
	return c.Delete(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/authentication/required-actions/"+url.PathEscape(alias))
}

// ============================================================================
// Authentication Flow Operations
// ============================================================================

// AuthenticationFlowRepresentation represents a Keycloak authentication flow
type AuthenticationFlowRepresentation struct {
	ID          *string `json:"id,omitempty"`
	Alias       *string `json:"alias,omitempty"`
	Description *string `json:"description,omitempty"`
	ProviderID  *string `json:"providerId,omitempty"`
	TopLevel    *bool   `json:"topLevel,omitempty"`
	BuiltIn     *bool   `json:"builtIn,omitempty"`
}

// AuthenticationExecutionInfo represents an execution within a flow as returned
// by GET /authentication/flows/{flowAlias}/executions. This is a flat list with
// level/index fields indicating the tree structure.
type AuthenticationExecutionInfo struct {
	ID                   *string `json:"id,omitempty"`
	Requirement          *string `json:"requirement,omitempty"`
	DisplayName          *string `json:"displayName,omitempty"`
	Alias                *string `json:"alias,omitempty"`
	Description          *string `json:"description,omitempty"`
	Configurable         *bool   `json:"configurable,omitempty"`
	AuthenticationFlow   *bool   `json:"authenticationFlow,omitempty"`
	ProviderID           *string `json:"providerId,omitempty"`
	AuthenticationConfig *string `json:"authenticationConfig,omitempty"`
	FlowID               *string `json:"flowId,omitempty"`
	Level                *int    `json:"level,omitempty"`
	Index                *int    `json:"index,omitempty"`
	// Priority is only honored on PUT since Keycloak 25
	// (keycloak/keycloak#27751); older versions silently drop it.
	Priority           *int     `json:"priority,omitempty"`
	RequirementChoices []string `json:"requirementChoices,omitempty"`
}

// AuthenticatorConfigRepresentation represents an authenticator config
type AuthenticatorConfigRepresentation struct {
	ID     *string           `json:"id,omitempty"`
	Alias  *string           `json:"alias,omitempty"`
	Config map[string]string `json:"config,omitempty"`
}

// GetAuthenticationFlows lists all authentication flows in a realm
func (c *Client) GetAuthenticationFlows(ctx context.Context, realmName string) ([]AuthenticationFlowRepresentation, error) {
	var flows []AuthenticationFlowRepresentation
	if err := c.List(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/authentication/flows", nil, &flows); err != nil {
		return nil, err
	}
	return flows, nil
}

// GetAuthenticationFlowByAlias finds an authentication flow by its alias
func (c *Client) GetAuthenticationFlowByAlias(ctx context.Context, realmName, alias string) (*AuthenticationFlowRepresentation, error) {
	flows, err := c.GetAuthenticationFlows(ctx, realmName)
	if err != nil {
		return nil, err
	}
	for i := range flows {
		if flows[i].Alias != nil && *flows[i].Alias == alias {
			return &flows[i], nil
		}
	}
	return nil, fmt.Errorf("authentication flow not found: %s", alias)
}

// CreateAuthenticationFlow creates a new top-level authentication flow
func (c *Client) CreateAuthenticationFlow(ctx context.Context, realmName string, flow AuthenticationFlowRepresentation) (string, error) {
	cfg := DefaultRetryConfig()
	return WithRetry(ctx, cfg, "CreateAuthenticationFlow", func() (string, error) {
		return c.Create(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/authentication/flows", flow)
	})
}

// DeleteAuthenticationFlow deletes an authentication flow by ID
func (c *Client) DeleteAuthenticationFlow(ctx context.Context, realmName, flowID string) error {
	return c.Delete(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/authentication/flows/"+url.PathEscape(flowID))
}

// UpdateAuthenticationFlow updates a top-level authentication flow's
// mutable fields (description, etc.) without recreating it. Provider type
// changes on a top-level flow are not supported by Keycloak.
func (c *Client) UpdateAuthenticationFlow(ctx context.Context, realmName, flowID string, flow AuthenticationFlowRepresentation) error {
	cfg := DefaultRetryConfig()
	return WithRetryVoid(ctx, cfg, "UpdateAuthenticationFlow", func() error {
		return c.Update(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/authentication/flows/"+url.PathEscape(flowID), flow)
	})
}

// GetFlowExecutions returns the flat execution list for a flow
func (c *Client) GetFlowExecutions(ctx context.Context, realmName, flowAlias string) ([]AuthenticationExecutionInfo, error) {
	var executions []AuthenticationExecutionInfo
	path := "/admin/realms/" + url.PathEscape(realmName) + "/authentication/flows/" + url.PathEscape(flowAlias) + "/executions"
	if err := c.List(ctx, path, nil, &executions); err != nil {
		return nil, err
	}
	return executions, nil
}

// UpdateFlowExecution updates an execution's requirement within a flow
func (c *Client) UpdateFlowExecution(ctx context.Context, realmName, flowAlias string, execution AuthenticationExecutionInfo) error {
	cfg := DefaultRetryConfig()
	return WithRetryVoid(ctx, cfg, "UpdateFlowExecution", func() error {
		path := "/admin/realms/" + url.PathEscape(realmName) + "/authentication/flows/" + url.PathEscape(flowAlias) + "/executions"
		return c.Update(ctx, path, execution)
	})
}

// AddFlowExecution adds an authenticator execution to a flow
func (c *Client) AddFlowExecution(ctx context.Context, realmName, flowAlias, provider string) (string, error) {
	cfg := DefaultRetryConfig()
	body := map[string]string{"provider": provider}
	return WithRetry(ctx, cfg, "AddFlowExecution", func() (string, error) {
		path := "/admin/realms/" + url.PathEscape(realmName) + "/authentication/flows/" + url.PathEscape(flowAlias) + "/executions/execution"
		return c.Create(ctx, path, body)
	})
}

// AddFlowSubFlow adds a sub-flow to a parent flow
func (c *Client) AddFlowSubFlow(ctx context.Context, realmName, parentFlowAlias string, subFlow map[string]interface{}) (string, error) {
	cfg := DefaultRetryConfig()
	return WithRetry(ctx, cfg, "AddFlowSubFlow", func() (string, error) {
		path := "/admin/realms/" + url.PathEscape(realmName) + "/authentication/flows/" + url.PathEscape(parentFlowAlias) + "/executions/flow"
		return c.Create(ctx, path, subFlow)
	})
}

// DeleteExecution deletes a specific execution by ID
func (c *Client) DeleteExecution(ctx context.Context, realmName, executionID string) error {
	return c.Delete(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/authentication/executions/"+url.PathEscape(executionID))
}

// RaiseExecutionPriority moves an execution higher (earlier) in the flow
func (c *Client) RaiseExecutionPriority(ctx context.Context, realmName, executionID string) error {
	return c.Post(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/authentication/executions/"+url.PathEscape(executionID)+"/raise-priority", nil, nil)
}

// LowerExecutionPriority moves an execution lower (later) in the flow
func (c *Client) LowerExecutionPriority(ctx context.Context, realmName, executionID string) error {
	return c.Post(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/authentication/executions/"+url.PathEscape(executionID)+"/lower-priority", nil, nil)
}

// CreateExecutionConfig sets authenticator config on an execution
func (c *Client) CreateExecutionConfig(ctx context.Context, realmName, executionID string, config AuthenticatorConfigRepresentation) (string, error) {
	cfg := DefaultRetryConfig()
	return WithRetry(ctx, cfg, "CreateExecutionConfig", func() (string, error) {
		path := "/admin/realms/" + url.PathEscape(realmName) + "/authentication/executions/" + url.PathEscape(executionID) + "/config"
		return c.Create(ctx, path, config)
	})
}

// GetExecutionConfig fetches an authenticator config by its ID. Used by the
// flow reconciler to compare the live config against the desired one before
// deciding to PUT.
func (c *Client) GetExecutionConfig(ctx context.Context, realmName, configID string) (*AuthenticatorConfigRepresentation, error) {
	var config AuthenticatorConfigRepresentation
	path := "/admin/realms/" + url.PathEscape(realmName) + "/authentication/config/" + url.PathEscape(configID)
	if err := c.Get(ctx, path, &config); err != nil {
		return nil, err
	}
	return &config, nil
}

// UpdateExecutionConfig replaces the contents of an existing authenticator
// config. The config's ID must be set on the representation.
func (c *Client) UpdateExecutionConfig(ctx context.Context, realmName, configID string, config AuthenticatorConfigRepresentation) error {
	cfg := DefaultRetryConfig()
	return WithRetryVoid(ctx, cfg, "UpdateExecutionConfig", func() error {
		path := "/admin/realms/" + url.PathEscape(realmName) + "/authentication/config/" + url.PathEscape(configID)
		return c.Update(ctx, path, config)
	})
}

// DeleteExecutionConfig removes an authenticator config by ID.
func (c *Client) DeleteExecutionConfig(ctx context.Context, realmName, configID string) error {
	return c.Delete(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/authentication/config/"+url.PathEscape(configID))
}

// ============================================================================
// Raw JSON Operations (for export)
// ============================================================================

// GetRaw retrieves a resource as raw JSON (full representation)
func (c *Client) GetRaw(ctx context.Context, path string) (json.RawMessage, error) {
	req, err := c.request(ctx)
	if err != nil {
		return nil, err
	}

	resp, err := req.Get(c.baseURL + path)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("%s: %s", resp.Status(), string(resp.Body()))
	}

	return resp.Body(), nil
}

// ListRaw retrieves a list of resources as raw JSON array
func (c *Client) ListRaw(ctx context.Context, path string, params map[string]string) ([]json.RawMessage, error) {
	req, err := c.request(ctx)
	if err != nil {
		return nil, err
	}

	if params != nil {
		req.SetQueryParams(params)
	}

	resp, err := req.Get(c.baseURL + path)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("%s: %s", resp.Status(), string(resp.Body()))
	}

	// Parse as array of raw messages
	var items []json.RawMessage
	if err := json.Unmarshal(resp.Body(), &items); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return items, nil
}

// GetRealmRaw gets a realm as raw JSON (full representation)
func (c *Client) GetRealmRaw(ctx context.Context, realmName string) (json.RawMessage, error) {
	return c.GetRaw(ctx, "/admin/realms/"+url.PathEscape(realmName))
}

// GetClientsRaw gets all clients in a realm as raw JSON
func (c *Client) GetClientsRaw(ctx context.Context, realmName string) ([]json.RawMessage, error) {
	return c.ListRaw(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/clients", nil)
}

// GetClientRaw gets a client by internal ID as raw JSON
func (c *Client) GetClientRaw(ctx context.Context, realmName, clientUUID string) (json.RawMessage, error) {
	return c.GetRaw(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/clients/"+url.PathEscape(clientUUID))
}

// GetUsersRaw gets all users in a realm as raw JSON
func (c *Client) GetUsersRaw(ctx context.Context, realmName string, params map[string]string) ([]json.RawMessage, error) {
	return c.ListRaw(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/users", params)
}

// GetUserRaw gets a user by ID as raw JSON
func (c *Client) GetUserRaw(ctx context.Context, realmName, userID string) (json.RawMessage, error) {
	return c.GetRaw(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/users/"+url.PathEscape(userID))
}

// GetGroupsRaw gets all groups in a realm as raw JSON
func (c *Client) GetGroupsRaw(ctx context.Context, realmName string) ([]json.RawMessage, error) {
	return c.ListRaw(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/groups", nil)
}

// GetGroupRaw gets a group by ID as raw JSON
func (c *Client) GetGroupRaw(ctx context.Context, realmName, groupID string) (json.RawMessage, error) {
	return c.GetRaw(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/groups/"+url.PathEscape(groupID))
}

// GetGroupChildrenRaw gets direct children of a group as raw JSON.
// Required since Keycloak 23+, where /groups no longer inlines nested subGroups.
func (c *Client) GetGroupChildrenRaw(ctx context.Context, realmName, groupID string, params map[string]string) ([]json.RawMessage, error) {
	return c.ListRaw(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/groups/"+url.PathEscape(groupID)+"/children", params)
}

// GetClientScopesRaw gets all client scopes in a realm as raw JSON
func (c *Client) GetClientScopesRaw(ctx context.Context, realmName string) ([]json.RawMessage, error) {
	return c.ListRaw(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/client-scopes", nil)
}

// GetClientScopeRaw gets a client scope by ID as raw JSON
func (c *Client) GetClientScopeRaw(ctx context.Context, realmName, scopeID string) (json.RawMessage, error) {
	return c.GetRaw(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/client-scopes/"+url.PathEscape(scopeID))
}

// GetIdentityProviders gets all identity providers in a realm
func (c *Client) GetIdentityProviders(ctx context.Context, realmName string) ([]IdentityProviderRepresentation, error) {
	var idps []IdentityProviderRepresentation
	if err := c.List(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/identity-provider/instances", nil, &idps); err != nil {
		return nil, err
	}
	return idps, nil
}

// GetIdentityProvidersRaw gets all identity providers in a realm as raw JSON
func (c *Client) GetIdentityProvidersRaw(ctx context.Context, realmName string) ([]json.RawMessage, error) {
	return c.ListRaw(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/identity-provider/instances", nil)
}

// GetIdentityProviderRaw gets an identity provider by alias as raw JSON
func (c *Client) GetIdentityProviderRaw(ctx context.Context, realmName, alias string) (json.RawMessage, error) {
	return c.GetRaw(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/identity-provider/instances/"+url.PathEscape(alias))
}

// GetRealmRoles gets all realm roles
func (c *Client) GetRealmRoles(ctx context.Context, realmName string) ([]RoleRepresentation, error) {
	return listAll[RoleRepresentation](ctx, c, "/admin/realms/"+url.PathEscape(realmName)+"/roles", nil)
}

// GetRealmRolesRaw gets all realm roles as raw JSON
func (c *Client) GetRealmRolesRaw(ctx context.Context, realmName string) ([]json.RawMessage, error) {
	return c.ListRaw(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/roles", nil)
}

// GetRealmRoleRaw gets a realm role by name as raw JSON
func (c *Client) GetRealmRoleRaw(ctx context.Context, realmName, roleName string) (json.RawMessage, error) {
	return c.GetRaw(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/roles/"+url.PathEscape(roleName))
}

// GetClientRoles gets all roles for a client
func (c *Client) GetClientRoles(ctx context.Context, realmName, clientUUID string) ([]RoleRepresentation, error) {
	var roles []RoleRepresentation
	if err := c.List(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/clients/"+url.PathEscape(clientUUID)+"/roles", nil, &roles); err != nil {
		return nil, err
	}
	return roles, nil
}

// GetClientRolesRaw gets all roles for a client as raw JSON
func (c *Client) GetClientRolesRaw(ctx context.Context, realmName, clientUUID string) ([]json.RawMessage, error) {
	return c.ListRaw(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/clients/"+url.PathEscape(clientUUID)+"/roles", nil)
}

// GetClientRoleRaw gets a client role by name as raw JSON
func (c *Client) GetClientRoleRaw(ctx context.Context, realmName, clientUUID, roleName string) (json.RawMessage, error) {
	return c.GetRaw(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/clients/"+url.PathEscape(clientUUID)+"/roles/"+url.PathEscape(roleName))
}

// GetUserRealmRoleMappings gets realm role mappings for a user
func (c *Client) GetUserRealmRoleMappings(ctx context.Context, realmName, userID string) ([]RoleRepresentation, error) {
	var roles []RoleRepresentation
	if err := c.List(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/users/"+url.PathEscape(userID)+"/role-mappings/realm", nil, &roles); err != nil {
		return nil, err
	}
	return roles, nil
}

// GetUserRealmRoleMappingsRaw gets realm role mappings for a user as raw JSON
func (c *Client) GetUserRealmRoleMappingsRaw(ctx context.Context, realmName, userID string) ([]json.RawMessage, error) {
	return c.ListRaw(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/users/"+url.PathEscape(userID)+"/role-mappings/realm", nil)
}

// GetUserClientRoleMappings gets client role mappings for a user
func (c *Client) GetUserClientRoleMappings(ctx context.Context, realmName, userID, clientUUID string) ([]RoleRepresentation, error) {
	var roles []RoleRepresentation
	if err := c.List(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/users/"+url.PathEscape(userID)+"/role-mappings/clients/"+url.PathEscape(clientUUID), nil, &roles); err != nil {
		return nil, err
	}
	return roles, nil
}

// GetUserClientRoleMappingsRaw gets client role mappings for a user as raw JSON
func (c *Client) GetUserClientRoleMappingsRaw(ctx context.Context, realmName, userID, clientUUID string) ([]json.RawMessage, error) {
	return c.ListRaw(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/users/"+url.PathEscape(userID)+"/role-mappings/clients/"+url.PathEscape(clientUUID), nil)
}

// GetGroupRealmRoleMappings gets realm role mappings for a group
func (c *Client) GetGroupRealmRoleMappings(ctx context.Context, realmName, groupID string) ([]RoleRepresentation, error) {
	var roles []RoleRepresentation
	if err := c.List(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/groups/"+url.PathEscape(groupID)+"/role-mappings/realm", nil, &roles); err != nil {
		return nil, err
	}
	return roles, nil
}

// GetGroupRealmRoleMappingsRaw gets realm role mappings for a group as raw JSON
func (c *Client) GetGroupRealmRoleMappingsRaw(ctx context.Context, realmName, groupID string) ([]json.RawMessage, error) {
	return c.ListRaw(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/groups/"+url.PathEscape(groupID)+"/role-mappings/realm", nil)
}

// GetGroupClientRoleMappings gets client role mappings for a group
func (c *Client) GetGroupClientRoleMappings(ctx context.Context, realmName, groupID, clientUUID string) ([]RoleRepresentation, error) {
	var roles []RoleRepresentation
	if err := c.List(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/groups/"+url.PathEscape(groupID)+"/role-mappings/clients/"+url.PathEscape(clientUUID), nil, &roles); err != nil {
		return nil, err
	}
	return roles, nil
}

// GetGroupClientRoleMappingsRaw gets client role mappings for a group as raw JSON
func (c *Client) GetGroupClientRoleMappingsRaw(ctx context.Context, realmName, groupID, clientUUID string) ([]json.RawMessage, error) {
	return c.ListRaw(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/groups/"+url.PathEscape(groupID)+"/role-mappings/clients/"+url.PathEscape(clientUUID), nil)
}

// GetComponentsRaw gets all components in a realm as raw JSON
func (c *Client) GetComponentsRaw(ctx context.Context, realmName string, params map[string]string) ([]json.RawMessage, error) {
	return c.ListRaw(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/components", params)
}

// GetComponentRaw gets a component by ID as raw JSON
func (c *Client) GetComponentRaw(ctx context.Context, realmName, componentID string) (json.RawMessage, error) {
	return c.GetRaw(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/components/"+url.PathEscape(componentID))
}

// GetOrganizationsRaw gets all organizations in a realm as raw JSON
func (c *Client) GetOrganizationsRaw(ctx context.Context, realmName string) ([]json.RawMessage, error) {
	return c.ListRaw(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/organizations", nil)
}

// GetOrganizationRaw gets an organization by ID as raw JSON
func (c *Client) GetOrganizationRaw(ctx context.Context, realmName, orgID string) (json.RawMessage, error) {
	return c.GetRaw(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/organizations/"+url.PathEscape(orgID))
}

// GetClientProtocolMappersRaw gets all protocol mappers for a client as raw JSON
func (c *Client) GetClientProtocolMappersRaw(ctx context.Context, realmName, clientUUID string) ([]json.RawMessage, error) {
	return c.ListRaw(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/clients/"+url.PathEscape(clientUUID)+"/protocol-mappers/models", nil)
}

// GetClientScopeProtocolMappersRaw gets all protocol mappers for a client scope as raw JSON
func (c *Client) GetClientScopeProtocolMappersRaw(ctx context.Context, realmName, scopeID string) ([]json.RawMessage, error) {
	return c.ListRaw(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/client-scopes/"+url.PathEscape(scopeID)+"/protocol-mappers/models", nil)
}

// ============================================================================
// Client Manager
// ============================================================================

// ClientManager handles Keycloak client lifecycle and rate limiting
type ClientManager struct {
	clients   sync.Map // map[string]*Client - key is instance name
	log       logr.Logger
	semaphore chan struct{}
}

// ClientManagerConfig holds configuration for the ClientManager
type ClientManagerConfig struct {
	// MaxConcurrentRequests limits the number of concurrent requests to Keycloak.
	// This prevents overwhelming Keycloak when reconciling many resources.
	// Default: 10 (0 means no limit)
	MaxConcurrentRequests int
}

// DefaultClientManagerConfig returns default client manager configuration
func DefaultClientManagerConfig() ClientManagerConfig {
	return ClientManagerConfig{
		MaxConcurrentRequests: 10,
	}
}

// NewClientManager creates a new client manager with default configuration
func NewClientManager(log logr.Logger) *ClientManager {
	return NewClientManagerWithConfig(log, DefaultClientManagerConfig())
}

// NewClientManagerWithConfig creates a new client manager with custom configuration
func NewClientManagerWithConfig(log logr.Logger, cfg ClientManagerConfig) *ClientManager {
	var sem chan struct{}
	if cfg.MaxConcurrentRequests > 0 {
		sem = make(chan struct{}, cfg.MaxConcurrentRequests)
	}
	return &ClientManager{
		log:       log.WithName("keycloak-manager"),
		semaphore: sem,
	}
}

// AcquireSlot acquires a rate-limiting slot. The returned function must be called to release the slot.
// If rate limiting is not configured, returns a no-op function immediately.
func (m *ClientManager) AcquireSlot(ctx context.Context) (release func(), err error) {
	if m.semaphore == nil {
		// No rate limiting configured
		return func() {}, nil
	}

	select {
	case m.semaphore <- struct{}{}:
		return func() {
			<-m.semaphore
		}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// GetOrCreateClient gets or creates a Keycloak client for an instance
func (m *ClientManager) GetOrCreateClient(instanceName string, cfg Config) *Client {
	if existing, ok := m.clients.Load(instanceName); ok {
		client := existing.(*Client)
		// If the config has changed, recreate the client
		if m.configChanged(client, cfg) {
			client = NewClient(cfg, m.log)
			m.clients.Store(instanceName, client)
		}
		return client
	}

	client := NewClient(cfg, m.log)
	m.clients.Store(instanceName, client)
	return client
}

// configChanged checks if the config has changed from what's in the existing client
func (m *ClientManager) configChanged(client *Client, cfg Config) bool {
	cfg = normalizeConfig(cfg)
	return client.baseURL != cfg.BaseURL ||
		client.username != cfg.Username ||
		client.password != cfg.Password ||
		client.realm != cfg.Realm ||
		client.clientID != cfg.ClientID ||
		client.clientSecret != cfg.ClientSecret ||
		client.caCert != cfg.CACert ||
		client.insecureSkipVerify != cfg.InsecureSkipVerify
}

// RemoveClient removes a client from the manager
func (m *ClientManager) RemoveClient(instanceName string) {
	m.clients.Delete(instanceName)
}

// ClearClients removes all clients
func (m *ClientManager) ClearClients() {
	m.clients.Range(func(key, value interface{}) bool {
		m.clients.Delete(key)
		return true
	})
}

// ============================================================================
// Retry Logic
// ============================================================================

// RetryConfig holds retry configuration
type RetryConfig struct {
	MaxRetries    int
	InitialDelay  time.Duration
	MaxDelay      time.Duration
	BackoffFactor float64
	RetryableFunc func(error) bool
}

// DefaultRetryConfig returns default retry configuration
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:    3,
		InitialDelay:  1 * time.Second,
		MaxDelay:      30 * time.Second,
		BackoffFactor: 2.0,
		RetryableFunc: isRetryableError,
	}
}

// isRetryableError determines if an error is retryable
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()

	// Network and connection errors
	if strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "connection reset") ||
		strings.Contains(errStr, "no such host") ||
		strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "temporary failure") ||
		strings.Contains(errStr, "EOF") {
		return true
	}

	// HTTP 5xx errors (server errors)
	if strings.Contains(errStr, "500") ||
		strings.Contains(errStr, "502") ||
		strings.Contains(errStr, "503") ||
		strings.Contains(errStr, "504") {
		return true
	}

	// Token/auth errors that might be resolved by re-auth
	if strings.Contains(errStr, "401") ||
		strings.Contains(errStr, "token") ||
		strings.Contains(errStr, "unauthorized") {
		return true
	}

	// Rate limiting
	if strings.Contains(errStr, "429") ||
		strings.Contains(errStr, "rate limit") {
		return true
	}

	return false
}

// WithRetry executes a function with exponential backoff retry
func WithRetry[T any](ctx context.Context, cfg RetryConfig, operation string, fn func() (T, error)) (T, error) {
	var result T
	var lastErr error
	delay := cfg.InitialDelay

	for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
		result, lastErr = fn()
		if lastErr == nil {
			return result, nil
		}

		// Check if error is retryable
		if cfg.RetryableFunc != nil && !cfg.RetryableFunc(lastErr) {
			return result, lastErr
		}

		// Check if we've exhausted retries
		if attempt >= cfg.MaxRetries {
			return result, fmt.Errorf("%s failed after %d attempts: %w", operation, attempt+1, lastErr)
		}

		// Wait before retry
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		case <-time.After(delay):
		}

		// Increase delay with exponential backoff
		delay = time.Duration(float64(delay) * cfg.BackoffFactor)
		if delay > cfg.MaxDelay {
			delay = cfg.MaxDelay
		}
	}

	return result, lastErr
}

// WithRetryVoid executes a void function with exponential backoff retry
func WithRetryVoid(ctx context.Context, cfg RetryConfig, operation string, fn func() error) error {
	_, err := WithRetry(ctx, cfg, operation, func() (struct{}, error) {
		return struct{}{}, fn()
	})
	return err
}
