package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"

	"github.com/jstreitberger03/sanctions-screener/pkg/ingest"
	"github.com/jstreitberger03/sanctions-screener/pkg/models"
	"github.com/jstreitberger03/sanctions-screener/pkg/screening"
	"github.com/spf13/cobra"
)

var (
	screeningName string
	screeningFile string
)

var screenCmd = &cobra.Command{
	Use:   "screen",
	Short: "Screen a name or file against sanctions lists",
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := ingest.NewStore("sanctions.db")
		if err != nil {
			return fmt.Errorf("open store: %w", err)
		}
		defer store.Close()

		var allPersons []models.Person
		lt := lists
		if len(lt) == 0 {
			lt = []string{"OFAC", "EU", "UN"}
		}
		for _, l := range lt {
			persons, err := store.LoadCached(models.ListType(l))
			if err != nil {
				return fmt.Errorf("load list %s: %w", l, err)
			}
			allPersons = append(allPersons, persons...)
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
	for _, row := range records {
		if len(row) == 0 {
			continue
		}
		matches := screening.Screen(row[0], persons, threshold)
		allMatches = append(allMatches, matches...)
	}

	if output != "" {
		return writeResults(allMatches, output)
	}

	for _, m := range allMatches {
		fmt.Printf("[%.2f] %s matched %s (%s)\n", m.Score, m.InputName, m.Person.Name, m.MatchType)
	}

	fmt.Printf("\n%d total matches from %d names\n", len(allMatches), len(records))
	return nil
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
