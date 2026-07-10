package main

import (
	"fmt"
	"log"
	"os"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/jstreitberger03/sanctions-screener/internal/server"
	"github.com/jstreitberger03/sanctions-screener/pkg/ingest"
)

type appConfig struct {
	Server struct {
		Port   int    `yaml:"port"`
		DBPath string `yaml:"db_path"`
	} `yaml:"server"`
	Screening struct {
		DefaultThreshold float64 `yaml:"default_threshold"`
	} `yaml:"screening"`
}

func loadConfig(path string) (*appConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg appConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return &cfg, nil
}

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the screening API server",
	RunE: func(cmd *cobra.Command, args []string) error {
		if config != "" {
			cfg, err := loadConfig(config)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			if cfg.Server.Port != 0 && port == 8080 {
				port = cfg.Server.Port
			}
			if cfg.Server.DBPath != "" && dbPath == "sanctions.db" {
				dbPath = cfg.Server.DBPath
			}
			if cfg.Screening.DefaultThreshold != 0 && threshold == 0.8 {
				threshold = cfg.Screening.DefaultThreshold
			}
		}

		// Auto-bootstrap: if the database is empty, download and ingest
		// the EU sanctions list so the server is ready immediately.
		bootstrapStore, err := ingest.NewStore(dbPath)
		if err != nil {
			log.Printf("Auto-bootstrap: cannot open store for check: %v", err)
		} else {
			ingest.AutoBootstrap(bootstrapStore)
			bootstrapStore.Close()
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
	},
}
