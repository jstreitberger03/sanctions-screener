package screening_test

import (
	"testing"

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

func BenchmarkScreen(b *testing.B) {
	list := testList()
	for b.Loop() {
		screening.Screen("Mohammed Al Rashid", list, 0.8)
	}
}
