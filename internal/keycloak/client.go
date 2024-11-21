// Package keycloak provides a client for interacting with the Keycloak Admin REST API.
package keycloak

import (
	"context"
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
	if cfg.Realm == "" {
		cfg.Realm = "master"
	}

	httpClient := resty.New().
		SetTimeout(30 * time.Second).
		SetRetryCount(0)

	return &Client{
		baseURL:      strings.TrimSuffix(cfg.BaseURL, "/"),
		realm:        cfg.Realm,
		username:     cfg.Username,
		password:     cfg.Password,
		clientID:     cfg.ClientID,
		clientSecret: cfg.ClientSecret,
		httpClient:   httpClient,
		log:          log.WithName("keycloak-client"),
	}
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

// ============================================================================
// Realm Operations
// ============================================================================

// RealmRepresentation represents a Keycloak realm
type RealmRepresentation struct {
	ID          *string `json:"id,omitempty"`
	Realm       *string `json:"realm,omitempty"`
	Enabled     *bool   `json:"enabled,omitempty"`
	DisplayName *string `json:"displayName,omitempty"`
}

// CreateRealmFromDefinition creates a realm from raw JSON definition
func (c *Client) CreateRealmFromDefinition(ctx context.Context, definition json.RawMessage) error {
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
	return c.Update(ctx, "/admin/realms/"+url.PathEscape(realmName), definition)
}

// DeleteRealm deletes a realm
func (c *Client) DeleteRealm(ctx context.Context, realmName string) error {
	return c.Delete(ctx, "/admin/realms/"+url.PathEscape(realmName))
}

// ============================================================================
// Client Operations
// ============================================================================

// ClientRepresentation represents a Keycloak client
type ClientRepresentation struct {
	ID       *string `json:"id,omitempty"`
	ClientID *string `json:"clientId,omitempty"`
	Name     *string `json:"name,omitempty"`
	Enabled  *bool   `json:"enabled,omitempty"`
}

// CreateClient creates a new client
func (c *Client) CreateClient(ctx context.Context, realmName string, clientDef json.RawMessage) (string, error) {
	return c.Create(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/clients", clientDef)
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
	var clients []ClientRepresentation
	req, err := c.request(ctx)
	if err != nil {
		return nil, err
	}
	if params != nil {
		req.SetQueryParams(params)
	}
	resp, err := req.SetResult(&clients).Get(c.baseURL + "/admin/realms/" + url.PathEscape(realmName) + "/clients")
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	if resp.IsError() {
		return nil, fmt.Errorf("%s: %s", resp.Status(), string(resp.Body()))
	}
	return clients, nil
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
	return c.Update(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/clients/"+url.PathEscape(clientID), clientDef)
}

// DeleteClient deletes a client
func (c *Client) DeleteClient(ctx context.Context, realmName, clientID string) error {
	return c.Delete(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/clients/"+url.PathEscape(clientID))
}

// ============================================================================
// User Operations
// ============================================================================

// UserRepresentation represents a Keycloak user
type UserRepresentation struct {
	ID        *string `json:"id,omitempty"`
	Username  *string `json:"username,omitempty"`
	Email     *string `json:"email,omitempty"`
	Enabled   *bool   `json:"enabled,omitempty"`
	FirstName *string `json:"firstName,omitempty"`
	LastName  *string `json:"lastName,omitempty"`
}

// CreateUser creates a new user
func (c *Client) CreateUser(ctx context.Context, realmName string, userDef json.RawMessage) (string, error) {
	return c.Create(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/users", userDef)
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
	var users []UserRepresentation
	req, err := c.request(ctx)
	if err != nil {
		return nil, err
	}
	if params != nil {
		req.SetQueryParams(params)
	}
	resp, err := req.SetResult(&users).Get(c.baseURL + "/admin/realms/" + url.PathEscape(realmName) + "/users")
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	if resp.IsError() {
		return nil, fmt.Errorf("%s: %s", resp.Status(), string(resp.Body()))
	}
	return users, nil
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
	return c.Update(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/users/"+url.PathEscape(userID), userDef)
}

// DeleteUser deletes a user
func (c *Client) DeleteUser(ctx context.Context, realmName, userID string) error {
	return c.Delete(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/users/"+url.PathEscape(userID))
}

// ============================================================================
// Role Operations
// ============================================================================

// RoleRepresentation represents a Keycloak role
type RoleRepresentation struct {
	ID          *string `json:"id,omitempty"`
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
	Composite   *bool   `json:"composite,omitempty"`
}

// CreateRealmRole creates a realm-level role
func (c *Client) CreateRealmRole(ctx context.Context, realmName string, roleDef json.RawMessage) error {
	req, err := c.request(ctx)
	if err != nil {
		return err
	}
	resp, err := req.SetBody(roleDef).Post(c.baseURL + "/admin/realms/" + url.PathEscape(realmName) + "/roles")
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	if resp.IsError() {
		return fmt.Errorf("%s: %s", resp.Status(), string(resp.Body()))
	}
	return nil
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

// ============================================================================
// Group Operations
// ============================================================================

// GroupRepresentation represents a Keycloak group
type GroupRepresentation struct {
	ID   *string `json:"id,omitempty"`
	Name *string `json:"name,omitempty"`
	Path *string `json:"path,omitempty"`
}

// CreateGroup creates a group
func (c *Client) CreateGroup(ctx context.Context, realmName string, groupDef json.RawMessage) (string, error) {
	return c.Create(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/groups", groupDef)
}

// GetGroup gets a group by ID
func (c *Client) GetGroup(ctx context.Context, realmName, groupID string) (*GroupRepresentation, error) {
	var group GroupRepresentation
	if err := c.Get(ctx, "/admin/realms/"+url.PathEscape(realmName)+"/groups/"+url.PathEscape(groupID), &group); err != nil {
		return nil, err
	}
	return &group, nil
}

// GetGroups gets all groups
func (c *Client) GetGroups(ctx context.Context, realmName string, params map[string]string) ([]GroupRepresentation, error) {
	var groups []GroupRepresentation
	req, err := c.request(ctx)
	if err != nil {
		return nil, err
	}
	if params != nil {
		req.SetQueryParams(params)
	}
	resp, err := req.SetResult(&groups).Get(c.baseURL + "/admin/realms/" + url.PathEscape(realmName) + "/groups")
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	if resp.IsError() {
		return nil, fmt.Errorf("%s: %s", resp.Status(), string(resp.Body()))
	}
	return groups, nil
}

// GetGroupByName finds a group by name
func (c *Client) GetGroupByName(ctx context.Context, realmName, name string) (*GroupRepresentation, error) {
	groups, err := c.GetGroups(ctx, realmName, map[string]string{"search": name})
	if err != nil {
		return nil, err
	}
	for _, g := range groups {
		if g.Name != nil && *g.Name == name {
			return &g, nil
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
