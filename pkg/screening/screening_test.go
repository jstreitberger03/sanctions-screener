package screening_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/jstreitberger03/sanctions-screener/pkg/ingest"
	"github.com/jstreitberger03/sanctions-screener/pkg/models"
	"github.com/jstreitberger03/sanctions-screener/pkg/screening"
)

func testList() []models.Person {
	return []models.Person{
		{ID: "SDN-001", Name: "Mohammed Al-Rashid", ListType: models.ListOFAC, Nationality: "SY"},
		{ID: "SDN-002", Name: "John Smith", ListType: models.ListOFAC, Nationality: "US"},
		{ID: "SDN-003", Name: "Viktor Ivanov", Aliases: []string{"Viktor Ivanovich"}, ListType: models.ListOFAC, Nationality: "RU"},
		{ID: "SDN-004", Name: "François Dupont", ListType: models.ListEU, Nationality: "FR"},
	}
}

func TestExactMatch(t *testing.T) {
	matches := screening.Screen("John Smith", testList(), 0.8)
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
	if matches[0].Score != 1.0 {
		t.Errorf("expected score 1.0, got %.2f", matches[0].Score)
	}
	if matches[0].MatchType != "exact" {
		t.Errorf("expected exact match, got %s", matches[0].MatchType)
	}
}

func TestFuzzyMatch(t *testing.T) {
	matches := screening.Screen("Johnson Smith", testList(), 0.7)
	if len(matches) == 0 {
		t.Fatal("expected at least 1 fuzzy match")
	}
	if matches[0].MatchType != "fuzzy" {
		t.Errorf("expected fuzzy match, got %s", matches[0].MatchType)
	}
}

func TestAliasMatch(t *testing.T) {
	matches := screening.Screen("Viktor Ivanovich", testList(), 0.8)
	if len(matches) == 0 {
		t.Fatal("expected match on alias")
	}
}

func TestNoMatchBelowThreshold(t *testing.T) {
	matches := screening.Screen("Completely Different Name", testList(), 0.95)
	if len(matches) != 0 {
		t.Errorf("expected 0 matches, got %d", len(matches))
	}
}

func TestDiacriticInsensitive(t *testing.T) {
	matches := screening.Screen("Francois Dupont", testList(), 0.8)
	if len(matches) == 0 {
		t.Fatal("expected match despite different diacritics")
	}
}

func TestResultSortedByScore(t *testing.T) {
	persons := []models.Person{
		{ID: "1", Name: "John Smith", ListType: models.ListOFAC},
		{ID: "2", Name: "John Smyth", ListType: models.ListOFAC},
	}
	matches := screening.Screen("John Smith", persons, 0.7)
	if len(matches) < 2 {
		t.Skip("not enough matches for sort test")
	}
	if matches[0].Score < matches[1].Score {
		t.Errorf("results not sorted by score descending")
	}
}

// Exact match must return even at the maximum threshold.
func TestExactMatchWithMaxThreshold(t *testing.T) {
	matches := screening.Screen("John Smith", testList(), 1.0)
	if len(matches) != 1 {
		t.Fatalf("expected 1 exact match at threshold 1.0, got %d", len(matches))
	}
	if matches[0].MatchType != "exact" {
		t.Errorf("expected exact match, got %s", matches[0].MatchType)
	}
	if matches[0].Score != 1.0 {
		t.Errorf("expected score 1.0, got %.2f", matches[0].Score)
	}
}

// Exact match must be based on string equality after normalization,
// not on Jaro-Winkler returning 1.0 for similar-but-different strings.
func TestExactMatchIsStringEquality(t *testing.T) {
	// "Francois Dupont" normalizes to "francois dupont"
	// "François Dupont" normalizes to "francois dupont"
	// These are string-equal after normalization → exact match.
	matches := screening.Screen("Francois Dupont", testList(), 0.8)
	if len(matches) == 0 {
		t.Fatal("expected match for diacritic-insensitive exact match")
	}
	if matches[0].MatchType != "exact" {
		t.Errorf("expected exact match (string equality after normalization), got %s", matches[0].MatchType)
	}
	if matches[0].Score != 1.0 {
		t.Errorf("expected score 1.0 for exact match, got %.2f", matches[0].Score)
	}
}

// Alias match must be returned when the threshold allows it.
func TestAliasMatchWithinThreshold(t *testing.T) {
	persons := []models.Person{
		{ID: "1", Name: "Viktor Ivanov", Aliases: []string{"Viktor Ivanovich"}, ListType: models.ListOFAC},
	}
	matches := screening.Screen("Viktor Ivanovich", persons, 0.8)
	if len(matches) != 1 {
		t.Fatalf("expected 1 alias match at threshold 0.8, got %d", len(matches))
	}
	if matches[0].MatchType != "alias" {
		t.Errorf("expected alias match, got %s", matches[0].MatchType)
	}
	if matches[0].Score != 0.95 {
		t.Errorf("expected score 0.95, got %.2f", matches[0].Score)
	}
}

// Alias match must be blocked when the threshold is higher than the alias score.
func TestAliasMatchBlockedByHighThreshold(t *testing.T) {
	persons := []models.Person{
		{ID: "1", Name: "Viktor Ivanov", Aliases: []string{"Viktor Ivanovich"}, ListType: models.ListOFAC},
	}
	matches := screening.Screen("Viktor Ivanovich", persons, 1.0)
	if len(matches) != 0 {
		t.Fatalf("expected 0 matches at threshold 1.0 (alias at 0.95 blocked), got %d", len(matches))
	}
}

// When an alias exact match is blocked by threshold, the person must not
// reappear as a fuzzy match via Jaro-Winkler (which would score 1.0 on the
// identical strings). The early return nil must prevent fuzzy fallback.
func TestAliasBlockedByThresholdNoFuzzyFallback(t *testing.T) {
	// Person has an alias that matches exactly.
	// At threshold 1.0 the alias is blocked (0.95 < 1.0).
	// The person should not show up at all — not even as fuzzy.
	persons := []models.Person{
		{
			ID:       "1",
			Name:     "Viktor Ivanov",
			Aliases:  []string{"Viktor Ivanovich"},
			ListType: models.ListOFAC,
		},
	}
	matches := screening.Screen("Viktor Ivanovich", persons, 1.0)
	if len(matches) != 0 {
		t.Fatalf("alias blocked by threshold must not fall through to fuzzy: got %d matches", len(matches))
	}

	// Sanity check: same query at threshold 0.8 DOES return the alias match.
	matchesLow := screening.Screen("Viktor Ivanovich", persons, 0.8)
	if len(matchesLow) != 1 {
		t.Fatalf("sanity check failed: expected 1 alias match at threshold 0.8, got %d", len(matchesLow))
	}
	if matchesLow[0].MatchType != "alias" {
		t.Errorf("sanity check failed: expected alias match, got %s", matchesLow[0].MatchType)
	}
}

// Exact match on primary name must take precedence over alias match.
func TestExactBeatsAlias(t *testing.T) {
	persons := []models.Person{
		{
			ID:       "1",
			Name:     "John Smith",
			Aliases:  []string{"John Smith"},
			ListType: models.ListOFAC,
		},
	}
	matches := screening.Screen("John Smith", persons, 0.8)
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
	if matches[0].MatchType != "exact" {
		t.Errorf("primary name exact match must win over alias: got %s", matches[0].MatchType)
	}
	if matches[0].Score != 1.0 {
		t.Errorf("expected score 1.0 (exact), got %.2f", matches[0].Score)
	}
}

func BenchmarkScreen(b *testing.B) {
	list := testList()
	for b.Loop() {
		screening.Screen("Mohammed Al Rashid", list, 0.8)
	}
}

func BenchmarkScreenConcurrent(b *testing.B) {
	// Build a large list (500 persons) to trigger the concurrent path (>100).
	list := make([]models.Person, 500)
	for i := range 500 {
		list[i] = models.Person{
			ID:          fmt.Sprintf("B-%d", i),
			Name:        fmt.Sprintf("Person %d", i),
			ListType:    models.ListOFAC,
			Nationality: "XX",
		}
	}
	b.ResetTimer()
	for b.Loop() {
		screening.Screen("John Smith", list, 0.8)
	}
}

func BenchmarkScreenLarge(b *testing.B) {
	// Same 500-person list, sequential baseline for concurrent comparison.
	list := make([]models.Person, 500)
	for i := range 500 {
		list[i] = models.Person{
			ID:          fmt.Sprintf("B-%d", i),
			Name:        fmt.Sprintf("Person %d", i),
			ListType:    models.ListOFAC,
			Nationality: "XX",
		}
	}
	// Force sequential by setting Concurrency to 1.
	prev := screening.Concurrency
	screening.Concurrency = 1
	defer func() { screening.Concurrency = prev }()
	b.ResetTimer()
	for b.Loop() {
		screening.Screen("John Smith", list, 0.8)
	}
}

func TestConcurrentMatchesSequential(t *testing.T) {
	// Build a 150-person list to trigger the concurrent path (>100).
	list := make([]models.Person, 150)
	for i := range 150 {
		list[i] = models.Person{
			ID:          fmt.Sprintf("C-%d", i),
			Name:        fmt.Sprintf("Person %d", i),
			Aliases:     []string{fmt.Sprintf("Alias %d", i)},
			ListType:    models.ListOFAC,
			Nationality: "XX",
		}
	}
	// Insert an exact match to verify it's found by both paths.
	list[100] = models.Person{
		ID:          "MATCH",
		Name:        "John Smith",
		ListType:    models.ListOFAC,
		Nationality: "US",
	}

	concurrent := screening.Screen("John Smith", list, 0.8)

	prev := screening.Concurrency
	screening.Concurrency = 1
	sequential := screening.Screen("John Smith", list, 0.8)
	screening.Concurrency = prev

	if len(concurrent) != len(sequential) {
		t.Fatalf("concurrent returned %d matches, sequential returned %d",
			len(concurrent), len(sequential))
	}
	for i := range concurrent {
		if concurrent[i].Person.ID != sequential[i].Person.ID ||
			concurrent[i].Score != sequential[i].Score {
			t.Errorf("match %d differs: concurrent={%s,%.2f} sequential={%s,%.2f}",
				i, concurrent[i].Person.ID, concurrent[i].Score,
				sequential[i].Person.ID, sequential[i].Score)
		}
	}
}

// BenchmarkScreenFullDataset loads the full 5,885-entry EU sanctions list
// from a local JSONL file and screens one name against it. Skips when the
// file is not present (not shipped in the repo) or when running -short.
//
// Download the dataset first:
//   curl -o eu_sanctions.jsonl https://data.opensanctions.org/datasets/latest/eu_fsf/entities.ftm.json
func BenchmarkScreenFullDataset(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping full-dataset benchmark in short mode")
	}

	// Try common paths for the full dataset.
	candidates := []string{
		"../../eu_sanctions.jsonl",
		"../../data/eu_full.jsonl",
		"../../data/eu_sanctions.jsonl",
	}
	var dataPath string
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			dataPath = p
			break
		}
	}
	if dataPath == "" {
		b.Skip("full dataset not found. Download: curl -o eu_sanctions.jsonl https://data.opensanctions.org/datasets/latest/eu_fsf/entities.ftm.json")
	}

	// Import into a temp DB once, then screen in the loop.
	store, err := ingest.NewStore(filepath.Join(b.TempDir(), "full.db"))
	if err != nil {
		b.Fatal(err)
	}
	defer store.Close()

	persons, err := store.ImportJSONL(dataPath)
	if err != nil {
		b.Fatal(err)
	}
	b.Logf("loaded %d persons from %s", len(persons), dataPath)

	b.ResetTimer()
	for b.Loop() {
		screening.Screen("Irina Kostenko", persons, 0.8)
	}
}
