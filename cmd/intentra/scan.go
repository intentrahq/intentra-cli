package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"text/tabwriter"
	"time"

	"github.com/intentrahq/intentra-cli/internal/api"
	"github.com/intentrahq/intentra-cli/internal/scanner"
	"github.com/intentrahq/intentra-cli/pkg/models"
	"github.com/spf13/cobra"
)

// sortScansByTime sorts scans by StartTime descending (latest first).
func sortScansByTime(scans []models.Scan) {
	sort.Slice(scans, func(i, j int) bool {
		return scans[i].StartTime.After(scans[j].StartTime)
	})
}

// newScanCmd returns a cobra.Command for managing scans.
func newScanCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "scan",
		Short: "Manage scans",
	}

	cmd.AddCommand(newScanListCmd())
	cmd.AddCommand(newScanShowCmd())
	cmd.AddCommand(newScanTodayCmd())
	cmd.AddCommand(newScanAggregateCmd())

	return cmd
}

// newScanListCmd returns a cobra.Command for listing all scans.
func newScanListCmd() *cobra.Command {
	var jsonOutput bool
	var summaryOnly bool
	var days int
	var limit int

	cmd := &cobra.Command{
		Use:           "list",
		Short:         "List all scans",
		SilenceUsage:  true,
		SilenceErrors: true,
		Long: `List scans from the server (if logged in and server enabled) or local storage.

When server mode is enabled, scans are fetched from the API.
When server mode is disabled (local-only), scans are read from local files.

Examples:
  intentra scan list                    # List recent scans (default limit: 20)
  intentra scan list --limit 100        # List up to 100 scans
  intentra scan list --summary          # Show summary only, no individual scans
  intentra scan list --days 7           # Look back 7 days`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			var scans []models.Scan
			var source string
			var totalScans int
			var serverSummary *api.ScansSummary

			if cfg.Server.Enabled {
				client, err := api.NewClient(cfg)
				if err != nil {
					return fmt.Errorf("failed to create API client: %w", err)
				}

				resp, err := client.GetScans(days, limit)
				if err != nil {
					return fmt.Errorf("failed to fetch scans from server: %w", err)
				}
				scans = resp.Scans
				source = "server"
				totalScans = resp.Summary.TotalScans
				serverSummary = &resp.Summary
			} else {
				localScans, err := scanner.LoadScans()
				if err != nil {
					return err
				}
				scans = localScans
				source = "local"
				totalScans = len(scans)
			}

			sortScansByTime(scans)

			if len(scans) == 0 {
				if source == "server" {
					fmt.Println("No scans found on server.")
				} else {
					fmt.Println("No scans found. Run 'intentra scan aggregate' to process events.")
				}
				return nil
			}

			var totalCost float64
			var totalTokens int
			for _, s := range scans {
				totalCost += s.EstimatedCost
				totalTokens += s.TotalTokens
			}

			if summaryOnly {
				if jsonOutput {
					summary := map[string]any{
						"total_scans":    totalScans,
						"total_tokens":   totalTokens,
						"estimated_cost": totalCost,
					}
					data, err := json.MarshalIndent(summary, "", "  ")
					if err != nil {
						return fmt.Errorf("failed to marshal summary: %w", err)
					}
					fmt.Println(string(data))
				} else if serverSummary != nil && serverSummary.TotalScans > 0 {
					fmt.Printf("Summary: %d scans, $%.2f total cost\n",
						serverSummary.TotalScans, serverSummary.TotalCost)
				} else {
					fmt.Printf("Summary: %d scans, $%.2f total cost\n",
						len(scans), totalCost)
				}
				return nil
			}

			if jsonOutput {
				data, err := json.MarshalIndent(scans, "", "  ")
				if err != nil {
					return fmt.Errorf("failed to marshal scans: %w", err)
				}
				fmt.Println(string(data))
				return nil
			}

			if serverSummary != nil && serverSummary.TotalScans > 0 {
				fmt.Printf("Summary: %d scans, $%.2f total cost\n\n",
					serverSummary.TotalScans, serverSummary.TotalCost)
			} else {
				fmt.Printf("Summary: %d scans, $%.2f total cost\n\n",
					len(scans), totalCost)
			}

			displayScans := scans
			if source == "local" && limit > 0 && len(displayScans) > limit {
				displayScans = displayScans[:limit]
				defer func() {
					fmt.Printf("\nShowing %d of %d scans. Use --limit to see more.\n", limit, len(scans))
				}()
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "ID\tEVENTS\tTOKENS\tCOST\tTIME")
			for _, s := range displayScans {
				id := s.ID
				if len(id) > 8 {
					id = id[:8]
				}
				startTime := s.StartTime
				if startTime.IsZero() {
					startTime = time.Now()
				}
				fmt.Fprintf(w, "%s\t%d\t%d\t$%.4f\t%s\n",
					id,
					len(s.Events),
					s.TotalTokens,
					s.EstimatedCost,
					startTime.Format("2006-01-02 15:04"),
				)
			}
			if err := w.Flush(); err != nil {
				return fmt.Errorf("failed to flush output: %w", err)
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output as JSON")
	cmd.Flags().BoolVar(&summaryOnly, "summary", false, "Show summary only, no individual scans")
	cmd.Flags().IntVar(&days, "days", 30, "Number of days to look back (server mode only)")
	cmd.Flags().IntVar(&limit, "limit", 20, "Maximum number of scans to display")

	return cmd
}

// newScanShowCmd returns a cobra.Command for displaying scan details.
func newScanShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "show <id>",
		Short:         "Show scan details",
		SilenceUsage:  true,
		SilenceErrors: true,
		Long: `Show detailed information about a specific scan.

When server mode is enabled, the scan is fetched from the API.
When server mode is disabled (local-only), the scan is read from local files.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			scanID := args[0]

			cfg, err := loadConfig()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			if cfg.Server.Enabled {
				client, err := api.NewClient(cfg)
				if err != nil {
					return fmt.Errorf("failed to create API client: %w", err)
				}

				resp, err := client.GetScan(scanID)
				if err != nil {
					return err
				}

				output := map[string]any{
					"scan": resp.Scan,
				}
				if len(resp.ViolationDetails) > 0 {
					output["violation_details"] = resp.ViolationDetails
				}

				data, err := json.MarshalIndent(output, "", "  ")
				if err != nil {
					return fmt.Errorf("failed to marshal scan: %w", err)
				}
				fmt.Println(string(data))
			} else {
				scan, err := scanner.LoadScan(scanID)
				if err != nil {
					return fmt.Errorf("scan not found: %s", scanID)
				}

				data, err := json.MarshalIndent(scan, "", "  ")
				if err != nil {
					return fmt.Errorf("failed to marshal scan: %w", err)
				}
				fmt.Println(string(data))
			}

			return nil
		},
	}
}

// newScanTodayCmd returns a cobra.Command for showing today's scans.
func newScanTodayCmd() *cobra.Command {
	var jsonOutput bool
	var summaryOnly bool
	var limit int

	cmd := &cobra.Command{
		Use:           "today",
		Short:         "Show today's scans",
		SilenceUsage:  true,
		SilenceErrors: true,
		Long: `List scans from today only. Uses server or local storage based on configuration.

Examples:
  intentra scan today                   # Show today's summary and recent scans
  intentra scan today --summary         # Show summary only
  intentra scan today --limit 50        # Show up to 50 scans`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			var scans []models.Scan
			today := time.Now().Truncate(24 * time.Hour)

			if cfg.Server.Enabled {
				client, err := api.NewClient(cfg)
				if err != nil {
					return fmt.Errorf("failed to create API client: %w", err)
				}

				resp, err := client.GetScans(1, 500)
				if err != nil {
					return fmt.Errorf("failed to fetch scans from server: %w", err)
				}
				scans = resp.Scans
			} else {
				localScans, err := scanner.LoadScans()
				if err != nil {
					return err
				}
				for _, s := range localScans {
					if !s.StartTime.Before(today) {
						scans = append(scans, s)
					}
				}
			}

			sortScansByTime(scans)

			if len(scans) == 0 {
				fmt.Println("No scans found for today.")
				return nil
			}

			var totalCost float64
			var totalTokens int
			for _, s := range scans {
				totalCost += s.EstimatedCost
				totalTokens += s.TotalTokens
			}

			if summaryOnly {
				if jsonOutput {
					summary := map[string]any{
						"date":           today.Format("2006-01-02"),
						"total_scans":    len(scans),
						"total_tokens":   totalTokens,
						"estimated_cost": totalCost,
					}
					data, err := json.MarshalIndent(summary, "", "  ")
					if err != nil {
						return fmt.Errorf("failed to marshal summary: %w", err)
					}
					fmt.Println(string(data))
				} else {
					fmt.Printf("Today: %d scans, %d tokens, $%.4f cost\n",
						len(scans), totalTokens, totalCost)
				}
				return nil
			}

			if jsonOutput {
				data, err := json.MarshalIndent(scans, "", "  ")
				if err != nil {
					return fmt.Errorf("failed to marshal scans: %w", err)
				}
				fmt.Println(string(data))
				return nil
			}

			fmt.Printf("Today: %d scans, %d tokens, $%.4f cost\n\n",
				len(scans), totalTokens, totalCost)

			displayScans := scans
			if limit > 0 && len(displayScans) > limit {
				displayScans = displayScans[:limit]
				defer func() {
					fmt.Printf("\nShowing %d of %d scans. Use --limit to see more.\n", limit, len(scans))
				}()
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "ID\tTOKENS\tCOST\tTIME")
			for _, s := range displayScans {
				id := s.ID
				if len(id) > 8 {
					id = id[:8]
				}
				fmt.Fprintf(w, "%s\t%d\t$%.4f\t%s\n",
					id,
					s.TotalTokens,
					s.EstimatedCost,
					s.StartTime.Format("15:04"),
				)
			}
			if err := w.Flush(); err != nil {
				return fmt.Errorf("failed to flush output: %w", err)
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output as JSON")
	cmd.Flags().BoolVar(&summaryOnly, "summary", false, "Show summary only, no individual scans")
	cmd.Flags().IntVar(&limit, "limit", 20, "Maximum number of scans to display (0 for all)")

	return cmd
}

// newScanAggregateCmd returns a cobra.Command for aggregating events into scans.
func newScanAggregateCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "aggregate",
		Short:         "Process events into scans",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			events, err := scanner.LoadEvents()
			if err != nil {
				return fmt.Errorf("failed to load events: %w", err)
			}

			if len(events) == 0 {
				fmt.Println("No events found. Use Cursor with hooks installed to generate events.")
				return nil
			}

			scans := scanner.AggregateEvents(events)
			fmt.Printf("Found %d events, aggregated into %d scans\n", len(events), len(scans))

			for _, scan := range scans {
				if err := scanner.SaveScan(&scan); err != nil {
					fmt.Printf("Warning: failed to save scan %s: %v\n", scan.ID, err)
					continue
				}
				id := scan.ID
				if len(id) > 8 {
					id = id[:8]
				}
				fmt.Printf("Saved scan %s (%d events, %d tokens)\n", id, len(scan.Events), scan.TotalTokens)
			}

			return nil
		},
	}
}
