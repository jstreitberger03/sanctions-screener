package sanctions_test

import (
	"testing"

	"github.com/jstreitberger03/sanctions-screener/pkg/sanctions"
)

func TestNormalizeVariants_Basic(t *testing.T) {
	cases := []struct {
		input    string
		expected []string
	}{
		{"John Smith", []string{"john smith"}},
		{"  John   Smith  ", []string{"john smith"}},
		{"Jöhn Smíth", []string{"john smith"}},
		{"O'Brien", []string{"o brien", "obrien"}},
		{"Al-Sayed", []string{"al sayed", "alsayed"}},
		{"J. P. Smith", []string{"j p smith", "jp smith"}},
		{"Straße", []string{"strasse"}},
		{"Café", []string{"cafe"}},
		{"François", []string{"francois"}},
		{"Łukasz", []string{"lukasz"}},
		{"José García", []string{"jose garcia"}},
	}

	for _, tc := range cases {
		variants := sanctions.NormalizeVariants(tc.input)
		if len(variants) < len(tc.expected) {
			t.Fatalf("NormalizeVariants(%q) returned %d variants, want at least %d: %v", tc.input, len(variants), len(tc.expected), variants)
		}
		for i, exp := range tc.expected {
			if variants[i].Text != exp {
				t.Errorf("NormalizeVariants(%q)[%d] = %q, want %q", tc.input, i, variants[i].Text, exp)
			}
		}
	}
}

func TestNormalizeVariants_CyrillicTransliteration(t *testing.T) {
	cases := []struct {
		input    string
		contains []string
	}{
		{"Владимир Путин", []string{"vladimir putin"}},
		{"Володимир Зеленський", []string{"volodimir zelenskiy", "volodymyr zelenskyy"}},
		{"Юрий", []string{"yuriy", "iuryy"}},
		{"Юрій", []string{"yuriy", "iuriy"}},
		{"Александр", []string{"aleksandr"}},
	}

	for _, tc := range cases {
		variants := sanctions.NormalizeVariants(tc.input)
		found := false
		for _, v := range variants {
			if v.Label != sanctions.VariantTranslit {
				continue
			}
			for _, exp := range tc.contains {
				if v.Text == exp {
					found = true
					break
				}
			}
			if found {
				break
			}
		}
		if !found {
			t.Errorf("NormalizeVariants(%q) did not produce expected translit variants; got %v", tc.input, variants)
		}
	}
}

func TestNormalizeVariants_Empty(t *testing.T) {
	variants := sanctions.NormalizeVariants("")
	if len(variants) != 1 || variants[0].Text != "" {
		t.Errorf("expected single empty variant, got %v", variants)
	}
}

func TestNormalizeVariants_Deduplication(t *testing.T) {
	// "O'Brien" and "O Brien" should not produce duplicate variants.
	variants := sanctions.NormalizeVariants("O'Brien")
	seen := make(map[string]bool)
	for _, v := range variants {
		if seen[v.Text] {
			t.Errorf("duplicate variant %q", v.Text)
		}
		seen[v.Text] = true
	}
}

func BenchmarkNormalizeVariants(b *testing.B) {
	for b.Loop() {
		_ = sanctions.NormalizeVariants("Jöhn Smíth Müller O'Brien")
	}
}

func BenchmarkNormalizeVariants_Cyrillic(b *testing.B) {
	for b.Loop() {
		_ = sanctions.NormalizeVariants("Владимир Путин")
	}
}

func TestIsEmptyOrPunctuationOnly(t *testing.T) {
	cases := []struct {
		input    string
		expected bool
	}{
		{"", true},
		{"   ", true},
		{"!@#$%", true},
		{"John", false},
		{"O'Brien", false},
	}

	for _, tc := range cases {
		got := sanctions.IsEmptyOrPunctuationOnly(tc.input)
		if got != tc.expected {
			t.Errorf("IsEmptyOrPunctuationOnly(%q) = %v, want %v", tc.input, got, tc.expected)
		}
	}
}
