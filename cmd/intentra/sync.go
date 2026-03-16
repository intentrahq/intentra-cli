package main

import (
	"fmt"
	"os"

	"github.com/intentrahq/intentra-cli/internal/api"
	"github.com/intentrahq/intentra-cli/internal/auth"
	"github.com/intentrahq/intentra-cli/internal/debug"
	"github.com/intentrahq/intentra-cli/internal/queue"
	"github.com/intentrahq/intentra-cli/internal/scanner"
	"github.com/intentrahq/intentra-cli/pkg/models"
	"github.com/spf13/cobra"
)

var keepLocal bool

// newSyncNowCmd returns the sync now command with flags.
func newSyncNowCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "now",
		Short:         "Force sync all pending scans",
		SilenceUsage:  true,
		SilenceErrors: true,
		Long: `Sync all pending scans to the server and clean up local files.

By default, local scan files are deleted after successful sync since
the server is the source of truth. Use --keep-local to preserve files.

Local scan files are automatically preserved when debug mode is enabled
(-d flag, debug: true in config, or INTENTRA_DEBUG=true).`,
		RunE: runSyncNow,
	}

	cmd.Flags().BoolVar(&keepLocal, "keep-local", false, "Keep local scan files after syncing")

	return cmd
}

// runSyncNow syncs all pending scans to the configured server.
func runSyncNow(cmd *cobra.Command, args []string) error {
	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return err
	}
	if !cfg.Server.Enabled {
		fmt.Fprintln(os.Stderr, "Error: server sync is not enabled. Set server.enabled=true in config or set INTENTRA_SERVER_ENDPOINT")
		return fmt.Errorf("server sync not enabled")
	}

	scans, err := scanner.LoadScans()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to load scans: %v\n", err)
		return err
	}

	if len(scans) == 0 {
		fmt.Println("No scans to sync. Run 'intentra scan aggregate' first to process events.")
		return nil
	}

	var pending []*models.Scan
	for i := range scans {
		if scans[i].Status == models.ScanStatusPending || scans[i].Status == models.ScanStatusAnalyzing {
			pending = append(pending, &scans[i])
		}
	}

	if len(pending) == 0 {
		fmt.Println("No pending scans to sync. All scans have been reviewed.")
		return nil
	}

	fmt.Printf("Syncing %d scans to %s...\n", len(pending), cfg.Server.Endpoint)

	client, err := api.NewClient(cfg)
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	if err := client.SendScans(pending); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: some scans failed to sync: %v\n", err)
	} else {
		fmt.Printf("✓ Successfully synced %d scans\n", len(pending))
	}

	preserveLocal := keepLocal || debug.Enabled
	if preserveLocal {
		for _, scan := range pending {
			scan.Status = models.ScanStatusReviewed
			if err := scanner.SaveScan(scan); err != nil {
				fmt.Printf("Warning: failed to update scan %s status: %v\n", scan.ID, err)
			}
		}
		if debug.Enabled {
			fmt.Println("Local scan files preserved (debug mode)")
		} else {
			fmt.Println("Local scan files preserved (--keep-local)")
		}
	} else {
		var deleted int
		for _, scan := range pending {
			if err := scanner.DeleteScan(scan.ID); err != nil {
				fmt.Printf("Warning: failed to delete local scan %s: %v\n", scan.ID, err)
			} else {
				deleted++
			}
		}
		fmt.Printf("Cleaned up %d local scan files (server is now source of truth)\n", deleted)
	}

	// Also flush any encrypted offline queue entries
	if queuedCount := queue.PendingCount(); queuedCount > 0 {
		fmt.Printf("Flushing %d offline queued scan(s)...\n", queuedCount)
		creds, err := auth.GetValidCredentials()
		if err != nil || creds == nil {
			fmt.Fprintln(os.Stderr, "Warning: cannot flush offline queue - not authenticated. Run 'intentra login' first.")
		} else {
			queue.FlushWithJWT(creds.AccessToken)
		}
	}

	return nil
}
