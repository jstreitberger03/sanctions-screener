package ingest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jstreitberger03/sanctions-screener/pkg/models"
)

func tempDB(t *testing.T) (*Store, func()) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	return store, func() {
		store.Close()
		os.Remove(dbPath)
	}
}

func TestStore_CacheAndLoad(t *testing.T) {
	store, cleanup := tempDB(t)
	defer cleanup()

	persons := []models.Person{
		{ID: "1", Name: "Test Person", ListType: models.ListOFAC, Nationality: "US"},
		{ID: "2", Name: "Another Person", ListType: models.ListOFAC, Nationality: "GB"},
	}

	if err := store.cache(persons); err != nil {
		t.Fatalf("cache: %v", err)
	}

	loaded, err := store.LoadCached(models.ListOFAC)
	if err != nil {
		t.Fatalf("LoadCached: %v", err)
	}

	if len(loaded) != 2 {
		t.Fatalf("expected 2 persons, got %d", len(loaded))
	}
	if loaded[0].Name != "Test Person" {
		t.Errorf("expected 'Test Person', got %q", loaded[0].Name)
	}
}

func TestStore_LoadCached_Empty(t *testing.T) {
	store, cleanup := tempDB(t)
	defer cleanup()

	loaded, err := store.LoadCached(models.ListUN)
	if err != nil {
		t.Fatalf("LoadCached: %v", err)
	}
	if len(loaded) != 0 {
		t.Errorf("expected 0 persons, got %d", len(loaded))
	}
}

func TestStore_CacheWithAliasesAndDOB(t *testing.T) {
	store, cleanup := tempDB(t)
	defer cleanup()

	persons := []models.Person{
		{
			ID:      "3",
			Name:    "Alias Person",
			Aliases: []string{"Alias One", "Alias Two"},
			Roles:   []string{"individual"},
			ListType: models.ListEU,
		},
	}

	if err := store.cache(persons); err != nil {
		t.Fatalf("cache: %v", err)
	}

	loaded, err := store.LoadCached(models.ListEU)
	if err != nil {
		t.Fatalf("LoadCached: %v", err)
	}

	if len(loaded) != 1 {
		t.Fatalf("expected 1 person, got %d", len(loaded))
	}
	if len(loaded[0].Aliases) != 2 {
		t.Errorf("expected 2 aliases, got %d", len(loaded[0].Aliases))
	}
	if loaded[0].Aliases[0] != "Alias One" {
		t.Errorf("expected 'Alias One', got %q", loaded[0].Aliases[0])
	}
}

func TestStore_Close(t *testing.T) {
	store, cleanup := tempDB(t)
	cleanup()

	if err := store.cache([]models.Person{{ID: "4", Name: "X", ListType: models.ListOFAC}}); err == nil {
		t.Error("expected error after close")
	}
}
