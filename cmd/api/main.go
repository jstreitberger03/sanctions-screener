package main

import (
	"fmt"
	"os"
	"strconv"

	"github.com/jstreitberger03/sanctions-screener/internal/server"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

// run creates and starts the API server. Reads PORT and SCREENER_DB_PATH
// from the environment. Separated from main() for testability.
func run() error {
	port := 8080
	if p := os.Getenv("PORT"); p != "" {
		if v, err := strconv.Atoi(p); err == nil {
			port = v
		}
	}

	dbPath := "sanctions.db"
	if d := os.Getenv("SCREENER_DB_PATH"); d != "" {
		dbPath = d
	}

	srv, err := server.New(server.Config{
		Port:   port,
		DBPath: dbPath,
	})
	if err != nil {
		return err
	}
	fmt.Printf("Starting API server on :%d\n", port)
	return srv.ListenAndServe()
}
