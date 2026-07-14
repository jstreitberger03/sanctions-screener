package screening_test

import (
	"testing"

	"github.com/jstreitberger03/sanctions-screener/pkg/models"
	"github.com/jstreitberger03/sanctions-screener/pkg/screening"
)

func TestPunctuationVariants(t *testing.T) {
	cases := []struct {
		name       string
		personName string
		query      string
		wantType   models.MatchType
		minScore   float64
	}{
		{"O'Brien vs O Brien", "O'Brien", "O Brien", models.MatchExact, 1.0},
		{"O'Brien vs OBrien", "O'Brien", "OBrien", models.MatchExact, 1.0},
		{"Al-Sayed vs Al Sayed", "Al-Sayed", "Al Sayed", models.MatchExact, 1.0},
		{"Al-Sayed vs AlSayed", "Al-Sayed", "AlSayed", models.MatchExact, 1.0},
		{"J. P. Smith vs J P Smith", "J. P. Smith", "J P Smith", models.MatchExact, 1.0},
		{"J. P. Smith vs JP Smith", "J. P. Smith", "JP Smith", models.MatchExact, 1.0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			persons := []models.Person{
				{ID: "X-1", Name: tc.personName, ListType: models.ListOFAC},
			}
			matches := screening.Screen(tc.query, persons, 0.8)
			if len(matches) == 0 {
				t.Fatalf("expected match for punctuation variant, got none")
			}
			if matches[0].MatchType != tc.wantType {
				t.Errorf("got match type %q, want %q", matches[0].MatchType, tc.wantType)
			}
			if matches[0].Score < tc.minScore {
				t.Errorf("got score %.4f, want >= %.4f", matches[0].Score, tc.minScore)
			}
		})
	}
}

func TestDiacriticsInsensitive(t *testing.T) {
	cases := []struct {
		name       string
		personName string
		query      string
	}{
		{"José García", "José García", "Jose Garcia"},
		{"François Dupont", "François Dupont", "Francois Dupont"},
		{"Łukasz Kowalski", "Łukasz Kowalski", "Lukasz Kowalski"},
		{"Jürgen Müller", "Jürgen Müller", "Jurgen Muller"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			persons := []models.Person{
				{ID: "X-1", Name: tc.personName, ListType: models.ListOFAC},
			}
			matches := screening.Screen(tc.query, persons, 0.8)
			if len(matches) == 0 {
				t.Fatalf("expected match for diacritic variant, got none")
			}
			if matches[0].MatchType != models.MatchExact {
				t.Errorf("got match type %q, want exact", matches[0].MatchType)
			}
			if matches[0].Score != 1.0 {
				t.Errorf("got score %.4f, want 1.0", matches[0].Score)
			}
		})
	}
}
