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

func TestReIngestRemovesStaleRows(t *testing.T) {
	store, cleanup := tempDB(t)
	defer cleanup()

	first := []models.Person{
		{ID: "1", Name: "Person One", ListType: models.ListOFAC},
		{ID: "2", Name: "Person Two", ListType: models.ListOFAC},
		{ID: "3", Name: "Person Three", ListType: models.ListOFAC},
	}
	if err := store.cache(first); err != nil {
		t.Fatalf("first cache: %v", err)
	}

	// Re-ingest with fewer entries — ID 3 is removed from the source.
	second := []models.Person{
		{ID: "1", Name: "Person One", ListType: models.ListOFAC},
		{ID: "2", Name: "Person Two", ListType: models.ListOFAC},
	}
	if err := store.cache(second); err != nil {
		t.Fatalf("second cache: %v", err)
	}

	loaded, err := store.LoadCached(models.ListOFAC)
	if err != nil {
		t.Fatalf("LoadCached: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("expected 2 persons after re-ingest, got %d", len(loaded))
	}
	ids := make(map[string]bool, len(loaded))
	for _, p := range loaded {
		ids[p.ID] = true
	}
	if ids["3"] {
		t.Error("stale row ID 3 should have been removed on re-ingest")
	}
}

func TestReIngestDoesNotAffectOtherLists(t *testing.T) {
	store, cleanup := tempDB(t)
	defer cleanup()

	if err := store.cache([]models.Person{
		{ID: "O1", Name: "OFAC Person", ListType: models.ListOFAC},
		{ID: "O2", Name: "OFAC Person 2", ListType: models.ListOFAC},
		{ID: "E1", Name: "EU Person", ListType: models.ListEU},
	}); err != nil {
		t.Fatalf("initial cache: %v", err)
	}

	// Re-ingest OFAC with fewer entries.
	if err := store.cache([]models.Person{
		{ID: "O1", Name: "OFAC Person", ListType: models.ListOFAC},
	}); err != nil {
		t.Fatalf("re-ingest OFAC: %v", err)
	}

	ofac, err := store.LoadCached(models.ListOFAC)
	if err != nil {
		t.Fatalf("LoadCached OFAC: %v", err)
	}
	if len(ofac) != 1 {
		t.Errorf("expected 1 OFAC person, got %d", len(ofac))
	}

	eu, err := store.LoadCached(models.ListEU)
	if err != nil {
		t.Fatalf("LoadCached EU: %v", err)
	}
	if len(eu) != 1 {
		t.Errorf("expected 1 EU person (unchanged), got %d", len(eu))
	}
}

func TestDSNIncludesBusyTimeout(t *testing.T) {
	store, cleanup := tempDB(t)
	defer cleanup()

	var val int
	if err := store.db.QueryRow("PRAGMA busy_timeout").Scan(&val); err != nil {
		t.Fatalf("query busy_timeout: %v", err)
	}
	if val != 5000 {
		t.Errorf("expected busy_timeout=5000, got %d", val)
	}

	// Pool tuning: CGO SQLite should serialize via a single connection.
	stats := store.db.Stats()
	if stats.MaxOpenConnections != 1 {
		t.Errorf("expected MaxOpenConnections=1, got %d", stats.MaxOpenConnections)
	}
}

func TestConcurrentReadWriteNoBusy(t *testing.T) {
	store, cleanup := tempDB(t)
	defer cleanup()

	// Seed initial data.
	if err := store.cache([]models.Person{
		{ID: "R1", Name: "Reader Test", ListType: models.ListOFAC},
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	done := make(chan error, 2)

	// Writer goroutine.
	go func() {
		for i := range 50 {
			err := store.cache([]models.Person{
				{ID: fmt.Sprintf("W%d", i), Name: fmt.Sprintf("Writer %d", i), ListType: models.ListOFAC},
			})
			if err != nil {
				done <- err
				return
			}
		}
		done <- nil
	}()

	// Reader goroutine.
	go func() {
		for range 50 {
			_, err := store.LoadCached(models.ListOFAC)
			if err != nil {
				done <- err
				return
			}
		}
		done <- nil
	}()

	for range 2 {
		if err := <-done; err != nil {
			t.Fatalf("concurrent read/write error: %v", err)
		}
	}
}

func TestIndexExistsAfterNewStore(t *testing.T) {
	store, cleanup := tempDB(t)
	defer cleanup()

	var name string
	if err := store.db.QueryRow(
		"SELECT name FROM sqlite_master WHERE type='index' AND name='idx_sanctions_list_type'",
	).Scan(&name); err != nil {
		t.Fatalf("index idx_sanctions_list_type not found: %v", err)
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
