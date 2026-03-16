// Package main implements the intentra CLI for monitoring AI coding assistants.
//
// Intentra provides commands for:
//   - Installing hooks into AI tools (Cursor, Claude Code, Gemini CLI, GitHub Copilot, Windsurf)
//   - Managing and aggregating scan data
//   - Syncing scans to a central server
package main

import (
	"fmt"
	"os"

	"github.com/intentrahq/intentra-cli/internal/config"
	"github.com/intentrahq/intentra-cli/internal/debug"
	"github.com/intentrahq/intentra-cli/internal/hooks"
	"github.com/spf13/cobra"
)

var (
	// version is set at build time via ldflags.
	version = "dev"

	// cfgFile holds the path to the configuration file.
	cfgFile string

	// debugMode enables debug output (HTTP requests, local scan saves).
	debugMode bool

	// apiServer, apiKeyID, and apiSecret are CLI flag overrides for server config.
	apiServer string
	apiKeyID  string
	apiSecret string
)

func main() {
	rootCmd := &cobra.Command{
		Use:     "intentra",
		Short:   "AI coding cost tracking and usage monitoring",
		Version: version,
		Long: `Intentra monitors AI coding assistants (Cursor, Claude Code, Gemini CLI, GitHub Copilot, Windsurf),
tracks usage metrics, and optionally syncs data to a central server.`,
	}

	// Global flags
	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "config file (default: ~/.intentra/config.yaml)")
	rootCmd.PersistentFlags().BoolVarP(&debugMode, "debug", "d", false, "enable debug output (HTTP requests, local scan saves)")
	rootCmd.PersistentFlags().StringVar(&apiServer, "api-server", "", "API server endpoint (e.g., https://app.example.com/api/v1)")
	rootCmd.PersistentFlags().StringVar(&apiKeyID, "api-key-id", "", "API key ID for authentication")
	rootCmd.PersistentFlags().StringVar(&apiSecret, "api-secret", "", "API secret for authentication")

	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		return initDebugMode()
	}

	rootCmd.AddCommand(newInstallCmd())
	rootCmd.AddCommand(newUninstallCmd())
	rootCmd.AddCommand(newHooksCmd())
	rootCmd.AddCommand(newScanCmd())
	rootCmd.AddCommand(newConfigCmd())
	rootCmd.AddCommand(newSyncCmd())
	rootCmd.AddCommand(newLoginCmd())
	rootCmd.AddCommand(newLogoutCmd())
	rootCmd.AddCommand(newStatusCmd())
	rootCmd.AddCommand(newExtensionInfoCmd())

	var hookTool string
	var hookEvent string
	hookCmd := &cobra.Command{
		Use:           "hook",
		Short:         "Process a hook event (internal use)",
		Hidden:        true,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := hooks.RunHookHandlerWithToolAndEvent(hookTool, hookEvent); err != nil {
				fmt.Fprintf(os.Stderr, "hook error: %v\n", err)
				return err
			}
			return nil
		},
	}
	hookCmd.Flags().StringVar(&hookTool, "tool", "", "AI tool (cursor, claude, gemini, copilot, windsurf)")
	hookCmd.Flags().StringVar(&hookEvent, "event", "", "Hook event type")
	rootCmd.AddCommand(hookCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// newConfigCmd returns a cobra.Command for managing configuration.
func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage configuration",
	}

	showCmd := &cobra.Command{
		Use:           "show",
		Short:         "Show current configuration",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			cfg.Print()
			return nil
		},
	}

	initCmd := &cobra.Command{
		Use:           "init",
		Short:         "Generate sample configuration file",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			config.PrintSample()
			return nil
		},
	}

	validateCmd := &cobra.Command{
		Use:           "validate",
		Short:         "Validate configuration for server sync",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: failed to load config: %v\n", err)
				return err
			}
			if err := cfg.Validate(); err != nil {
				fmt.Fprintf(os.Stderr, "Error: validation failed: %v\n", err)
				return err
			}
			fmt.Println("✓ Configuration is valid")
			if cfg.Server.Enabled {
				fmt.Printf("  Server: %s\n", cfg.Server.Endpoint)
				fmt.Printf("  Auth: %s\n", cfg.Server.Auth.Mode)
			} else {
				fmt.Println("  Server sync: disabled (local-only mode)")
			}
			return nil
		},
	}

	cmd.AddCommand(showCmd, initCmd, validateCmd)
	return cmd
}

// newSyncCmd returns a cobra.Command for syncing scans to a server.
func newSyncCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Sync scans to server",
		Long: `Sync locally buffered scans to the configured server.
Requires server sync to be enabled in config.`,
	}

	statusCmd := &cobra.Command{
		Use:           "status",
		Short:         "Show sync status",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				return err
			}

			fmt.Println("Sync Status:")
			if cfg.Server.Enabled {
				fmt.Printf("  Server: %s\n", cfg.Server.Endpoint)
				fmt.Printf("  Buffer: %s\n", cfg.Buffer.Path)
			} else {
				fmt.Println("  Server sync: disabled")
				fmt.Println("  Running in local-only mode")
			}
			return nil
		},
	}

	cmd.AddCommand(newSyncNowCmd(), statusCmd)
	return cmd
}

// loadConfig returns the configuration, applying file and CLI flag overrides.
func loadConfig() (*config.Config, error) {
	var cfg *config.Config
	var err error

	if cfgFile != "" {
		cfg, err = config.LoadWithFile(cfgFile)
	} else {
		cfg, err = config.Load()
	}
	if err != nil {
		return nil, err
	}

	// CLI flags override config file and environment variables
	if apiServer != "" {
		cfg.Server.Enabled = true
		cfg.Server.Endpoint = apiServer
	}
	if apiKeyID != "" {
		cfg.Server.Auth.APIKey.KeyID = apiKeyID
	}
	if apiSecret != "" {
		cfg.Server.Auth.APIKey.Secret = apiSecret
	}

	return cfg, nil
}

// initDebugMode initializes debug mode based on config and CLI flag.
// It generates a config file on first run if one doesn't exist.
// If -d flag is used, it persists debug: true to the config file.
func initDebugMode() error {
	if !config.ConfigExists() {
		cfg := config.DefaultConfig()
		if debugMode {
			cfg.Debug = true
		}
		if err := config.SaveConfig(cfg); err != nil {
			debug.Warn("could not save config: %v", err)
		}
	}

	cfg, err := loadConfig()
	if err != nil {
		debug.Enabled = debugMode
		return nil
	}

	if debugMode && !cfg.Debug {
		cfg.Debug = true
		if err := config.SaveConfig(cfg); err != nil {
			debug.Warn("could not persist debug setting: %v", err)
		}
	}

	debug.Enabled = debugMode || cfg.Debug
	return nil
}
