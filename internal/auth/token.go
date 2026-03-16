// Package auth provides authentication and token management for the Intentra CLI.
// It handles OAuth 2.0 device flow authentication with Auth0, secure token storage,
// and automatic token refresh.
package auth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/intentrahq/intentra-cli/internal/config"
	"github.com/intentrahq/intentra-cli/internal/httputil"
)

// Credentials represents stored authentication credentials.
type Credentials struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	IDToken      string    `json:"id_token,omitempty"`
	TokenType    string    `json:"token_type"`
	ExpiresAt    time.Time `json:"expires_at"`
	UserID       string    `json:"user_id,omitempty"`
	Email        string    `json:"email,omitempty"`
}

// DeviceCodeResponse represents the response from the device code endpoint.
type DeviceCodeResponse struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

// TokenResponse represents the response from the token endpoint.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	Error        string `json:"error,omitempty"`
	ErrorDesc    string `json:"error_description,omitempty"`
}

// IsExpired returns true if the credentials have expired or will expire within the buffer period.
func (c *Credentials) IsExpired() bool {
	buffer := 5 * time.Minute
	return time.Now().Add(buffer).After(c.ExpiresAt)
}

// IsValid returns true if credentials exist and are not expired.
func (c *Credentials) IsValid() bool {
	return c.AccessToken != "" && !c.IsExpired()
}


// CredentialsFromTokenResponse creates Credentials from a TokenResponse.
func CredentialsFromTokenResponse(resp *TokenResponse) *Credentials {
	expiresAt := time.Now().Add(time.Duration(resp.ExpiresIn) * time.Second)

	return &Credentials{
		AccessToken:  resp.AccessToken,
		RefreshToken: resp.RefreshToken,
		IDToken:      resp.IDToken,
		TokenType:    resp.TokenType,
		ExpiresAt:    expiresAt,
	}
}

// GetValidCredentials loads credentials from secure storage, refreshes if needed, and returns them if valid.
// Returns (nil, nil) when the user is simply not logged in, and (nil, err) on system failures.
func GetValidCredentials() (*Credentials, error) {
	creds, err := LoadCredentialsFromKeyring()
	if err != nil {
		return nil, fmt.Errorf("failed to load credentials: %w", err)
	}
	if creds == nil {
		return nil, nil
	}

	if creds.IsValid() {
		return creds, nil
	}

	if creds.RefreshToken == "" {
		return nil, nil
	}

	refreshed, err := RefreshCredentials(creds)
	if err != nil {
		return nil, fmt.Errorf("failed to refresh credentials: %w", err)
	}

	return refreshed, nil
}

// RefreshCredentials uses the refresh token to obtain new credentials.
func RefreshCredentials(creds *Credentials) (*Credentials, error) {
	if creds.RefreshToken == "" {
		return nil, fmt.Errorf("no refresh token available")
	}

	if creds.RefreshToken == "env-token-no-refresh" {
		return creds, nil
	}

	// Quick unlocked check — if someone already refreshed, use that
	currentCreds, _ := LoadCredentialsFromKeyring()
	if currentCreds != nil && currentCreds.IsValid() && currentCreds.AccessToken != creds.AccessToken {
		return currentCreds, nil
	}

	// Perform refresh (outside lock — HTTP call can be slow)
	newCreds, err := doRefreshHTTP(creds)
	if err != nil {
		return nil, err
	}

	// Store under lock with re-check
	err = WithCredentialLock(func() error {
		latestCreds, _ := LoadCredentialsFromKeyring()
		if latestCreds != nil && latestCreds.IsValid() && latestCreds.AccessToken != creds.AccessToken {
			// Another process already refreshed — use their token
			newCreds = latestCreds
			return nil
		}

		return storeCredentialsInKeyringUnlocked(newCreds)
	})

	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: credential lock failed, falling back to encrypted cache: %v\n", err)
		if writeErr := WriteEncryptedCache(newCreds); writeErr != nil {
			return nil, fmt.Errorf("failed to save refreshed credentials: %w", writeErr)
		}
	}

	return newCreds, nil
}

// doRefreshHTTP performs the HTTP refresh token exchange.
func doRefreshHTTP(creds *Credentials) (*Credentials, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	endpoint := cfg.Server.Endpoint
	if endpoint == "" {
		endpoint = config.DefaultAPIEndpoint
	}

	url := endpoint + "/oauth/refresh"
	payload := map[string]string{
		"refresh_token": creds.RefreshToken,
	}
	payloadBytes, _ := json.Marshal(payload)

	resp, err := httputil.DefaultClient.Post(url, "application/json", bytes.NewReader(payloadBytes))
	if err != nil {
		return nil, fmt.Errorf("refresh request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, httputil.MaxResponseSize))
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("refresh failed (%d): %s", resp.StatusCode, string(body))
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if tokenResp.AccessToken == "" {
		return nil, fmt.Errorf("no access token in response")
	}

	newCreds := CredentialsFromTokenResponse(&tokenResp)
	newCreds.UserID = creds.UserID
	newCreds.Email = creds.Email

	if newCreds.RefreshToken == "" {
		newCreds.RefreshToken = creds.RefreshToken
	}

	return newCreds, nil
}
