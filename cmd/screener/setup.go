package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/jstreitberger03/sanctions-screener/pkg/ingest"
	"github.com/jstreitberger03/sanctions-screener/pkg/models"
)

var (
	setupSource string
	setupData   string
	setupURL    string
)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Download and ingest default sanctions lists for a ready-to-use deployment",
	Long: `setup downloads the EU sanctions list from OpenSanctions and ingests it
into the local database. Optionally also ingests a local OFAC CSV file.

After setup, the server is immediately ready to serve screening requests without
any further manual data loading.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := ingest.NewStore(dbPath)
		if err != nil {
			return fmt.Errorf("open store: %w", err)
		}
		defer store.Close()

		switch setupSource {
		case "eu":
			url := ingest.EUDatasetURL
			if setupURL != "" {
				url = setupURL
			}
			n, err := ingest.DownloadAndIngest(store, url, models.ListEU)
			if err != nil {
				return fmt.Errorf("setup EU: %w", err)
			}
			fmt.Printf("✓ Downloaded and ingested %d EU sanctions entries\n", n)

		case "ofac":
			if setupData == "" {
				return fmt.Errorf("--data is required for OFAC (local CSV file path)")
			}
			persons, err := store.ImportOFAC(setupData)
			if err != nil {
				return fmt.Errorf("setup OFAC: %w", err)
			}
			fmt.Printf("✓ Ingested %d OFAC entries from %s\n", len(persons), setupData)

		case "all":
			// Download EU
			url := ingest.EUDatasetURL
			if setupURL != "" {
				url = setupURL
			}
			n, err := ingest.DownloadAndIngest(store, url, models.ListEU)
			if err != nil {
				return fmt.Errorf("setup EU: %w", err)
			}
			fmt.Printf("✓ Downloaded and ingested %d EU sanctions entries\n", n)

			// Ingest OFAC if data file provided
			if setupData != "" {
				persons, err := store.ImportOFAC(setupData)
				if err != nil {
					return fmt.Errorf("setup OFAC: %w", err)
				}
				fmt.Printf("✓ Ingested %d OFAC entries from %s\n", len(persons), setupData)
			} else {
				fmt.Println("ℹ OFAC data file not provided (use --data for local CSV). Skipping OFAC.")
			}

		default:
			return fmt.Errorf("unknown source: %s (use eu, ofac, all)", setupSource)
		}

		fmt.Println("\nSetup complete. The server is ready: screener serve")
		return nil
	},
}
