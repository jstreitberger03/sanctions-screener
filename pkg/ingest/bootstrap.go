package ingest

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/jstreitberger03/sanctions-screener/pkg/models"
)

// EUDatasetURL is the canonical OpenSanctions FTM JSONL export for the EU
// consolidated sanctions list. Used by the setup command and auto-bootstrap.
const EUDatasetURL = "https://data.opensanctions.org/datasets/latest/eu_fsf/entities.ftm.json"

// DownloadAndIngest fetches a sanctions dataset from sourceURL, saves it to a
// temp file, ingests it into the store with the given listType, and removes
// the temp file. Returns the number of imported entries.
func DownloadAndIngest(store *Store, sourceURL string, listType models.ListType) (int, error) {
	tmpFile, err := os.CreateTemp("", "screener-ingest-*.jsonl")
	if err != nil {
		return 0, fmt.Errorf("create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	log.Printf("Downloading sanctions data from %s ...", sourceURL)

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(sourceURL)
	if err != nil {
		return 0, fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("download returned HTTP %d", resp.StatusCode)
	}

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		return 0, fmt.Errorf("write temp file: %w", err)
	}
	if err := tmpFile.Sync(); err != nil {
		return 0, fmt.Errorf("sync temp file: %w", err)
	}

	log.Printf("Downloaded to %s, ingesting as %s ...", tmpFile.Name(), listType)
	persons, err := store.ImportJSONLWithType(tmpFile.Name(), listType)
	if err != nil {
		return 0, fmt.Errorf("ingest: %w", err)
	}
	return len(persons), nil
}

// AutoBootstrap checks whether the store already has EU sanctions data. If
// the EU list is empty, it downloads and ingests the EU sanctions list from
// OpenSanctions so the server can serve useful results immediately without
// manual setup.
//
// When the store already contains EU data (even if other lists are empty),
// AutoBootstrap is a no-op — it only bootstraps the list it can auto-download.
func AutoBootstrap(store *Store) {
	persons, err := store.LoadCached(models.ListEU)
	if err == nil && len(persons) > 0 {
		log.Printf("Auto-bootstrap: store already contains %d EU entries, skipping download", len(persons))
		return
	}

	log.Println("Auto-bootstrap: empty store detected, downloading EU sanctions list ...")
	n, err := DownloadAndIngest(store, EUDatasetURL, models.ListEU)
	if err != nil {
		log.Printf("Auto-bootstrap: WARNING — failed to download EU list: %v", err)
		log.Println("Auto-bootstrap: server will start with an empty database. Run 'screener setup' or 'screener ingest' manually.")
		return
	}
	log.Printf("Auto-bootstrap: successfully ingested %d EU entries", n)
}
