package screening_test

import (
	"testing"

	"github.com/jstreitberger03/sanctions-screener/pkg/models"
	"github.com/jstreitberger03/sanctions-screener/pkg/screening"
)

func TestNegative_ShortNames(t *testing.T) {
	persons := []models.Person{
		{ID: "X-1", Name: "Li", ListType: models.ListOFAC},
	}
	matches := screening.Screen("Lo", persons, 0.8)
	if len(matches) != 0 {
		t.Fatalf("expected no match for short unrelated names, got %d", len(matches))
	}
}

func TestNegative_CommonSurname(t *testing.T) {
	persons := []models.Person{
		{ID: "X-1", Name: "John Smith", ListType: models.ListOFAC},
	}
	matches := screening.Screen("Jane Smith", persons, 0.8)
	if len(matches) == 0 {
		t.Fatalf("expected match for common surname variant, got none")
	}
	// Should be fuzzy, not exact.
	if matches[0].Score >= 1.0 {
		t.Errorf("expected score < 1.0 for common surname variant, got %.4f", matches[0].Score)
	}
}

func TestNegative_UnrelatedAlphabets(t *testing.T) {
	persons := []models.Person{
		{ID: "X-1", Name: "Αλέξανδρος", ListType: models.ListOFAC},
	}
	matches := screening.Screen("Владимир Путин", persons, 0.8)
	if len(matches) != 0 {
		t.Fatalf("expected no match for unrelated alphabets, got %d", len(matches))
	}
}

func TestNegative_EmptyInput(t *testing.T) {
	persons := []models.Person{
		{ID: "X-1", Name: "John Smith", ListType: models.ListOFAC},
	}
	matches := screening.Screen("", persons, 0.8)
	if len(matches) != 0 {
		t.Fatalf("expected no match for empty query, got %d", len(matches))
	}
}

func TestNegative_PunctuationOnly(t *testing.T) {
	persons := []models.Person{
		{ID: "X-1", Name: "John Smith", ListType: models.ListOFAC},
	}
	matches := screening.Screen("!@#$%", persons, 0.8)
	if len(matches) != 0 {
		t.Fatalf("expected no match for punctuation-only query, got %d", len(matches))
	}
}

func TestNegative_InvalidThreshold(t *testing.T) {
	persons := []models.Person{
		{ID: "X-1", Name: "John Smith", ListType: models.ListOFAC},
	}
	if matches, err := screening.ScreenErr("John Smith", persons, 0); err == nil {
		t.Fatalf("expected error for threshold 0, got %d matches", len(matches))
	}
	if matches, err := screening.ScreenErr("John Smith", persons, -0.5); err == nil {
		t.Fatalf("expected error for negative threshold, got %d matches", len(matches))
	}
	if matches, err := screening.ScreenErr("John Smith", persons, 1.5); err == nil {
		t.Fatalf("expected error for threshold > 1, got %d matches", len(matches))
	}
}
