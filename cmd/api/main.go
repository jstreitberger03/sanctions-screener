package main

import (
	"fmt"
	"log"
	"os"

	"github.com/jstreitberger03/sanctions-screener/internal/server"
)

func main() {
	port := 8080
	dbPath := "sanctions.db"

	if p := os.Getenv("PORT"); p != "" {
		fmt.Sscanf(p, "%d", &port)
	}
	if d := os.Getenv("SCREENER_DB_PATH"); d != "" {
		dbPath = d
	}

	srv := server.New(server.Config{
		Port:   port,
		DBPath: dbPath,
	})

	log.Printf("screener API listening on :%d\n", port)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
