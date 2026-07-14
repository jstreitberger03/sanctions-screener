package screening_test

import (
	"testing"

	"github.com/jstreitberger03/sanctions-screener/pkg/models"
	"github.com/jstreitberger03/sanctions-screener/pkg/screening"
)

func TestTokenMatch_ReversedOrder(t *testing.T) {
	cases := []struct {
		name       string
		personName string
		query      string
		minScore   float64
	}{
		{"John Smith reversed", "John Smith", "Smith John", 0.9},
		{"Vladimir Putin reversed", "Vladimir Putin", "Putin Vladimir", 0.9},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			persons := []models.Person{
				{ID: "X-1", Name: tc.personName, ListType: models.ListOFAC},
			}
			matches := screening.Screen(tc.query, persons, 0.8)
			if len(matches) == 0 {
				t.Fatalf("expected match for reversed order, got none")
			}
			if matches[0].Score < tc.minScore {
				t.Errorf("got score %.4f, want >= %.4f", matches[0].Score, tc.minScore)
			}
		})
	}
}

func TestTokenMatch_MiddleNames(t *testing.T) {
	cases := []struct {
		name       string
		personName string
		query      string
		minScore   float64
	}{
		{"missing middle name", "John Paul Smith", "John Smith", 0.80},
		{"extra middle name", "John Smith", "John Paul Smith", 0.80},
		{"Vladimir Vladimirovich Putin", "Vladimir Vladimirovich Putin", "Vladimir Putin", 0.80},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			persons := []models.Person{
				{ID: "X-1", Name: tc.personName, ListType: models.ListOFAC},
			}
			matches := screening.Screen(tc.query, persons, 0.8)
			if len(matches) == 0 {
				t.Fatalf("expected match for middle-name variant, got none")
			}
			if matches[0].Score < tc.minScore {
				t.Errorf("got score %.4f, want >= %.4f", matches[0].Score, tc.minScore)
			}
		})
	}
}

func TestTokenMatch_Initials(t *testing.T) {
	cases := []struct {
		name       string
		personName string
		query      string
		wantMatch  bool
	}{
		{"J. P. Smith", "John Paul Smith", "J. P. Smith", true},
		{"J Smith", "John Smith", "J Smith", true},
		{"JP Smith", "John Paul Smith", "JP Smith", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			persons := []models.Person{
				{ID: "X-1", Name: tc.personName, ListType: models.ListOFAC},
			}
			matches := screening.Screen(tc.query, persons, 0.8)
			if tc.wantMatch && len(matches) == 0 {
				t.Fatalf("expected match for initials query, got none")
			}
			if !tc.wantMatch && len(matches) > 0 {
				t.Fatalf("expected no match, got %d", len(matches))
			}
		})
	}
}
