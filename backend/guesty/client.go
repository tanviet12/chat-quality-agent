package guesty

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sync"
	"time"
)

const (
	guestyOAuthURL     = "https://open-api.guesty.com/oauth2/token"
	guestyAPIBaseURL   = "https://open-api.guesty.com/v1"
	tokenRefreshBuffer = 5 * time.Minute // Refresh 5 minutes before expiry
)

// Credentials holds Guesty API credentials
type Credentials struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

// TokenResponse represents the OAuth token response
type TokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
	Scope       string `json:"scope"`
}

// Client handles Guesty API communication with automatic token refresh
type Client struct {
	credentials    Credentials
	httpClient     *http.Client
	mu             sync.RWMutex
	token          string
	tokenExpiresAt time.Time
}

// NewClient creates a new Guesty API client
func NewClient(clientID, clientSecret string) *Client {
	return &Client{
		credentials: Credentials{
			ClientID:     clientID,
			ClientSecret: clientSecret,
		},
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// GetToken returns a valid access token, refreshing if necessary
func (c *Client) GetToken(ctx context.Context) (string, error) {
	// Check if we have a valid token
	c.mu.RLock()
	if c.token != nil && c.token != "" && time.Now().Add(tokenRefreshBuffer).Before(c.tokenExpiresAt) {
		token := c.token
		c.mu.RUnlock()
		return token, nil
	}
	c.mu.RUnlock()

	// Need to refresh token
	return c.refreshToken(ctx)
}

// refreshToken obtains a new access token
func (c *Client) refreshToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check after acquiring write lock
	if c.token != "" && time.Now().Add(tokenRefreshBuffer).Before(c.tokenExpiresAt) {
		return c.token, nil
	}

	formData := url.Values{}
	formData.Set("grant_type", "client_credentials")
	formData.Set("scope", "open-api")
	formData.Set("client_id", c.credentials.ClientID)
	formData.Set("client_secret", c.credentials.ClientSecret)

	req, err := http.NewRequestWithContext(ctx, "POST", guestyOAuthURL, nil)
	if err != nil {
		return "", fmt.Errorf("create token request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.URL.RawQuery = formData.Encode()

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", fmt.Errorf("parse token response: %w", err)
	}

	// Update cached token
	c.token = tokenResp.AccessToken
	c.tokenExpiresAt = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)

	log.Printf("[Guesty] Token refreshed, expires at: %s", c.tokenExpiresAt.Format(time.RFC3339))

	return c.token, nil
}

// Do performs an authenticated API request with automatic retry on token expiry
func (c *Client) Do(ctx context.Context, method, url string, body io.Reader) (*http.Response, error) {
	token, err := c.GetToken(ctx)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}

	// If we get a 403, try once more with a fresh token
	if resp.StatusCode == http.StatusForbidden {
		resp.Body.Close()
		log.Printf("[Guesty] Got 403, refreshing token and retrying")

		// Force token refresh
		c.mu.Lock()
		c.token = ""
		c.tokenExpiresAt = time.Time{}
		c.mu.Unlock()

		token, err = c.refreshToken(ctx)
		if err != nil {
			return nil, err
		}

		req, err = http.NewRequestWithContext(ctx, method, url, body)
		if err != nil {
			return nil, fmt.Errorf("create retry request: %w", err)
		}

		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Accept", "application/json")
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		resp, err = c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("do retry request: %w", err)
		}
	}

	return resp, nil
}

// Get performs a GET request
func (c *Client) Get(ctx context.Context, path string) (*http.Response, error) {
	return c.Do(ctx, "GET", guestyAPIBaseURL+path, nil)
}

// Post performs a POST request
func (c *Client) Post(ctx context.Context, path string, body io.Reader) (*http.Response, error) {
	return c.Do(ctx, "POST", guestyAPIBaseURL+path, body)
}
