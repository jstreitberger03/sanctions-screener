package screening_test

import (
	"testing"

	"github.com/jstreitberger03/sanctions-screener/pkg/models"
	"github.com/jstreitberger03/sanctions-screener/pkg/screening"
)

// Multilingual exact match: Normalize handles non-Latin scripts for literal
// exact matches (lowercase + trim) but does NOT transliterate between
// scripts. "Владимир Путин" matches only "Владимир Путин", not "Vladimir
// Putin". Real-world callers must pass names in the same script as the
// sanctions list entry. This is documented behavior, not a bug — fixing
// transliteration is a separate project (phonetic buckets etc.).
func TestMultilingualExactMatch(t *testing.T) {
	cases := []struct {
		name         string
		personName   string
		query        string
		threshold    float64
		wantCount    int
		wantType     models.MatchType
		wantMinScore float64
	}{
		{
			name:         "cyrillic exact match after lowercasing",
			personName:   "Иван Иванов",
			query:        "иван иванов",
			threshold:    0.8,
			wantCount:    1,
			wantType:     models.MatchExact,
			wantMinScore: 1.0,
		},
		{
			name:         "cyrillic query against latin transliteration now matches",
			personName:   "Владимир Путин",
			query:        "Vladimir Putin",
			threshold:    0.8,
			wantCount:    1,
			wantType:     models.MatchFuzzy,
			wantMinScore: 1.0,
		},
		{
			name:         "cjk exact match (literal same string)",
			personName:   "李明",
			query:        "李明",
			threshold:    0.8,
			wantCount:    1,
			wantType:     models.MatchExact,
			wantMinScore: 1.0,
		},
		{
			name:       "cjk query against romanized pinyin does NOT match",
			personName: "李明",
			query:      "Li Ming",
			threshold:  0.8,
			wantCount:  0,
		},
		{
			name:         "arabic exact match after lowercasing",
			personName:   "محمد الرشيد",
			query:        "محمد الرشيد",
			threshold:    0.8,
			wantCount:    1,
			wantType:     models.MatchExact,
			wantMinScore: 1.0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			persons := []models.Person{
				{ID: "X-1", Name: tc.personName, ListType: models.ListOFAC},
			}
			matches := screening.Screen(tc.query, persons, tc.threshold)
			if len(matches) != tc.wantCount {
				t.Fatalf("got %d matches, want %d", len(matches), tc.wantCount)
			}
			if tc.wantCount == 0 {
				return
			}
			if matches[0].MatchType != tc.wantType {
				t.Errorf("got match type %q, want %q", matches[0].MatchType, tc.wantType)
			}
			if matches[0].Score < tc.wantMinScore {
				t.Errorf("got score %.4f, want >= %.4f", matches[0].Score, tc.wantMinScore)
			}
		})
	}
}

// TestNFDFoldsToNFCThroughScreen is the end-to-end counterpart of
// the unit-level NFC tests in pkg/sanctions/sanctions_test.go. A
// list entry stored in NFC ("Café") and a query in NFD ("Cafe\u0301")
// must produce an exact match (score 1.0) because Normalize unifies
// both into "cafe". Pre-fix this would have fallen through to fuzzy
// with a JW score near zero — silently missing the case. This guards
// end-to-end regression (e.g. someone reordering the Normalize
// pipeline or swapping the NFC step out).
func TestNFDFoldsToNFCThroughScreen(t *testing.T) {
	persons := []models.Person{
		{ID: "X-1", Name: "Café", ListType: models.ListOFAC},
		{ID: "X-2", Name: "John Smith", ListType: models.ListOFAC},
	}
	t.Run("NFC list entry vs NFD query", func(t *testing.T) {
		matches := screening.Screen("Cafe\u0301", persons, 0.8)
		if len(matches) != 1 {
			t.Fatalf("got %d matches, want 1 (NFC/NFD unification should produce exact): %+v", len(matches), matches)
		}
		if matches[0].Person.ID != "X-1" {
			t.Errorf("matched wrong entry: got %s, want X-1", matches[0].Person.ID)
		}
		if matches[0].MatchType != models.MatchExact {
			t.Errorf("got match type %q, want exact (NFC unification)", matches[0].MatchType)
		}
		if matches[0].Score != 1.0 {
			t.Errorf("got score %.4f, want 1.0", matches[0].Score)
		}
	})

	t.Run("NFD list entry vs NFC query", func(t *testing.T) {
		nfdPersons := []models.Person{
			{ID: "Y-1", Name: "Cafe\u0301"},
		}
		matches := screening.Screen("Café", nfdPersons, 0.8)
		if len(matches) != 1 {
			t.Fatalf("got %d matches, want 1", len(matches))
		}
		if matches[0].MatchType != models.MatchExact {
			t.Errorf("got match type %q, want exact", matches[0].MatchType)
		}
	})

	t.Run("two decomposition points in one query both unify", func(t *testing.T) {
		// č composed (U+010D) is in the diacritic replacer. NFD form
		// is c + U+030C. Verify both collapse to "c" identically —
		// one NFC pass handles every combining mark in the string.
		nfdPersons := []models.Person{
			{ID: "Z-1", Name: "Fančovič"},
		}
		// NFC list, NFD query with two combining marks.
		matches := screening.Screen("Fanc\u030Covic\u030C", nfdPersons, 0.8)
		if len(matches) != 1 {
			t.Fatalf("got %d matches, want 1", len(matches))
		}
		if matches[0].MatchType != models.MatchExact {
			t.Errorf("got match type %q, want exact (multi-point NFC unification)", matches[0].MatchType)
		}
		if matches[0].Score != 1.0 {
			t.Errorf("got score %.4f, want 1.0", matches[0].Score)
		}
	})
}

// Punctuation (', -, .) is not stripped by Normalize. A query that drops
// punctuation produces a fuzzy match, scores 0.85–0.95 typically, and
// matches at standard 0.8 thresholds. Match type is fuzzy, not exact.
// Sanctions: this means any apostrophe/hyphen variation must rely on
// fuzzy matching to be caught. Empirically safe but worth documenting.
func TestPunctuationFallsThroughToFuzzy(t *testing.T) {
	cases := []struct {
		name       string
		query      string
		personName string
		wantCount  int
		wantType   models.MatchType
		minScore   float64
	}{
		{
			name:       "apostrophe dropped in query (OBrien vs O'Brien)",
			query:      "OBrien",
			personName: "O'Brien",
			wantCount:  1,
			wantType:   models.MatchExact,
			minScore:   1.0,
		},
		{
			name:       "hyphen replaced by space (Jean Paul vs Jean-Paul)",
			query:      "Jean Paul Sartre",
			personName: "Jean-Paul Sartre",
			wantCount:  1,
			wantType:   models.MatchExact,
			minScore:   1.0,
		},
		{
			name:       "dot in initials query still produces fuzzy match",
			query:      "J. Smith",
			personName: "John Smith",
			wantCount:  1,
			wantType:   models.MatchFuzzy,
			minScore:   0.85,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			persons := []models.Person{
				{ID: "X-1", Name: tc.personName, ListType: models.ListOFAC},
			}
			matches := screening.Screen(tc.query, persons, 0.8)
			if len(matches) != tc.wantCount {
				t.Fatalf("got %d matches, want %d (matches=%+v)", len(matches), tc.wantCount, matches)
			}
			if matches[0].MatchType != tc.wantType {
				t.Errorf("got match type %q, want %q (fuzzy fallback expected)", matches[0].MatchType, tc.wantType)
			}
			if matches[0].Score < tc.minScore {
				t.Errorf("got score %.4f, want >= %.4f", matches[0].Score, tc.minScore)
			}
		})
	}
}

// Reversed name order is now handled by the token-based matcher.
// "Smith John" against "John Smith" should produce a high-scoring fuzzy
// match because the tokens are the same, just reordered.
func TestReversedNameOrderMatches(t *testing.T) {
	persons := []models.Person{
		{ID: "1", Name: "Smith John", ListType: models.ListOFAC},
	}
	matches := screening.Screen("John Smith", persons, 0.8)
	if len(matches) == 0 {
		t.Fatalf("reversed-name pair produced no matches, want at least 1")
	}
	if matches[0].Score < 0.9 {
		t.Errorf("expected high score for reversed names, got %.4f", matches[0].Score)
	}
}

// Pure initialism query ("JS") against a full-name person does NOT match.
// The initials heuristic extracts "JS" from "John Smith" but the post-
// expansion JW between "js" and "john smith" is ~0.76, below the 0.8
// threshold. Users who want initials to fire should either pass a name
// fragment matching the full pattern (relying on JW) or pre-process names
// to expand initials themselves.
func TestPureInitialsQueryDoesNotMatch(t *testing.T) {
	persons := []models.Person{
		{ID: "1", Name: "John Smith", ListType: models.ListOFAC},
	}
	matches := screening.Screen("JS", persons, 0.8)
	// JW("js","john smith") ≈ 0.76, below threshold.
	if len(matches) != 0 {
		t.Fatalf("initials-only query produced %d matches, want 0 (JW limitation): %+v",
			len(matches), matches)
	}
}

// Empty / whitespace inputs and empty primary names. Documents behavior:
// empty query normalizes to ""; two empty strings compare equal and yield
// an exact match. Whitespace-only queries normalize to "". Empty aliases
// combine with empty query to produce a non-empty alias-exact match
// (false-positive on malformed data) — flagged for future hardening but
// not asserted here since it's a defect, not steady-state behavior.
func TestEmptyInputs(t *testing.T) {
	t.Run("empty query against non-empty names yields no match", func(t *testing.T) {
		matches := screening.Screen("", testList(), 0.8)
		if len(matches) != 0 {
			t.Errorf("got %d matches, want 0", len(matches))
		}
	})

	t.Run("whitespace-only query normalizes to empty (no match)", func(t *testing.T) {
		matches := screening.Screen("   ", testList(), 0.8)
		if len(matches) != 0 {
			t.Errorf("got %d matches, want 0", len(matches))
		}
	})

	t.Run("empty primary name matches empty query as exact", func(t *testing.T) {
		persons := []models.Person{
			{ID: "EMPTY", Name: ""},
		}
		matches := screening.Screen("", persons, 0.8)
		if len(matches) != 1 {
			t.Fatalf("got %d matches, want 1 (two empty strings are equal after Normalize)", len(matches))
		}
		if matches[0].MatchType != models.MatchExact {
			t.Errorf("got match type %q, want exact", matches[0].MatchType)
		}
	})

	t.Run("non-empty query against list with empty-name entry does not match empty entry", func(t *testing.T) {
		persons := []models.Person{
			{ID: "EMPTY", Name: ""},
			{ID: "1", Name: "Mohammed Al-Rashid"},
		}
		matches := screening.Screen("Mohammed Al-Rashid", persons, 0.8)
		if len(matches) != 1 {
			t.Fatalf("got %d matches, want 1", len(matches))
		}
		if matches[0].Person.ID != "1" {
			t.Errorf("matched wrong entry: got %s, want 1", matches[0].Person.ID)
		}
	})
}

// Sanctions lists sometimes contain the same name across multiple
// programs (SDN + EU + UN). A query matching a non-unique name must
// return ALL matches, not collapse to one. Pinned here so future
// dedup-by-name changes are intentional.
func TestTwoSameNamedPersonsBothReturned(t *testing.T) {
	persons := []models.Person{
		{ID: "OFAC-1", Name: "John Smith", ListType: models.ListOFAC},
		{ID: "EU-2", Name: "John Smith", ListType: models.ListEU},
	}
	matches := screening.Screen("John Smith", persons, 0.8)
	if len(matches) != 2 {
		t.Fatalf("got %d matches, want 2", len(matches))
	}
	ids := []string{matches[0].Person.ID, matches[1].Person.ID}
	if ids[0] == ids[1] {
		t.Errorf("expected distinct persons, both matches have ID %q", ids[0])
	}
}

// Threshold validation rejects non-positive thresholds. The engine only
// accepts thresholds in (0, 1].
func TestInvalidThresholdRejected(t *testing.T) {
	persons := []models.Person{
		{ID: "1", Name: "John Smith", ListType: models.ListOFAC},
	}

	t.Run("threshold 0 is rejected", func(t *testing.T) {
		matches := screening.Screen("Jon Smith", persons, 0.0)
		if len(matches) != 0 {
			t.Fatalf("expected 0 matches for invalid threshold, got %d", len(matches))
		}
	})

	t.Run("negative threshold is rejected", func(t *testing.T) {
		matches := screening.Screen("X", persons, -0.5)
		if len(matches) != 0 {
			t.Fatalf("expected 0 matches for invalid threshold, got %d", len(matches))
		}
	})

	t.Run("threshold above 1 is rejected", func(t *testing.T) {
		matches := screening.Screen("X", persons, 1.5)
		if len(matches) != 0 {
			t.Fatalf("expected 0 matches for invalid threshold, got %d", len(matches))
		}
	})
}
