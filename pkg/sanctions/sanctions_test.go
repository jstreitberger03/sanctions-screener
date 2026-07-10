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
		// Composed (NFC): replacer sequence "é" → "e" hits.
		{"Café", "cafe"},

		// Decomposed (NFD) forms: norm.NFC unifies them up-front, so the
		// diacritic replacer catches the resulting composed character.
		// Inputs are lowercase so ToLower is a no-op and the test
		// isolates the NFC pass from case-folding.
		// Composed (NFC) -> diacritic-strip base
		{"Café", "cafe"},

		// Decomposed (NFD) forms. norm.NFC composes them to NFC, then
		// the replacer strips the diacritic. Inputs are lowercase so
		// ToLower is a no-op and the test isolates the NFC pass.
		{"cafe" + "\u0301", "cafe"},              // e + combining acute (U+0301)
		{"franc" + "\u0327" + "ois", "francois"}, // c + combining cedilla (U+0327)
		{"a" + "\u0308", "a"},                    // a + combining diaeresis (U+0308)

		// Non-Latin scripts: exercises both NFC (must preserve bytes
		// for scripts without combining-mark precomposed forms) AND
		// ToLower (Cyrillic has real uppercase/lowercase pairing).
		// The Hebrew and CJK rows are kept as no-op round-trips so a
		// future regression in the NFC fast-path is still caught on
		// scripts we don't translate at all.
		{"Привет", "привет"},
		{"שָׁלוֹם", "שָׁלוֹם"},
		{"名前", "名前"},
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

func BenchmarkNormalize(b *testing.B) {
	for b.Loop() {
		sanctions.Normalize("Jöhn Smíth Müller Straße")
	}
}

func BenchmarkLoadCSV(b *testing.B) {
	for b.Loop() {
		_, err := sanctions.Load("../../data/sdn_sample.csv", sanctions.FormatCSV)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkLoadJSON(b *testing.B) {
	for b.Loop() {
		_, err := sanctions.Load("../../data/sdn_sample.json", sanctions.FormatJSON)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkLoadEU(b *testing.B) {
	for b.Loop() {
		_, err := sanctions.Load("../../data/eu_sample.json", sanctions.FormatJSON)
		if err != nil {
			b.Fatal(err)
		}
	}
}
