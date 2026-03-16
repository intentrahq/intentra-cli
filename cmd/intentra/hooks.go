package main

import (
	"fmt"
	"strings"

	"github.com/intentrahq/intentra-cli/internal/config"
	"github.com/intentrahq/intentra-cli/internal/hooks"
	"github.com/spf13/cobra"
)

func newHooksCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "hooks",
		Short: "Check hook installation status",
	}

	cmd.AddCommand(newHooksStatusCmd())

	return cmd
}

func saveAPIConfig(server, keyID, secret string) error {
	cfg, err := config.Load()
	if err != nil {
		cfg = config.DefaultConfig()
	}

	cfg.Server.Enabled = true
	cfg.Server.Endpoint = server
	cfg.Server.Auth.Mode = config.AuthModeAPIKey
	cfg.Server.Auth.APIKey.KeyID = keyID
	cfg.Server.Auth.APIKey.Secret = secret

	return config.SaveConfig(cfg)
}

func newHooksStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "status",
		Short:         "Check hooks installation status",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			statuses := hooks.Status()

			fmt.Println("Hook Installation Status:")
			fmt.Println(strings.Repeat("-", 50))

			for _, s := range statuses {
				status := "✗ Not installed"
				if s.Installed {
					status = "✓ Installed"
				}
				fmt.Printf("%-12s %s\n", s.Tool+":", status)
				if s.Path != "" {
					fmt.Printf("             Path: %s\n", s.Path)
				}
				if s.Error != nil {
					fmt.Printf("             Error: %v\n", s.Error)
				}
			}

			return nil
		},
	}
}
