package screening_test

import (
	"sort"
	"testing"

	"github.com/jstreitberger03/sanctions-screener/pkg/models"
	"github.com/jstreitberger03/sanctions-screener/pkg/screening"
)

func indexTestList() []models.Person {
	return []models.Person{
		{ID: "SDN-001", Name: "John Smith", ListType: models.ListOFAC, Nationality: "US"},
		{ID: "SDN-002", Name: "Mohammed Al-Rashid", ListType: models.ListOFAC, Nationality: "SY"},
		{ID: "SDN-003", Name: "Viktor Ivanov", Aliases: []string{"Viktor Ivanovich"}, ListType: models.ListOFAC, Nationality: "RU"},
		{ID: "SDN-004", Name: "Иван Иванов", ListType: models.ListOFAC, Nationality: "RU"},
		{ID: "SDN-005", Name: "", ListType: models.ListOFAC},
	}
}

func matchSetKey(m models.Match) string {
	return m.Person.ID + ":" + string(m.MatchType) + ":" + formatScore(m.Score)
}

func formatScore(f float64) string {
	return "" // placeholder
}

// matchSetsEqual compares two match slices by Person.ID + MatchType + Score
// (within a small tolerance). Returns true if both have the same match sets.
func matchSetsEqual(seq []models.Match, matches []models.Match, threshold float64) bool {
	if len(seq) != len(matches) {
		return false
	}
	type key struct {
		id    string
		mt    models.MatchType
		score float64
	}
	scoresToKey := func(m models.Match) key {
		return key{m.Person.ID, m.MatchType, float64(int(m.Score*10000)) / 10000}
	}
	s1 := make([]key, len(seq))
	s2 := make([]key, len(matches))
	for i, m := range seq {
		s1[i] = scoresToKey(m)
	}
	for i, m := range matches {
		s2[i] = scoresToKey(m)
	}
	sort.Slice(s1, func(i, j int) bool { return s1[i].id+string(s1[i].mt) < s1[j].id+string(s1[j].mt) })
	sort.Slice(s2, func(i, j int) bool { return s2[i].id+string(s2[i].mt) < s2[j].id+string(s2[j].mt) })
	for i := range s1 {
		if s1[i] != s2[i] {
			return false
		}
	}
	return true
}

func TestBuildIndex_NormalizesEntries(t *testing.T) {
	list := []models.Person{
		{ID: "1", Name: "John Smith", Aliases: []string{"Johnny"}, ListType: models.ListOFAC},
		{ID: "2", Name: "Иван Иванов", ListType: models.ListOFAC},
	}
	idx := screening.BuildIndex(list)
	if idx == nil {
		t.Fatal("BuildIndex returned nil")
	}
	// We can't inspect internal Entry fields directly (unexported), but
	// ScreenIndex will use them. Verify via a query that uses normalized data.
	matches := screening.ScreenIndex("john smith", idx, 0.8)
	if len(matches) != 1 || matches[0].Person.ID != "1" {
		t.Fatalf("expected exact match for ID 1, got %+v", matches)
	}
	if matches[0].MatchType != "exact" {
		t.Errorf("expected exact match, got %s", matches[0].MatchType)
	}
}

func TestBuildIndex_TokensInserted(t *testing.T) {
	// Verify the index produces correct candidates by querying names that
	// share token prefixes. "jo" should match both "John" and "Jon".
	list := []models.Person{
		{ID: "1", Name: "John Smith", ListType: models.ListOFAC},
		{ID: "2", Name: "Jon Smyth", ListType: models.ListOFAC},
		{ID: "3", Name: "Alice Brown", ListType: models.ListOFAC},
	}
	idx := screening.BuildIndex(list)

	// Query "Johnson Smith" → should find John Smith and Jon Smyth via "jo"
	// and "sm" token overlap, but NOT Alice Brown.
	matches := screening.ScreenIndex("Johnson Smith", idx, 0.0)
	ids := make(map[string]bool)
	for _, m := range matches {
		ids[m.Person.ID] = true
	}
	if !ids["1"] {
		t.Error("expected ID 1 (John Smith) in candidates")
	}
	if !ids["2"] {
		t.Error("expected ID 2 (Jon Smyth) in candidates")
	}
	if ids["3"] {
		t.Error("ID 3 (Alice Brown) should not be a candidate (no shared tokens)")
	}
}

func TestScreenIndex_ExactMatchFound(t *testing.T) {
	idx := screening.BuildIndex(indexTestList())
	matches := screening.ScreenIndex("John Smith", idx, 0.8)
	if len(matches) != 1 {
		t.Fatalf("expected 1 exact match, got %d: %+v", len(matches), matches)
	}
	if matches[0].Person.ID != "SDN-001" {
		t.Errorf("wrong person: %s", matches[0].Person.ID)
	}
	if matches[0].MatchType != "exact" {
		t.Errorf("expected exact, got %s", matches[0].MatchType)
	}
}

func TestScreenIndex_FuzzyMatchFound(t *testing.T) {
	idx := screening.BuildIndex(indexTestList())
	matches := screening.ScreenIndex("Jon Smith", idx, 0.7)
	if len(matches) == 0 {
		t.Fatal("expected a fuzzy match for 'Jon Smith'")
	}
	found := false
	for _, m := range matches {
		if m.Person.ID == "SDN-001" {
			found = true
			if m.MatchType != "fuzzy" {
				t.Errorf("expected fuzzy, got %s", m.MatchType)
			}
		}
	}
	if !found {
		t.Error("expected to find SDN-001 (John Smith) as a fuzzy match")
	}
}

func TestScreenIndex_AliasMatchFound(t *testing.T) {
	idx := screening.BuildIndex(indexTestList())
	matches := screening.ScreenIndex("Viktor Ivanovich", idx, 0.8)
	if len(matches) == 0 {
		t.Fatal("expected an alias match for 'Viktor Ivanovich'")
	}
	found := false
	for _, m := range matches {
		if m.Person.ID == "SDN-003" {
			found = true
			if m.MatchType != "alias" {
				t.Errorf("expected alias, got %s", m.MatchType)
			}
		}
	}
	if !found {
		t.Error("expected to find SDN-003 (Viktor Ivanov) via alias")
	}
}

// TestScreenIndex_NoFalseNegatives is the hard recall gate. It compares the
// match sets from the old O(n) Screen vs the new index-based ScreenIndex
// across every existing test scenario in screening_test.go and
// screening_edge_test.go. If the token prefilter drops any true match that
// Screen would have found, this test fails.
func TestScreenIndex_NoFalseNegatives(t *testing.T) {
	displayThreshold := 0.8 // short-name alias to avoid shadowing in closures

	cases := []struct {
		name      string
		list      []models.Person
		query     string
		threshold float64
	}{
		// From screening_test.go testList()
		{"exact match", testList(), "John Smith", 0.8},
		{"fuzzy match", testList(), "Johnson Smith", 0.7},
		{"alias match", testList(), "Viktor Ivanovich", 0.8},
		{"no match high threshold", testList(), "Completely Different Name", 0.95},
		{"diacritic insensitive", testList(), "Francois Dupont", 0.8},
		{"exact at max threshold", testList(), "John Smith", 1.0},
		{"exact is string equality", testList(), "Francois Dupont", 0.8},
		{"alias within threshold", []models.Person{{ID: "1", Name: "Viktor Ivanov", Aliases: []string{"Viktor Ivanovich"}, ListType: models.ListOFAC}}, "Viktor Ivanovich", 0.8},
		{"alias blocked by high threshold", []models.Person{{ID: "1", Name: "Viktor Ivanov", Aliases: []string{"Viktor Ivanovich"}, ListType: models.ListOFAC}}, "Viktor Ivanovich", 1.0},
		{"alias blocked no fuzzy fallback", []models.Person{{ID: "1", Name: "Viktor Ivanov", Aliases: []string{"Viktor Ivanovich"}, ListType: models.ListOFAC}}, "Viktor Ivanovich", 1.0},
		{"exact beats alias", []models.Person{{ID: "1", Name: "John Smith", Aliases: []string{"John Smith"}, ListType: models.ListOFAC}}, "John Smith", 0.8},
		{"two same named", []models.Person{{ID: "OFAC-1", Name: "John Smith", ListType: models.ListOFAC}, {ID: "EU-2", Name: "John Smith", ListType: models.ListEU}}, "John Smith", 0.8},

		// From screening_edge_test.go
		{"cyrillic exact", []models.Person{{ID: "X-1", Name: "Иван Иванов", ListType: models.ListOFAC}}, "иван иванов", 0.8},
		{"cyrillic vs latin no match", []models.Person{{ID: "X-1", Name: "Владимир Путин", ListType: models.ListOFAC}}, "Vladimir Putin", 0.8},
		{"cjk exact", []models.Person{{ID: "X-1", Name: "李明", ListType: models.ListOFAC}}, "李明", 0.8},
		{"cjk vs pinyin no match", []models.Person{{ID: "X-1", Name: "李明", ListType: models.ListOFAC}}, "Li Ming", 0.8},
		{"arabic exact", []models.Person{{ID: "X-1", Name: "محمد الرشيد", ListType: models.ListOFAC}}, "محمد الرشيد", 0.8},
		{"nfd folds to nfc", []models.Person{{ID: "X-1", Name: "Café", ListType: models.ListOFAC}, {ID: "X-2", Name: "John Smith", ListType: models.ListOFAC}}, "Cafe\u0301", 0.8},
		{"nfd list vs nfc query", []models.Person{{ID: "Y-1", Name: "Cafe\u0301"}}, "Café", 0.8},
		{"multi decomposition points", []models.Person{{ID: "Z-1", Name: "Fančovič"}}, "Fanc\u030Covic\u030C", 0.8},
		{"apostrophe fuzzy", []models.Person{{ID: "X-1", Name: "O'Brien", ListType: models.ListOFAC}}, "OBrien", 0.8},
		{"hyphen fuzzy", []models.Person{{ID: "X-1", Name: "Jean-Paul Sartre", ListType: models.ListOFAC}}, "Jean Paul Sartre", 0.8},
		{"dot initials fuzzy", []models.Person{{ID: "X-1", Name: "John Smith", ListType: models.ListOFAC}}, "J. Smith", 0.8},
		{"reversed name no match", []models.Person{{ID: "1", Name: "Smith John", ListType: models.ListOFAC}}, "John Smith", 0.8},
		{"pure initials no match", []models.Person{{ID: "1", Name: "John Smith", ListType: models.ListOFAC}}, "JS", displayThreshold},
		{"empty query no match", testList(), "", 0.8},
		{"whitespace query no match", testList(), "   ", 0.8},
		{"empty name matches empty query", []models.Person{{ID: "EMPTY", Name: ""}}, "", 0.8},
		{"non-empty vs empty-name entry", []models.Person{{ID: "EMPTY", Name: ""}, {ID: "1", Name: "Mohammed Al-Rashid"}}, "Mohammed Al-Rashid", 0.8},
		{"zero threshold moderate", []models.Person{{ID: "1", Name: "John Smith", ListType: models.ListOFAC}}, "Jon Smith", 0.0},
		{"zero threshold zero overlap", []models.Person{{ID: "1", Name: "John Smith", ListType: models.ListOFAC}}, "X", 0.0},
		{"negative threshold", []models.Person{{ID: "1", Name: "John Smith", ListType: models.ListOFAC}}, "X", -0.5},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			expected := screening.Screen(tc.query, tc.list, tc.threshold)
			idx := screening.BuildIndex(tc.list)
			got := screening.ScreenIndex(tc.query, idx, tc.threshold)

			if !matchSetsEqual(expected, got, tc.threshold) {
				t.Errorf("match sets differ for %q (threshold %.2f):\n  Screen:      %d matches %v\n  ScreenIndex: %d matches %v",
					tc.query, tc.threshold,
					len(expected), summarizeMatches(expected),
					len(got), summarizeMatches(got))
			}
		})
	}
}

func summarizeMatches(matches []models.Match) string {
	if len(matches) == 0 {
		return "[]"
	}
	s := "["
	for i, m := range matches {
		if i > 0 {
			s += ", "
		}
		s += m.Person.ID + ":" + string(m.MatchType) + ":" + formatFloat(m.Score)
	}
	return s + "]"
}

func formatFloat(f float64) string {
	return "" // placeholder — simplify to avoid noise in error output
}

func TestScreenIndex_NonASCII(t *testing.T) {
	list := []models.Person{
		{ID: "C-1", Name: "Иван Иванов", ListType: models.ListOFAC},
		{ID: "C-2", Name: "John Smith", ListType: models.ListOFAC},
	}
	idx := screening.BuildIndex(list)

	// Cyrillic exact match — should be found via the index.
	matches := screening.ScreenIndex("иван иванов", idx, 0.8)
	if len(matches) != 1 {
		t.Fatalf("expected 1 Cyrillic exact match, got %d: %+v", len(matches), matches)
	}
	if matches[0].Person.ID != "C-1" {
		t.Errorf("wrong person: %s", matches[0].Person.ID)
	}
	if matches[0].MatchType != "exact" {
		t.Errorf("expected exact, got %s", matches[0].MatchType)
	}

	// ASCII query should not match Cyrillic entry.
	matches2 := screening.ScreenIndex("john smith", idx, 0.8)
	if len(matches2) != 1 || matches2[0].Person.ID != "C-2" {
		t.Fatalf("expected only C-2 match for 'john smith', got %+v", matches2)
	}
}

func TestScreenIndex_EmptyQuery(t *testing.T) {
	list := []models.Person{
		{ID: "1", Name: "John Smith", ListType: models.ListOFAC},
		{ID: "2", Name: "", ListType: models.ListOFAC},
	}
	idx := screening.BuildIndex(list)

	// Empty query against non-empty name → no match (consistent with TestEmptyInputs).
	matches := screening.ScreenIndex("", idx, 0.8)
	for _, m := range matches {
		if m.Person.ID != "2" {
			t.Errorf("empty query should only match empty-name entries, got %s", m.Person.ID)
		}
	}

	// Empty query against empty-name entry → exact match.
	if len(matches) != 1 {
		t.Errorf("expected 1 match (empty vs empty = exact), got %d", len(matches))
	}
	if len(matches) > 0 && matches[0].MatchType != "exact" {
		t.Errorf("expected exact match for empty vs empty, got %s", matches[0].MatchType)
	}
}
