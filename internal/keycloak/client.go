// Package keycloak provides a client for interacting with the Keycloak Admin REST API.
package keycloak

import (
	"context"
	"fmt"
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
