package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	threshold float64
	lists     []string
	output    string
	port      int
	config    string
	dbPath    string
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "screener",
	Short: "Sanctions screening tool",
	Long:  "screener is a CLI tool for screening names and transactions against OFAC/EU/UN sanctions lists.",
}

func init() {
	rootCmd.AddCommand(screenCmd)
	rootCmd.AddCommand(ingestCmd)
	rootCmd.AddCommand(serveCmd)
	rootCmd.AddCommand(versionCmd)

	screenCmd.Flags().StringVarP(&screeningName, "name", "n", "", "Name to screen")
	screenCmd.Flags().StringVarP(&screeningFile, "file", "f", "", "CSV file to bulk screen")
	screenCmd.Flags().Float64VarP(&threshold, "threshold", "t", 0.8, "Match threshold (0.0-1.0)")
	screenCmd.Flags().StringSliceVarP(&lists, "list", "l", nil, "Sanctions lists to check (default: all available)")
	screenCmd.Flags().StringVarP(&output, "output", "o", "", "Output file for results")

	ingestCmd.Flags().StringVarP(&ingestSource, "source", "s", "ofac", "Source: ofac, eu, json")
	ingestCmd.Flags().StringVarP(&ingestData, "data", "d", "", "Path to sanctions data file")

	serveCmd.Flags().IntVarP(&port, "port", "p", 8080, "API server port")
	serveCmd.Flags().StringVarP(&config, "config", "c", "", "Config file path")

	rootCmd.PersistentFlags().StringVar(&dbPath, "db", "sanctions.db", "Path to SQLite database")
}
