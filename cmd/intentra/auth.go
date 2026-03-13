package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"time"
	"unicode"

	"github.com/atbabers/intentra-cli/internal/auth"
	"github.com/atbabers/intentra-cli/internal/config"
	"github.com/atbabers/intentra-cli/internal/debug"
	"github.com/atbabers/intentra-cli/internal/device"
	"github.com/atbabers/intentra-cli/internal/httputil"
	"github.com/atbabers/intentra-cli/internal/queue"
	"github.com/spf13/cobra"
)


func newLoginCmd() *cobra.Command {
	var noBrowser, force bool

	cmd := &cobra.Command{
		Use:           "login",
		Short:         "Authenticate with Intentra",
		SilenceUsage:  true,
		SilenceErrors: true,
		Long: `Authenticate with your Intentra account using device authorization flow.

This will:
1. Generate a device code
2. Open your browser to authorize (or display URL if --no-browser)
3. Poll for authorization and save credentials`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLogin(noBrowser, force)
		},
	}

	cmd.Flags().BoolVar(&noBrowser, "no-browser", false, "Print URL instead of opening browser")
	cmd.Flags().BoolVar(&force, "force", false, "Force re-authentication even if already logged in")

	return cmd
}

func newLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "logout",
		Short:         "Log out of Intentra",
		SilenceUsage:  true,
		SilenceErrors: true,
		Long:          `Remove stored credentials and log out of Intentra.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLogout()
		},
	}
}

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "status",
		Short:         "Show authentication status",
		SilenceUsage:  true,
		SilenceErrors: true,
		Long:          `Display current authentication status and user information.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStatus()
		},
	}
}

func runLogin(noBrowser, force bool) error {
	creds, _ := auth.GetValidCredentials()
	if creds != nil && !force {
		fmt.Println("Already logged in.")
		fmt.Println("Use 'intentra login --force' to re-authenticate.")
		return nil
	}

	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	endpoint := cfg.Server.Endpoint
	if endpoint == "" {
		endpoint = config.DefaultAPIEndpoint
	}

	fmt.Println("Initiating device authorization...")

	deviceResp, err := requestDeviceCode(endpoint)
	if err != nil {
		return fmt.Errorf("failed to initiate login: %w", err)
	}

	fmt.Println()
	fmt.Printf("Please visit: %s\n", deviceResp.VerificationURI)
	fmt.Printf("Enter code: %s\n", deviceResp.UserCode)
	fmt.Println()

	if !noBrowser && deviceResp.VerificationURIComplete != "" {
		if err := openBrowser(deviceResp.VerificationURIComplete); err != nil {
			fmt.Println("Could not open browser automatically.")
			fmt.Println("Please visit the URL above manually.")
		} else {
			fmt.Println("Browser opened. Complete authorization in your browser.")
		}
	}

	fmt.Println("Waiting for authorization...")

	tokenResp, err := pollForToken(endpoint, deviceResp)
	if err != nil {
		return fmt.Errorf("authorization failed: %w", err)
	}

	creds = auth.CredentialsFromTokenResponse(tokenResp)
	if err := auth.StoreCredentialsInKeyring(creds); err != nil {
		fmt.Printf("Warning: secure storage unavailable, using encrypted cache: %v\n", err)
		if err := auth.WriteEncryptedCache(creds); err != nil {
			return fmt.Errorf("failed to save credentials: %w", err)
		}
	}

	fmt.Println()
	fmt.Println("✓ Successfully logged in!")

	if err := registerMachine(endpoint, creds.AccessToken); err != nil {
		fmt.Printf("\nWarning: failed to register device: %v\n", err)
		fmt.Println("You can retry by running 'intentra login' again.")
	} else {
		fmt.Println("✓ Device registered")
	}

	// Flush any scans queued while unauthenticated
	if pending := queue.PendingCount(); pending > 0 {
		fmt.Printf("\nFound %d offline scan(s). Syncing...\n", pending)
		queue.FlushWithJWT(creds.AccessToken)
	}

	fmt.Println()
	fmt.Println("You can now use Intentra with server sync enabled.")
	fmt.Println("Run 'intentra status' to see your account info.")

	return nil
}

func runLogout() error {
	creds, _ := auth.GetValidCredentials()
	if creds == nil {
		fmt.Println("You are not logged in.")
		return nil
	}

	if err := auth.DeleteCredentialsFromKeyring(); err != nil {
		return fmt.Errorf("failed to logout: %w", err)
	}

	fmt.Println("✓ Successfully logged out.")
	return nil
}

func runStatus() error {
	creds, err := auth.LoadCredentialsFromKeyring()
	if err != nil {
		return fmt.Errorf("failed to load credentials: %w", err)
	}

	if creds == nil {
		fmt.Println("Status: Not logged in")
		fmt.Println()
		fmt.Println("Run 'intentra login' to authenticate.")
		return nil
	}

	if creds.IsExpired() {
		refreshed, err := auth.RefreshCredentials(creds)
		if err != nil {
			fmt.Println("Status: Session expired")
			fmt.Println()
			if creds.Email != "" {
				fmt.Printf("Email: %s\n", creds.Email)
			}
			fmt.Println()
			fmt.Println("Run 'intentra login' to re-authenticate.")
			return nil
		}
		creds = refreshed
	}

	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	endpoint := cfg.Server.Endpoint
	if endpoint == "" {
		endpoint = config.DefaultAPIEndpoint
	}

	profile, err := fetchUserProfile(endpoint, creds.AccessToken)
	if err != nil {
		fmt.Println("Status: Logged in")
		fmt.Println()
		if creds.Email != "" {
			fmt.Printf("Email: %s\n", creds.Email)
		}
		fmt.Println("Unable to fetch profile details. Server may be temporarily slow.")
		return nil
	}

	if profile.CurrentOrgID == "" {
		fmt.Printf("Email: %s\n", profile.Email)
		fmt.Println("Organization: None")
		fmt.Println("Plan: Free")
		return nil
	}

	org, err := fetchOrganization(endpoint, creds.AccessToken, profile.CurrentOrgID)
	if err != nil {
		fmt.Printf("Email: %s\n", profile.Email)
		fmt.Println("Unable to fetch organization details.")
		return nil
	}

	fmt.Printf("Email: %s\n", profile.Email)
	fmt.Printf("Organization: %s\n", org.Name)
	fmt.Printf("Plan: %s\n", capitalizeFirst(org.Plan))

	return nil
}

type userProfile struct {
	Email        string `json:"email"`
	Name         string `json:"name"`
	CurrentOrgID string `json:"current_org_id"`
}

type organization struct {
	OrgID string `json:"org_id"`
	Name  string `json:"name"`
	Plan  string `json:"plan"`
}

func fetchUserProfile(endpoint, accessToken string) (*userProfile, error) {
	url := endpoint + "/users/me"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := httputil.DefaultClient.Do(req)
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

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch user profile: %d - %s", resp.StatusCode, string(body))
	}

	var result struct {
		User userProfile `json:"user"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &result.User, nil
}

func fetchOrganization(endpoint, accessToken, orgID string) (*organization, error) {
	url := endpoint + "/orgs/" + url.PathEscape(orgID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := httputil.DefaultClient.Do(req)
	if err != nil {
		debug.LogHTTP("GET", url, 0)
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	debug.LogHTTP("GET", url, resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch organization: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, httputil.MaxResponseSize))
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var result struct {
		Organization organization `json:"organization"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &result.Organization, nil
}

func capitalizeFirst(s string) string {
	if s == "" {
		return s
	}
	runes := []rune(s)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}


func requestDeviceCode(endpoint string) (*auth.DeviceCodeResponse, error) {
	url := endpoint + "/oauth/device/code"

	resp, err := httputil.DefaultClient.Post(url, "application/json", nil)
	if err != nil {
		debug.LogHTTP("POST", url, 0)
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	debug.LogHTTP("POST", url, resp.StatusCode)

	body, err := io.ReadAll(io.LimitReader(resp.Body, httputil.MaxResponseSize))
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server error: %s", string(body))
	}

	var deviceResp auth.DeviceCodeResponse
	if err := json.Unmarshal(body, &deviceResp); err != nil {
		return nil, fmt.Errorf("invalid response: %w", err)
	}

	return &deviceResp, nil
}

func pollForToken(endpoint string, deviceResp *auth.DeviceCodeResponse) (*auth.TokenResponse, error) {
	url := endpoint + "/oauth/token"
	interval := time.Duration(deviceResp.Interval) * time.Second
	if interval < time.Second {
		interval = 5 * time.Second
	}
	timeout := time.After(time.Duration(deviceResp.ExpiresIn) * time.Second)

	payload := map[string]string{
		"grant_type":  "urn:ietf:params:oauth:grant-type:device_code",
		"device_code": deviceResp.DeviceCode,
	}
	payloadBytes, _ := json.Marshal(payload)

	for {
		select {
		case <-timeout:
			return nil, fmt.Errorf("authorization timed out - device code expired")
		default:
		}

		resp, err := httputil.DefaultClient.Post(url, "application/json", bytes.NewReader(payloadBytes))
		if err != nil {
			debug.LogHTTP("POST", url, 0)
			time.Sleep(interval)
			continue
		}

		body, _ := io.ReadAll(io.LimitReader(resp.Body, httputil.MaxResponseSize))
		resp.Body.Close()
		debug.LogHTTP("POST", url, resp.StatusCode)

		var tokenResp auth.TokenResponse
		if err := json.Unmarshal(body, &tokenResp); err != nil {
			time.Sleep(interval)
			continue
		}

		if tokenResp.AccessToken != "" {
			return &tokenResp, nil
		}

		switch tokenResp.Error {
		case "authorization_pending":
			time.Sleep(interval)
			continue
		case "slow_down":
			interval += 5 * time.Second
			time.Sleep(interval)
			continue
		case "expired_token":
			return nil, fmt.Errorf("device code expired - please try again")
		case "access_denied":
			return nil, fmt.Errorf("authorization denied by user")
		default:
			if tokenResp.Error != "" {
				return nil, fmt.Errorf("%s: %s", tokenResp.Error, tokenResp.ErrorDesc)
			}
			time.Sleep(interval)
			continue
		}
	}
}

var browserLauncher func(url string) error

func openBrowser(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if parsed.Scheme != "https" {
		return fmt.Errorf("refusing to open non-HTTPS URL: %s", parsed.Scheme)
	}

	if browserLauncher != nil {
		return browserLauncher(rawURL)
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", rawURL)
	case "linux":
		cmd = exec.Command("xdg-open", rawURL)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", rawURL)
	default:
		return fmt.Errorf("unsupported platform")
	}
	return cmd.Start()
}

func registerMachine(endpoint, accessToken string) error {
	deviceID, err := device.GetDeviceID()
	if err != nil {
		return fmt.Errorf("failed to get device ID: %w", err)
	}

	metadata := device.GetMetadata()

	payload := map[string]string{
		"machine_id": deviceID,
		"os":         metadata.Platform,
		"hostname":   metadata.Hostname,
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	url := endpoint + "/machines"
	req, err := http.NewRequest("POST", url, bytes.NewReader(payloadBytes))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := httputil.DefaultClient.Do(req)
	if err != nil {
		debug.LogHTTP("POST", url, 0)
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	debug.LogHTTP("POST", url, resp.StatusCode)

	body, _ := io.ReadAll(io.LimitReader(resp.Body, httputil.MaxResponseSize))

	switch resp.StatusCode {
	case http.StatusOK, http.StatusCreated:
		return nil
	case http.StatusForbidden:
		var errResp struct {
			Error     string `json:"error"`
			ErrorCode string `json:"error_code"`
		}
		if json.Unmarshal(body, &errResp) == nil {
			switch errResp.ErrorCode {
			case "MACHINE_LIMIT_REACHED":
				return fmt.Errorf("device limit reached for your plan - upgrade at https://intentra.sh/billing")
			case "MACHINE_ADMIN_REVOKED":
				return fmt.Errorf("this device was revoked by an administrator - contact support")
			}
		}
		return fmt.Errorf("registration forbidden: %s", string(body))
	default:
		return fmt.Errorf("registration failed (%d): %s", resp.StatusCode, string(body))
	}
}
