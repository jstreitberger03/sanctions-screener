package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jstreitberger03/sanctions-screener/pkg/ingest"
	"github.com/jstreitberger03/sanctions-screener/pkg/models"
	"github.com/jstreitberger03/sanctions-screener/pkg/screening"
)

var (
	screeningName string
	screeningFile string
)

var screenCmd = &cobra.Command{
	Use:   "screen",
	Short: "Screen a name or file against sanctions lists",
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := ingest.NewStore(dbPath)
		if err != nil {
			return fmt.Errorf("open store: %w", err)
		}
		defer store.Close()

		var allPersons []models.Person
		lt := lists
		if len(lt) == 0 {
			lt = []string{"OFAC", "EU", "UN"}
		}
		allPersons, err = store.LoadLists(lt)
		if err != nil {
			return fmt.Errorf("load lists %v: %w", lt, err)
		}

		if screeningFile != "" {
			return screenFile(screeningFile, allPersons)
		}

		if screeningName == "" {
			return fmt.Errorf("either --name or --file is required")
		}

		return screenName(screeningName, allPersons)
	},
}

func screenName(name string, persons []models.Person) error {
	matches := screening.Screen(name, persons, threshold)

	if output != "" {
		return writeResults(matches, output)
	}

	for _, m := range matches {
		fmt.Printf("[%.2f] %s (%s) — %s\n", m.Score, m.Person.Name, m.MatchType, m.Person.ListType)
	}

	fmt.Printf("\n%d matches found (threshold: %.2f)\n", len(matches), threshold)
	return nil
}

func screenFile(path string, persons []models.Person) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	reader := csv.NewReader(f)
	records, err := reader.ReadAll()
	if err != nil {
		return fmt.Errorf("read csv: %w", err)
	}

	var allMatches []models.Match
	screenedCount := 0
	for i, row := range records {
		if len(row) == 0 {
			continue
		}
		// Skip the header row if it looks like one.
		if i == 0 && isHeaderRow(row[0]) {
			continue
		}
		screenedCount++
		matches := screening.Screen(row[0], persons, threshold)
		allMatches = append(allMatches, matches...)
	}

	if output != "" {
		return writeResults(allMatches, output)
	}

	for _, m := range allMatches {
		fmt.Printf("[%.2f] %s matched %s (%s)\n", m.Score, m.InputName, m.Person.Name, m.MatchType)
	}

	fmt.Printf("\n%d total matches from %d names\n", len(allMatches), screenedCount)
	return nil
}

// isHeaderRow returns true if the cell looks like a CSV header (common column names).
func isHeaderRow(cell string) bool {
	norm := strings.ToLower(strings.TrimSpace(cell))
	switch norm {
	case "name", "full_name", "fullname", "entity_name":
		return true
	}
	return false
}

func writeResults(matches []models.Match, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer f.Close()

	encoder := json.NewEncoder(f)
	encoder.SetIndent("", "  ")
	return encoder.Encode(matches)
}
