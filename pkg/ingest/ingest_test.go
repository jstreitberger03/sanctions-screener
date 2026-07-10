package ingest

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/jstreitberger03/sanctions-screener/pkg/models"
)

func tempDB(tb testing.TB) (*Store, func()) {
	tb.Helper()
	dir := tb.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		tb.Fatalf("NewStore: %v", err)
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
			ID:       "3",
			Name:     "Alias Person",
			Aliases:  []string{"Alias One", "Alias Two"},
			Roles:    []string{"individual"},
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

func BenchmarkLoadCached(b *testing.B) {
	store, cleanup := tempDB(b)
	defer cleanup()

	persons := benchPersons(500)
	if err := store.cache(persons); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for b.Loop() {
		_, err := store.LoadCached(models.ListOFAC)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCache(b *testing.B) {
	persons := benchPersons(100)

	for b.Loop() {
		store, cleanup := tempDB(b)
		err := store.cache(persons)
		if err != nil {
			b.Fatal(err)
		}
		cleanup()
	}
}

func benchPersons(n int) []models.Person {
	persons := make([]models.Person, n)
	for i := range n {
		persons[i] = models.Person{
			ID:          fmt.Sprintf("BENCH-%d", i),
			Name:        fmt.Sprintf("Benchmark Person %d", i),
			Aliases:     []string{fmt.Sprintf("Alias %d", i), fmt.Sprintf("AKA %d", i)},
			Roles:       []string{"individual"},
			Nationality: "XX",
			ListType:    models.ListOFAC,
		}
	}
	return persons
}

// BenchmarkImportEU measures the full Read→Parse→Cache pipeline
// using the real 100-entry EU sample in OpenSanctions format.
// Exercises: os.ReadFile → sanctions.Load (parseJSON/openSanctionsToPerson) → cache with transaction.
func BenchmarkImportEU(b *testing.B) {
	for b.Loop() {
		store, cleanup := tempDB(b)
		_, err := store.ImportEU("../../data/eu_sample.json")
		if err != nil {
			b.Fatal(err)
		}
		cleanup()
	}
}
