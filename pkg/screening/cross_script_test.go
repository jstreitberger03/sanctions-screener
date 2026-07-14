package screening_test

import (
	"testing"

	"github.com/jstreitberger03/sanctions-screener/pkg/models"
	"github.com/jstreitberger03/sanctions-screener/pkg/screening"
)

func TestCrossScript_CyrillicLatin(t *testing.T) {
	cases := []struct {
		name       string
		personName string
		query      string
		threshold  float64
		wantCount  int
		minScore   float64
	}{
		{"Vladimir Putin", "Владимир Путин", "Vladimir Putin", 0.8, 1, 1.0},
		{"Volodymyr Zelenskyy", "Володимир Зеленський", "Volodymyr Zelenskyy", 0.8, 1, 1.0},
		{"Yuri", "Юрий", "Yuri", 0.8, 1, 0.9},
		{"Yuriy", "Юрій", "Yuriy", 0.8, 1, 0.9},
		{"Aleksandr", "Александр", "Aleksandr", 0.8, 1, 1.0},
		{"Alexander", "Александр", "Alexander", 0.8, 1, 0.9},
		{"Latin to Cyrillic", "Vladimir Putin", "Владимир Путин", 0.8, 1, 1.0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			persons := []models.Person{
				{ID: "X-1", Name: tc.personName, ListType: models.ListOFAC},
			}
			matches := screening.Screen(tc.query, persons, tc.threshold)
			if len(matches) != tc.wantCount {
				t.Fatalf("got %d matches, want %d: %+v", len(matches), tc.wantCount, matches)
			}
			if tc.wantCount == 0 {
				return
			}
			if matches[0].Score < tc.minScore {
				t.Errorf("got score %.4f, want >= %.4f", matches[0].Score, tc.minScore)
			}
			if matches[0].Explain == nil || !matches[0].Explain.IsTranslit {
				t.Errorf("expected explainability data with IsTranslit=true, got %+v", matches[0].Explain)
			}
		})
	}
}

func TestCrossScript_NonMatchingScripts(t *testing.T) {
	// Arabic and Latin should not match without plausible transliteration.
	persons := []models.Person{
		{ID: "X-1", Name: "محمد الرشيد", ListType: models.ListOFAC},
	}
	matches := screening.Screen("John Smith", persons, 0.8)
	if len(matches) != 0 {
		t.Fatalf("expected 0 matches for unrelated scripts, got %d: %+v", len(matches), matches)
	}
}
