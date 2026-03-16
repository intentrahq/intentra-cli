package main

import (
	"encoding/json"
	"fmt"

	"github.com/intentrahq/intentra-cli/internal/auth"
	"github.com/intentrahq/intentra-cli/internal/config"
	"github.com/intentrahq/intentra-cli/internal/hooks"
	"github.com/spf13/cobra"
)

type extensionInfo struct {
	Version           string `json:"version"`
	SupportsExtension bool   `json:"supports_extension"`
	CredentialsPath   string `json:"credentials_path"`
	HooksInstalled    bool   `json:"hooks_installed"`
	Authenticated     bool   `json:"authenticated"`
}

func newExtensionInfoCmd() *cobra.Command {
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:           "extension-info",
		Short:         "Show info for IDE extension coordination",
		Long:          `Returns CLI information for IDE extension coordination.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runExtensionInfo(jsonOutput)
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output as JSON")

	return cmd
}

func runExtensionInfo(jsonOutput bool) error {
	creds, _ := auth.GetValidCredentials()
	authenticated := creds != nil

	hooksInstalled := hooks.AnyHooksInstalled()

	info := extensionInfo{
		Version:           version,
		SupportsExtension: true,
		CredentialsPath:   func() string { p, _ := config.GetCredentialsFile(); return p }(),
		HooksInstalled:    hooksInstalled,
		Authenticated:     authenticated,
	}

	if jsonOutput {
		data, err := json.Marshal(info)
		if err != nil {
			return fmt.Errorf("failed to marshal JSON: %w", err)
		}
		fmt.Println(string(data))
		return nil
	}

	fmt.Printf("Version: %s\n", info.Version)
	fmt.Printf("Supports Extension: %v\n", info.SupportsExtension)
	fmt.Printf("Credentials Path: %s\n", info.CredentialsPath)
	fmt.Printf("Hooks Installed: %v\n", info.HooksInstalled)
	fmt.Printf("Authenticated: %v\n", info.Authenticated)

	return nil
}
