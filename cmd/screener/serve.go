package main

import (
	"fmt"

	"github.com/jstreitberger03/sanctions-screener/internal/server"
	"github.com/spf13/cobra"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the screening API server",
	RunE: func(cmd *cobra.Command, args []string) error {
		srv, err := server.New(server.Config{
			Port:   port,
			DBPath: "sanctions.db",
		})
		if err != nil {
			return err
		}
		fmt.Printf("Starting API server on :%d\n", port)
		return srv.ListenAndServe()
	},
}
