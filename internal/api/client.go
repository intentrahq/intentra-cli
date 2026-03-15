// Package api provides HTTP client functionality for communicating with the
// Intentra server. It supports JWT authentication (via intentra login) and
// API key authentication (Enterprise).
package api

import (
	"bytes"
	"compress/gzip"
	"crypto/hmac"
	cryptoRand "crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"time"

	"github.com/atbabers/intentra-cli/internal/auth"
	"github.com/atbabers/intentra-cli/internal/config"
	"github.com/atbabers/intentra-cli/internal/debug"
	"github.com/atbabers/intentra-cli/internal/device"
	"github.com/atbabers/intentra-cli/internal/httputil"
	"github.com/atbabers/intentra-cli/pkg/models"
)

func gzipCompress(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write(data); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// UserAgent is the User-Agent header value sent with all API requests.
const UserAgent = "intentra-cli/1.0"

// ScansResponse represents the response from GET /scans.
type ScansResponse struct {
	Scans   []models.Scan `json:"scans"`
	Summary ScansSummary  `json:"summary"`
}

// ScansSummary contains aggregated scan statistics.
type ScansSummary struct {
	TotalScans          int     `json:"total_scans"`
	TotalCost           float64 `json:"total_cost"`
	ScansWithViolations int     `json:"scans_with_violations"`
}

// ScanDetailResponse represents the response from GET /scans/{id}.
type ScanDetailResponse struct {
	Scan             models.Scan       `json:"scan"`
	ViolationDetails map[string]string `json:"violation_details,omitempty"`
}

// Client handles communication with the Intentra API.
type Client struct {
	cfg        *config.Config
	httpClient *http.Client
}

// NewClient creates a new API client configured with the provided settings.
func NewClient(cfg *config.Config) (*Client, error) {
	if !cfg.Server.Enabled {
		return nil, fmt.Errorf("server sync is not enabled")
	}

	httpClient := &http.Client{
		Timeout: cfg.Server.Timeout,
	}

	return &Client{
		cfg:        cfg,
		httpClient: httpClient,
	}, nil
}

// SendScan sends a single scan to the API with gzip compression.
func (c *Client) SendScan(scan *models.Scan) error {
	deviceID, err := device.GetDeviceID()
	if err != nil {
		return fmt.Errorf("failed to get device ID: %w", err)
	}

	jsonBody, err := json.Marshal(scan.BuildAPIPayload(deviceID))
	if err != nil {
		return fmt.Errorf("failed to marshal scan: %w", err)
	}

	compressed, err := gzipCompress(jsonBody)
	if err != nil {
		return fmt.Errorf("failed to compress scan: %w", err)
	}

	url := c.cfg.Server.Endpoint + "/scans"
	req, err := http.NewRequest("POST", url, bytes.NewReader(compressed))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Encoding", "gzip")
	req.Header.Set("User-Agent", UserAgent)

	if err := c.addAuth(req); err != nil {
		return fmt.Errorf("failed to add auth: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		debug.LogHTTP("POST", url, 0)
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	debug.LogHTTP("POST", url, resp.StatusCode)

	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, readErr := io.ReadAll(io.LimitReader(resp.Body, httputil.MaxResponseSize))
		if readErr != nil {
			return fmt.Errorf("API returned %d (failed to read body: %w)", resp.StatusCode, readErr)
		}
		return fmt.Errorf("API returned %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// SendScans sends a batch of scans to the API by calling SendScan for each.
func (c *Client) SendScans(scans []*models.Scan) error {
	for _, scan := range scans {
		if err := c.SendScan(scan); err != nil {
			return err
		}
	}
	return nil
}

// addAuth adds authentication headers based on config.
// Priority: JWT credentials (from 'intentra login') > config auth mode (api_key)
func (c *Client) addAuth(req *http.Request) error {
	creds, err := auth.GetValidCredentials()
	if err != nil {
		debug.Warn("credential check failed: %v", err)
	}
	if creds != nil {
		return c.addJWTAuthWithCreds(req, creds)
	}

	switch c.cfg.Server.Auth.Mode {
	case config.AuthModeAPIKey:
		return c.addAPIKeyAuth(req)
	default:
		return fmt.Errorf("not authenticated - run 'intentra login' or configure api_key auth in config.yaml")
	}
}

// addAPIKeyAuth adds API key authentication headers for Enterprise organizations.
// When hmac_key is configured, signs the request with HMAC-SHA256 so the raw
// secret never leaves the client. Falls back to legacy bcrypt mode when only
// secret is configured (for keys created before HMAC support).
func (c *Client) addAPIKeyAuth(req *http.Request) error {
	if req.URL.Scheme != "https" {
		return fmt.Errorf("API key auth requires HTTPS; refusing to send credentials over HTTP")
	}

	keyID := c.cfg.Server.Auth.APIKey.KeyID
	hmacKey := c.cfg.Server.Auth.APIKey.HMACKey
	secret := c.cfg.Server.Auth.APIKey.Secret

	if keyID == "" {
		return fmt.Errorf("API key auth requires key_id")
	}
	if hmacKey == "" && secret == "" {
		return fmt.Errorf("API key auth requires hmac_key (preferred) or secret")
	}

	timestamp := time.Now().UTC().Format(time.RFC3339)

	nonceBytes := make([]byte, 16)
	if _, err := io.ReadFull(cryptoRand.Reader, nonceBytes); err != nil {
		return fmt.Errorf("failed to generate nonce: %w", err)
	}
	nonce := hex.EncodeToString(nonceBytes)

	req.Header.Set("X-API-Key-ID", keyID)
	req.Header.Set("X-API-Timestamp", timestamp)
	req.Header.Set("X-API-Nonce", nonce)

	if hmacKey != "" {
		message := fmt.Sprintf("%s\n%s\n%s\n%s", req.Method, req.URL.Path, timestamp, nonce)
		mac := hmac.New(sha256.New, []byte(hmacKey))
		mac.Write([]byte(message))
		signature := hex.EncodeToString(mac.Sum(nil))
		req.Header.Set("X-API-Key-Signature", signature)
	} else {
		req.Header.Set("X-API-Key-Secret", secret)
	}

	return nil
}

// addJWTAuth adds JWT Bearer token authentication from stored credentials.
// Loads credentials internally; use addJWTAuthWithCreds when creds are already loaded.
func (c *Client) addJWTAuth(req *http.Request) error {
	creds, err := auth.GetValidCredentials()
	if err != nil {
		return fmt.Errorf("credential retrieval failed: %w", err)
	}
	if creds == nil {
		return fmt.Errorf("not authenticated - run 'intentra login' first")
	}
	return c.addJWTAuthWithCreds(req, creds)
}

// addJWTAuthWithCreds adds JWT auth headers using pre-loaded credentials.
func (c *Client) addJWTAuthWithCreds(req *http.Request, creds *auth.Credentials) error {
	req.Header.Set("Authorization", "Bearer "+creds.AccessToken)

	deviceID, err := device.GetDeviceID()
	if err != nil {
		return fmt.Errorf("failed to get device ID: %w", err)
	}
	req.Header.Set("X-Machine-ID", deviceID)

	return nil
}

// doJWTRequest executes an authenticated JSON request against the default API endpoint.
func doJWTRequest(method, path, accessToken string, body []byte, acceptedStatuses ...int) error {
	deviceID, err := device.GetDeviceID()
	if err != nil {
		return fmt.Errorf("failed to get device ID: %w", err)
	}

	compressed, err := gzipCompress(body)
	if err != nil {
		return fmt.Errorf("failed to compress request body: %w", err)
	}

	reqURL := config.DefaultAPIEndpoint + path
	req, err := http.NewRequest(method, reqURL, bytes.NewReader(compressed))
	if err != nil {
		return fmt.Errorf("failed to create %s request: %w", method, err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Encoding", "gzip")
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("X-Machine-ID", deviceID)

	resp, err := httputil.DefaultClient.Do(req)
	if err != nil {
		debug.LogHTTP(method, reqURL, 0)
		return fmt.Errorf("%s request failed: %w", method, err)
	}
	defer resp.Body.Close()
	debug.LogHTTP(method, reqURL, resp.StatusCode)

	if slices.Contains(acceptedStatuses, resp.StatusCode) {
		return nil
	}
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, httputil.MaxResponseSize))
	return fmt.Errorf("%s returned %d: %s", method, resp.StatusCode, string(respBody))
}

// SendScanWithJWT sends a scan to the default API endpoint using JWT auth.
func SendScanWithJWT(scan *models.Scan, accessToken string) error {
	deviceID, err := device.GetDeviceID()
	if err != nil {
		return fmt.Errorf("failed to get device ID: %w", err)
	}

	jsonBody, err := json.Marshal(scan.BuildAPIPayload(deviceID))
	if err != nil {
		return fmt.Errorf("failed to marshal scan: %w", err)
	}

	return doJWTRequest("POST", "/scans", accessToken, jsonBody,
		http.StatusAccepted, http.StatusOK, http.StatusCreated)
}

// PatchSessionEnd sends a PATCH to update session-end metadata on a scan.
func PatchSessionEnd(scanID, accessToken, reason string, durationMs int64) error {
	body := map[string]any{}
	if reason != "" {
		body["session_end_reason"] = reason
	}
	if durationMs > 0 {
		body["session_duration_ms"] = durationMs
	}

	if len(body) == 0 {
		return nil
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("failed to marshal session end body: %w", err)
	}

	return doJWTRequest("PATCH", "/scans/"+url.PathEscape(scanID)+"/session", accessToken, jsonBody,
		http.StatusOK, http.StatusNoContent)
}

// GetScans retrieves scans from the API.
func (c *Client) GetScans(days, limit int) (*ScansResponse, error) {
	if days <= 0 {
		days = 30
	}
	if limit <= 0 {
		limit = 50
	}

	url := fmt.Sprintf("%s/scans?days=%d&limit=%d", c.cfg.Server.Endpoint, days, limit)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", UserAgent)

	if err := c.addJWTAuth(req); err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		debug.LogHTTP("GET", url, 0)
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	debug.LogHTTP("GET", url, resp.StatusCode)

	body, err := io.ReadAll(io.LimitReader(resp.Body, httputil.MaxResponseSize))
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("authentication failed - run 'intentra login' to re-authenticate")
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned %d: %s", resp.StatusCode, string(body))
	}

	var result ScansResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &result, nil
}

// GetScan retrieves a single scan by ID from the API.
func (c *Client) GetScan(scanID string) (*ScanDetailResponse, error) {
	if scanID == "" {
		return nil, fmt.Errorf("scan ID is required")
	}

	url := fmt.Sprintf("%s/scans/%s", c.cfg.Server.Endpoint, url.PathEscape(scanID))

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", UserAgent)

	if err := c.addJWTAuth(req); err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		debug.LogHTTP("GET", url, 0)
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	debug.LogHTTP("GET", url, resp.StatusCode)

	body, err := io.ReadAll(io.LimitReader(resp.Body, httputil.MaxResponseSize))
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("authentication failed - run 'intentra login' to re-authenticate")
	}

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("scan not found: %s", scanID)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned %d: %s", resp.StatusCode, string(body))
	}

	var result ScanDetailResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &result, nil
}
