package main

import (
	"fmt"

	"github.com/jstreitberger03/sanctions-screener/pkg/ingest"
	"github.com/jstreitberger03/sanctions-screener/pkg/models"
	"github.com/spf13/cobra"
)

var (
	ingestSource string
	ingestData   string
)

var ingestCmd = &cobra.Command{
	Use:   "ingest",
	Short: "Import sanctions list data",
	RunE: func(cmd *cobra.Command, args []string) error {
		if ingestData == "" {
			return fmt.Errorf("--data is required")
		}

		store, err := ingest.NewStore(dbPath)
		if err != nil {
			return fmt.Errorf("open store: %w", err)
		}
		defer store.Close()

		var persons []models.Person
		switch ingestSource {
		case "ofac":
			persons, err = store.ImportOFAC(ingestData)
		case "eu":
			persons, err = store.ImportEU(ingestData)
		case "jsonl":
			persons, err = store.ImportJSONL(ingestData)
		case "json":
			persons, err = store.ImportJSON(ingestData)
		default:
			return fmt.Errorf("unknown source: %s (use ofac, eu, json, jsonl)", ingestSource)
		}

		if err != nil {
			return fmt.Errorf("ingest: %w", err)
		}

		fmt.Printf("Imported %d entries from %s\n", len(persons), ingestSource)
		return nil
	},
}
