package sanctions_test

import (
	"testing"

	"github.com/jstreitberger03/sanctions-screener/pkg/sanctions"
)

func TestNormalize(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"John Smith", "john smith"},
		{"Jöhn Smíth", "john smith"},
		{"  MÜLLER  ", "muller"},
		{"françois", "francois"},
		{"Straße", "strasse"},
	}

	for _, tt := range tests {
		got := sanctions.Normalize(tt.input)
		if got != tt.expected {
			t.Errorf("Normalize(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestLoadCSV(t *testing.T) {
	persons, err := sanctions.Load("../../data/sdn_sample.csv", sanctions.FormatCSV)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if len(persons) != 5 {
		t.Errorf("expected 5 persons, got %d", len(persons))
	}

	if persons[0].Name != "Mohammed Al-Rashid" {
		t.Errorf("expected Mohammed Al-Rashid, got %s", persons[0].Name)
	}

	if persons[0].ListType != "OFAC" {
		t.Errorf("expected OFAC list type, got %s", persons[0].ListType)
	}
}

func TestLoadJSON(t *testing.T) {
	persons, err := sanctions.Load("../../data/sdn_sample.json", sanctions.FormatJSON)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if len(persons) != 5 {
		t.Errorf("expected 5 persons, got %d", len(persons))
	}

	if persons[0].Name != "Mohammed Al-Rashid" {
		t.Errorf("expected Mohammed Al-Rashid, got %s", persons[0].Name)
	}
}
