// Package config manages Intentra configuration loading, validation, and
// defaults. It supports file-based configuration, environment variables,
// and multiple authentication modes for server sync.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/spf13/viper"
)

var (
	configMu     sync.Mutex
	configLoaded bool
	cachedConfig *Config
	cachedErr    error
)

// DefaultAPIEndpoint is the default Intentra API server endpoint.
const DefaultAPIEndpoint = "https://api.intentra.sh"

// AuthModeAPIKey is the config value for API key authentication.
const AuthModeAPIKey = "api_key"

// Config represents the intentra configuration.
type Config struct {
	// Debug mode enables HTTP request logging and local scan saving
	Debug bool `mapstructure:"debug"`

	// RichTraces enables inclusion of tool inputs/outputs and command content in event payloads.
	// Controlled via INTENTRA_RICH_TRACES environment variable or config key rich_traces.
	RichTraces bool `mapstructure:"rich_traces"`

	// Server sync configuration (optional - for team deployments)
	Server ServerConfig `mapstructure:"server"`

	// Local settings
	Local LocalConfig `mapstructure:"local"`

	// Buffer configuration
	Buffer BufferConfig `mapstructure:"buffer"`

	// Logging configuration
	Log LogConfig `mapstructure:"logging"`
}

// ServerConfig contains API server settings for team deployments.
type ServerConfig struct {
	Enabled  bool          `mapstructure:"enabled"`
	Endpoint string        `mapstructure:"endpoint"`
	Timeout  time.Duration `mapstructure:"timeout"`
	Auth     AuthConfig    `mapstructure:"auth"`
}

// AuthConfig contains authentication settings.
type AuthConfig struct {
	Mode   string       `mapstructure:"mode"` // api_key (or use 'intentra login' for JWT)
	APIKey APIKeyConfig `mapstructure:"api_key"`
}

// APIKeyConfig contains API key authentication settings for Enterprise organizations.
// When hmac_key is set, requests are signed with HMAC-SHA256 and the raw secret
// is never transmitted. When only secret is set, legacy bcrypt mode is used.
type APIKeyConfig struct {
	KeyID   string `mapstructure:"key_id"`   // API key identifier from Settings > API Keys
	Secret  string `mapstructure:"secret"`   // API key secret (legacy bcrypt mode)
	HMACKey string `mapstructure:"hmac_key"` // HMAC signing key (preferred, shown once at creation)
}

// LocalConfig contains local-only settings.
type LocalConfig struct {
	AnthropicAPIKey  string        `mapstructure:"anthropic_api_key"`
	Model            string        `mapstructure:"model"`
	ScanTimeout      int           `mapstructure:"scan_timeout"`
	MinEventsPerScan int           `mapstructure:"min_events_per_scan"`
	CharsPerToken    int           `mapstructure:"chars_per_token"`
	Archive          ArchiveConfig `mapstructure:"archive"`
}

// ArchiveConfig contains local scan archive settings for benchmarking.
type ArchiveConfig struct {
	Enabled       bool   `mapstructure:"enabled"`
	Path          string `mapstructure:"path"`
	Redacted      bool   `mapstructure:"redacted"`
	IncludeEvents bool   `mapstructure:"include_events"`
}

// BufferConfig contains local buffer settings.
type BufferConfig struct {
	Enabled        bool          `mapstructure:"enabled"`
	Path           string        `mapstructure:"path"`
	MaxSizeMB      int           `mapstructure:"max_size_mb"`
	MaxAgeHours    int           `mapstructure:"max_age_hours"`
	FlushInterval  time.Duration `mapstructure:"flush_interval"`
	FlushThreshold int           `mapstructure:"flush_threshold"`
}

// LogConfig contains logging settings.
type LogConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
}

// DefaultConfig returns configuration with sensible defaults.
func DefaultConfig() *Config {
	dataDir, _ := GetDataDir()
	return &Config{
		Debug: false,
		Server: ServerConfig{
			Enabled:  false,
			Endpoint: "",
			Timeout:  30 * time.Second,
			Auth: AuthConfig{
				Mode: "",
			},
		},
		Local: LocalConfig{
			Model:            "claude-3-5-haiku-latest",
			ScanTimeout:      30,
			MinEventsPerScan: 2,
			CharsPerToken:    4,
			Archive: ArchiveConfig{
				Enabled:       false,
				Path:          filepath.Join(dataDir, "archive"),
				Redacted:      true,
				IncludeEvents: false,
			},
		},
		Buffer: BufferConfig{
			Enabled:        false,
			Path:           filepath.Join(dataDir, "buffer.db"),
			MaxSizeMB:      50,
			MaxAgeHours:    24,
			FlushInterval:  30 * time.Second,
			FlushThreshold: 10,
		},
		Log: LogConfig{
			Level:  "warn",
			Format: "text",
		},
	}
}

// Load reads configuration from file and environment.
// Results are cached; call InvalidateCache to force re-read from disk.
func Load() (*Config, error) {
	configMu.Lock()
	defer configMu.Unlock()
	if !configLoaded {
		cachedConfig, cachedErr = loadImpl()
		configLoaded = true
	}
	return cachedConfig, cachedErr
}

// InvalidateCache resets the cached config so the next Load re-reads from disk.
func InvalidateCache() {
	configMu.Lock()
	defer configMu.Unlock()
	configLoaded = false
	cachedConfig = nil
	cachedErr = nil
}

func loadImpl() (*Config, error) {
	if err := EnsureDirectories(); err != nil {
		return nil, err
	}

	cfg := DefaultConfig()
	v := viper.New()

	// Config file locations
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	if configDir, err := GetConfigDir(); err == nil {
		v.AddConfigPath(configDir)
	}
	v.AddConfigPath("/etc/intentra")
	v.AddConfigPath(".")

	// Set defaults
	v.SetDefault("local.model", cfg.Local.Model)
	v.SetDefault("local.scan_timeout", cfg.Local.ScanTimeout)
	v.SetDefault("local.min_events_per_scan", cfg.Local.MinEventsPerScan)
	v.SetDefault("local.chars_per_token", cfg.Local.CharsPerToken)
	v.SetDefault("local.archive.enabled", cfg.Local.Archive.Enabled)
	v.SetDefault("local.archive.path", cfg.Local.Archive.Path)
	v.SetDefault("local.archive.redacted", cfg.Local.Archive.Redacted)
	v.SetDefault("local.archive.include_events", cfg.Local.Archive.IncludeEvents)
	v.SetDefault("buffer.enabled", cfg.Buffer.Enabled)
	v.SetDefault("buffer.path", cfg.Buffer.Path)
	v.SetDefault("buffer.max_size_mb", cfg.Buffer.MaxSizeMB)
	v.SetDefault("buffer.max_age_hours", cfg.Buffer.MaxAgeHours)
	v.SetDefault("buffer.flush_interval", cfg.Buffer.FlushInterval)
	v.SetDefault("buffer.flush_threshold", cfg.Buffer.FlushThreshold)

	// Environment variable overrides
	v.SetEnvPrefix("INTENTRA")
	v.AutomaticEnv()

	// Try to read config file (ignore if not exists)
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("error reading config: %w", err)
		}
	} else {
		// Warn if the config file is world-readable (group or other permissions set)
		if cfgPath := v.ConfigFileUsed(); cfgPath != "" {
			if info, statErr := os.Stat(cfgPath); statErr == nil {
				if info.Mode().Perm()&0o077 != 0 {
					fmt.Fprintf(os.Stderr, "Warning: config file %s has overly permissive permissions %o; consider chmod 600\n", cfgPath, info.Mode().Perm())
				}
			}
		}
	}

	// Unmarshal
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("error parsing config: %w", err)
	}

	cfg.applyEnvOverrides()

	return cfg, nil
}

// LoadWithFile reads configuration from a specific file.
func LoadWithFile(cfgFile string) (*Config, error) {
	if err := EnsureDirectories(); err != nil {
		return nil, err
	}

	cfg := DefaultConfig()
	v := viper.New()

	if cfgFile != "" {
		v.SetConfigFile(cfgFile)
	} else {
		return Load()
	}

	v.SetEnvPrefix("INTENTRA")
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("error reading config: %w", err)
	}

	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("error parsing config: %w", err)
	}

	cfg.applyEnvOverrides()

	return cfg, nil
}

// applyEnvOverrides expands environment variables in sensitive fields and
// applies environment variable overrides. Called by both Load and LoadWithFile.
func (cfg *Config) applyEnvOverrides() {
	cfg.Server.Auth.APIKey.KeyID = os.ExpandEnv(cfg.Server.Auth.APIKey.KeyID)
	cfg.Server.Auth.APIKey.Secret = os.ExpandEnv(cfg.Server.Auth.APIKey.Secret)
	cfg.Server.Auth.APIKey.HMACKey = os.ExpandEnv(cfg.Server.Auth.APIKey.HMACKey)
	cfg.Local.AnthropicAPIKey = os.ExpandEnv(cfg.Local.AnthropicAPIKey)

	if keyID := os.Getenv("INTENTRA_API_KEY_ID"); keyID != "" {
		cfg.Server.Auth.APIKey.KeyID = keyID
	}
	if secret := os.Getenv("INTENTRA_API_SECRET"); secret != "" {
		cfg.Server.Auth.APIKey.Secret = secret
	}
	if hmacKey := os.Getenv("INTENTRA_API_HMAC_KEY"); hmacKey != "" {
		cfg.Server.Auth.APIKey.HMACKey = hmacKey
	}
	if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
		cfg.Local.AnthropicAPIKey = key
	}
	if endpoint := os.Getenv("INTENTRA_SERVER_ENDPOINT"); endpoint != "" {
		cfg.Server.Enabled = true
		cfg.Server.Endpoint = endpoint
	}
	if os.Getenv("INTENTRA_RICH_TRACES") == "true" || os.Getenv("INTENTRA_RICH_TRACES") == "1" {
		cfg.RichTraces = true
	}
}

// Validate checks if the configuration is valid for server sync.
func (c *Config) Validate() error {
	if !c.Server.Enabled {
		return nil
	}

	if c.Server.Endpoint == "" {
		return fmt.Errorf("server.endpoint is required when server sync is enabled")
	}

	switch c.Server.Auth.Mode {
	case AuthModeAPIKey:
		if c.Server.Auth.APIKey.KeyID == "" {
			return fmt.Errorf("api_key auth requires key_id")
		}
		if c.Server.Auth.APIKey.HMACKey == "" && c.Server.Auth.APIKey.Secret == "" {
			return fmt.Errorf("api_key auth requires hmac_key (preferred) or secret")
		}
		if !strings.HasPrefix(c.Server.Endpoint, "https://") {
			return fmt.Errorf("api_key auth requires HTTPS endpoint")
		}
	case "":
		return nil
	default:
		return fmt.Errorf("unknown auth mode: %s (supported: api_key, or use 'intentra login' for JWT)", c.Server.Auth.Mode)
	}

	return nil
}

// Print outputs the current configuration (redacting secrets).
func (c *Config) Print() {
	fmt.Println("=== Intentra Configuration ===")
	fmt.Println()

	fmt.Printf("Debug: %v\n", c.Debug)
	fmt.Println()

	fmt.Println("Server Sync:")
	fmt.Printf("  Enabled: %v\n", c.Server.Enabled)
	if c.Server.Enabled {
		fmt.Printf("  Endpoint: %s\n", c.Server.Endpoint)
		fmt.Printf("  Timeout: %s\n", c.Server.Timeout)
		if c.Server.Auth.Mode != "" {
			fmt.Printf("  Auth Mode: %s\n", c.Server.Auth.Mode)
		} else {
			fmt.Printf("  Auth Mode: jwt (via 'intentra login')\n")
		}
		if c.Server.Auth.Mode == AuthModeAPIKey {
			fmt.Printf("  Key ID: %s\n", c.Server.Auth.APIKey.KeyID)
			if c.Server.Auth.APIKey.HMACKey != "" {
				fmt.Printf("  HMAC Key: [REDACTED] (HMAC signing enabled)\n")
			} else if c.Server.Auth.APIKey.Secret != "" {
				fmt.Printf("  Secret: [REDACTED] (legacy mode)\n")
			}
		}
	}
	fmt.Println()

	fmt.Println("Local:")
	fmt.Printf("  Model: %s\n", c.Local.Model)
	if c.Local.AnthropicAPIKey != "" {
		fmt.Printf("  Anthropic API Key: [REDACTED]\n")
	}
	fmt.Println()

	fmt.Println("Archive:")
	fmt.Printf("  Enabled: %v\n", c.Local.Archive.Enabled)
	fmt.Printf("  Path: %s\n", c.Local.Archive.Path)
	fmt.Printf("  Redacted: %v\n", c.Local.Archive.Redacted)
	fmt.Printf("  Include Events: %v\n", c.Local.Archive.IncludeEvents)
	fmt.Println()

	fmt.Println("Buffer:")
	fmt.Printf("  Enabled: %v\n", c.Buffer.Enabled)
	fmt.Printf("  Path: %s\n", c.Buffer.Path)
	fmt.Printf("  Max Size: %d MB\n", c.Buffer.MaxSizeMB)
	fmt.Printf("  Flush Interval: %s\n", c.Buffer.FlushInterval)
}

// PrintSample outputs a sample configuration file.
func PrintSample() {
	sample := `# Intentra Configuration
# ~/.intentra/config.yaml

# Debug mode (logs HTTP requests, saves scans locally)
debug: false

# Server sync (for team deployments)
# Most users should use 'intentra login' instead of configuring auth here.
server:
  enabled: false
  endpoint: "https://api.intentra.sh"
  timeout: 30s
  auth:
    # Auth mode: api_key
    # Leave mode empty to use JWT from 'intentra login' (recommended)
    mode: ""

    # API key authentication (Enterprise only)
    # Generate keys in Settings > API Keys on the web dashboard
    # api_key:
    #   key_id: "${INTENTRA_API_KEY_ID}"       # API key ID (apk_...)
    #   hmac_key: "${INTENTRA_API_HMAC_KEY}"   # HMAC signing key (preferred, never transmitted)
    #   secret: "${INTENTRA_API_SECRET}"       # Legacy mode: raw secret (use hmac_key instead)

# Local settings
local:
  anthropic_api_key: "${ANTHROPIC_API_KEY}"
  model: "claude-3-5-haiku-latest"
  scan_timeout: 30
  min_events_per_scan: 2
  chars_per_token: 4

  # Local scan archive (for benchmarking)
  archive:
    enabled: false
    path: ~/.intentra/archive
    redacted: true
    include_events: false

# Buffer for offline resilience
buffer:
  enabled: true
  path: ~/.intentra/buffer.db
  max_size_mb: 50
  max_age_hours: 24
  flush_interval: 30s
  flush_threshold: 10

# Logging
logging:
  level: warn
  format: text
`
	fmt.Print(sample)
}

// ConfigExists returns true if the config file exists.
func ConfigExists() bool {
	p, err := GetConfigPath()
	if err != nil {
		return false
	}
	_, statErr := os.Stat(p)
	return statErr == nil
}

// SaveConfig writes the configuration to the config file.
// It preserves existing values and only updates specified fields.
func SaveConfig(cfg *Config) error {
	configPath, err := GetConfigPath()
	if err != nil {
		return fmt.Errorf("failed to determine config path: %w", err)
	}

	configDir, err := GetConfigDir()
	if err != nil {
		return fmt.Errorf("failed to determine config directory: %w", err)
	}
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	v := viper.New()
	v.SetConfigType("yaml")

	if _, err := os.Stat(configPath); err == nil {
		v.SetConfigFile(configPath)
		if err := v.ReadInConfig(); err != nil {
			v = viper.New()
			v.SetConfigType("yaml")
		}
	}

	v.Set("debug", cfg.Debug)
	v.Set("server.enabled", cfg.Server.Enabled)
	v.Set("server.endpoint", cfg.Server.Endpoint)
	v.Set("server.timeout", cfg.Server.Timeout.String())
	v.Set("server.auth.mode", cfg.Server.Auth.Mode)
	v.Set("local.model", cfg.Local.Model)
	v.Set("local.scan_timeout", cfg.Local.ScanTimeout)
	v.Set("local.min_events_per_scan", cfg.Local.MinEventsPerScan)
	v.Set("local.chars_per_token", cfg.Local.CharsPerToken)
	v.Set("local.archive.enabled", cfg.Local.Archive.Enabled)
	v.Set("local.archive.path", cfg.Local.Archive.Path)
	v.Set("local.archive.redacted", cfg.Local.Archive.Redacted)
	v.Set("local.archive.include_events", cfg.Local.Archive.IncludeEvents)
	v.Set("logging.level", cfg.Log.Level)
	v.Set("logging.format", cfg.Log.Format)

	// Write to temp file first, then atomically rename
	tmpPath := configPath + ".tmp"
	if err := v.WriteConfigAs(tmpPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to write config: %w", err)
	}
	if err := os.Chmod(tmpPath, 0600); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to set config file permissions: %w", err)
	}
	if err := os.Rename(tmpPath, configPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to install config: %w", err)
	}
	return nil
}

