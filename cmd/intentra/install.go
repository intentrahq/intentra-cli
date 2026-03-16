package main

import (
	"fmt"
	"os"

	"github.com/intentrahq/intentra-cli/internal/hooks"
	"github.com/spf13/cobra"
)

func newInstallCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "install [tool]",
		Short:         "Install hooks for AI tools",
		SilenceUsage:  true,
		SilenceErrors: true,
		Long: `Install hooks for AI coding tools. Supported tools:
  - cursor: Cursor editor
  - claude: Claude Code CLI
  - gemini: Gemini CLI
  - copilot: GitHub Copilot
  - windsurf: Windsurf Cascade
  - all: All supported tools (default)

Examples:
  intentra install           # Install for all tools
  intentra install cursor    # Install for Cursor only
  intentra install claude    # Install for Claude Code only`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if apiServer != "" && apiKeyID != "" && apiSecret != "" {
				if err := saveAPIConfig(apiServer, apiKeyID, apiSecret); err != nil {
					fmt.Fprintf(os.Stderr, "Error: failed to save config: %v\n", err)
					return err
				}
				fmt.Println("✓ Saved API configuration")
			}

			execPath := "intentra"

			tool := "all"
			if len(args) > 0 {
				tool = args[0]
			}

			if tool == "all" {
				results := hooks.InstallAll(execPath)
				var errors []string
				for t, err := range results {
					if err != nil {
						errors = append(errors, fmt.Sprintf("%s: %v", t, err))
					} else {
						fmt.Printf("✓ Installed hooks for %s\n", t)
					}
				}
				if len(errors) > 0 {
					fmt.Println("\nSome installations failed:")
					for _, e := range errors {
						fmt.Printf("  ✗ %s\n", e)
					}
				}
				fmt.Println("\nPlease restart your AI tools for hooks to take effect.")
				return nil
			}

			t := hooks.Tool(tool)
			if err := hooks.Install(t, execPath); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				return err
			}

			fmt.Printf("✓ Hooks installed for %s\n", tool)
			fmt.Printf("Please restart %s for hooks to take effect.\n", tool)
			return nil
		},
	}

	return cmd
}

func newUninstallCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "uninstall [tool]",
		Short:         "Remove hooks from AI tools",
		SilenceUsage:  true,
		SilenceErrors: true,
		Long: `Remove hooks from AI coding tools. Supported tools:
  - cursor: Cursor editor
  - claude: Claude Code CLI
  - gemini: Gemini CLI
  - copilot: GitHub Copilot
  - windsurf: Windsurf Cascade
  - all: All supported tools (default)

Examples:
  intentra uninstall         # Uninstall from all tools
  intentra uninstall cursor  # Uninstall from Cursor only
  intentra uninstall claude  # Uninstall from Claude Code only`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			tool := "all"
			if len(args) > 0 {
				tool = args[0]
			}

			if tool == "all" {
				results := hooks.UninstallAll()
				var errors []string
				for t, err := range results {
					if err != nil {
						errors = append(errors, fmt.Sprintf("%s: %v", t, err))
					} else {
						fmt.Printf("✓ Uninstalled hooks from %s\n", t)
					}
				}
				if len(errors) > 0 {
					fmt.Println("\nSome uninstallations had issues:")
					for _, e := range errors {
						fmt.Printf("  ✗ %s\n", e)
					}
				}
				fmt.Println("\nPlease restart your AI tools for changes to take effect.")
				return nil
			}

			t := hooks.Tool(tool)
			if err := hooks.Uninstall(t); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				return err
			}

			fmt.Printf("✓ Hooks uninstalled from %s\n", tool)
			fmt.Printf("Please restart %s for changes to take effect.\n", tool)
			return nil
		},
	}

	return cmd
}
