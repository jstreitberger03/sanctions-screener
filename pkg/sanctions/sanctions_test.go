package sanctions_test

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/jstreitberger03/sanctions-screener/pkg/models"
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

func TestParseJSON_WhitespaceOnly(t *testing.T) {
	tests := []struct {
		name string
		data string
	}{
		{"whitespace only", "   \n  \n"},
		{"empty string", ""},
		{"tabs and spaces", "\t\t  \t"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := os.CreateTemp(t.TempDir(), "ws-*.json")
			if err != nil {
				t.Fatalf("create temp file: %v", err)
			}
			if _, err := f.WriteString(tt.data); err != nil {
				t.Fatalf("write temp file: %v", err)
			}
			f.Close()

			// Must return an error, not panic.
			_, err = sanctions.Load(f.Name(), sanctions.FormatJSON)
			if err == nil {
				t.Fatal("expected error for whitespace-only input, got nil")
			}
			if !strings.Contains(err.Error(), "empty") {
				t.Errorf("expected error mentioning 'empty', got %q", err.Error())
			}
		})
	}
}

func TestLoadWithTypeCustomDefault(t *testing.T) {
	// Write a minimal JSONL file with one Person entry.
	jsonl := `{"id":"test-1","schema":"Person","properties":{"name":["Test Person"],"country":["RU"]}}`
	f, err := os.CreateTemp(t.TempDir(), "test-*.jsonl")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	if _, err := f.WriteString(jsonl); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	f.Close()

	persons, err := sanctions.LoadWithType(f.Name(), sanctions.FormatJSONL, models.ListUN)
	if err != nil {
		t.Fatalf("LoadWithType: %v", err)
	}
	if len(persons) != 1 {
		t.Fatalf("expected 1 person, got %d", len(persons))
	}
	if persons[0].ListType != models.ListUN {
		t.Errorf("expected ListType UN, got %s", persons[0].ListType)
	}
}

func TestFromSimpleExplicitListOverridesDefault(t *testing.T) {
	// JSON array with explicit "list": "OFAC" should override the default UN.
	jsonArr := `[{"id":"1","name":"Test","list":"OFAC"}]`
	f, err := os.CreateTemp(t.TempDir(), "test-*.json")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	if _, err := f.WriteString(jsonArr); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	f.Close()

	persons, err := sanctions.LoadWithType(f.Name(), sanctions.FormatJSON, models.ListUN)
	if err != nil {
		t.Fatalf("LoadWithType: %v", err)
	}
	if len(persons) != 1 {
		t.Fatalf("expected 1 person, got %d", len(persons))
	}
	if persons[0].ListType != models.ListOFAC {
		t.Errorf("expected ListType OFAC (explicit overrides default), got %s", persons[0].ListType)
	}
}

func TestLoadJSONLDefaultsToEU(t *testing.T) {
	// Backward compat: Load (not LoadWithType) defaults to EU for JSONL.
	jsonl := `{"id":"test-2","schema":"Person","properties":{"name":["EU Person"],"country":["DE"]}}`
	f, err := os.CreateTemp(t.TempDir(), "test-*.jsonl")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	if _, err := f.WriteString(jsonl); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	f.Close()

	persons, err := sanctions.Load(f.Name(), sanctions.FormatJSONL)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(persons) != 1 {
		t.Fatalf("expected 1 person, got %d", len(persons))
	}
	if persons[0].ListType != models.ListEU {
		t.Errorf("expected ListType EU (backward compat default), got %s", persons[0].ListType)
	}
}

// TestParseJSONL_StreamingIdenticalResults verifies the streaming typed-struct
// JSONL parser (parseJSONL) produces the same results as the JSON array parser
// (fromSimple) when given the same data in different formats.
func TestParseJSONL_StreamingIdenticalResults(t *testing.T) {
	// Load the EU sample as a JSON array (exercises fromSimple).
	jsonPersons, err := sanctions.Load("../../data/eu_sample.json", sanctions.FormatJSON)
	if err != nil {
		t.Fatalf("Load JSON: %v", err)
	}

	// Convert the first few entries to FTM JSONL format and write to a temp file.
	// Use the eu_sample.json entries converted to FTM schema format.
	var lines []string
	for i, p := range jsonPersons {
		if i >= 5 {
			break // first 5 is enough for a correctness cross-check
		}
		line := fmt.Sprintf(
			`{"id":%q,"schema":"Person","properties":{"name":[%q],"alias":%s,"country":[%q],"programId":%s}}`,
			p.ID,
			p.Name,
			toJSONArray(p.Aliases),
			strings.ToLower(p.Nationality),
			toJSONArray(p.Roles),
		)
		lines = append(lines, line)
	}
	jsonlData := strings.Join(lines, "\n") + "\n"

	f, err := os.CreateTemp(t.TempDir(), "test-*.jsonl")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	if _, err := f.WriteString(jsonlData); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	f.Close()

	jsonlPersons, err := sanctions.Load(f.Name(), sanctions.FormatJSONL)
	if err != nil {
		t.Fatalf("Load JSONL: %v", err)
	}

	if len(jsonlPersons) < 5 {
		t.Fatalf("expected >=5 persons from JSONL, got %d", len(jsonlPersons))
	}

	for i := range 5 {
		if jsonlPersons[i].Name != jsonPersons[i].Name {
			t.Errorf("person %d: name mismatch: JSONL=%q JSON=%q", i, jsonlPersons[i].Name, jsonPersons[i].Name)
		}
		if jsonlPersons[i].ID != jsonPersons[i].ID {
			t.Errorf("person %d: ID mismatch: JSONL=%q JSON=%q", i, jsonlPersons[i].ID, jsonPersons[i].ID)
		}
	}
}

// toJSONArray returns a JSON array string from a string slice.
func toJSONArray(ss []string) string {
	if len(ss) == 0 {
		return "[]"
	}
	quoted := make([]string, len(ss))
	for i, s := range ss {
		quoted[i] = fmt.Sprintf("%q", s)
	}
	return "[" + strings.Join(quoted, ",") + "]"
}

// TestParseJSONL_WhitespaceAndSkips verifies that the streaming JSONL parser
// gracefully handles blank lines, non-Person/non-Organization schemas, and
// unparseable lines without error.
func TestParseJSONL_WhitespaceAndSkips(t *testing.T) {
	jsonl := "" +
		// Valid Person entry
		`{"id":"ok-1","schema":"Person","properties":{"name":["Valid Person"],"country":["US"]}}` + "\n" +
		// Blank line
		"\n" +
		// Whitespace-only line
		"   \n" +
		// Non-Person/Organization schema — should be skipped
		`{"id":"skip-1","schema":"Company","properties":{"name":["Skip Me"]}}` + "\n" +
		// Unparseable line
		`{this is not json}` + "\n" +
		// Valid Organization entry
		`{"id":"ok-2","schema":"Organization","properties":{"name":["Valid Org"],"country":["DE"]}}` + "\n"

	f, err := os.CreateTemp(t.TempDir(), "test-*.jsonl")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	if _, err := f.WriteString(jsonl); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	f.Close()

	persons, err := sanctions.Load(f.Name(), sanctions.FormatJSONL)
	if err != nil {
		t.Fatalf("Load JSONL: %v", err)
	}
	if len(persons) != 2 {
		t.Fatalf("expected 2 valid persons (Person + Organization), got %d", len(persons))
	}
	if persons[0].Name != "Valid Person" {
		t.Errorf("first person name: expected 'Valid Person', got %q", persons[0].Name)
	}
	if persons[1].Name != "Valid Org" {
		t.Errorf("second person name: expected 'Valid Org', got %q", persons[1].Name)
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
