package sanctions_test

import (
	"testing"

	"github.com/jstreitberger03/sanctions-screener/pkg/sanctions"
)

func TestTransliterateCyrillic_Russian(t *testing.T) {
	cases := []struct {
		input    string
		expected []string
	}{
		{"Владимир Путин", []string{"vladimir putin", "vladymyr putyn"}},
		{"Александр", []string{"aleksandr"}},
		{"Юрий", []string{"yuriy", "iuryy"}},
		{"Жанна", []string{"zhanna"}},
		{"Харьков", []string{"kharkov", "harkov"}},
		{"Центр", []string{"tsentr"}},
		{"Челябинск", []string{"chelyabinsk", "cheliabynsk"}},
		{"Шостакович", []string{"shostakovich"}},
		{"Щербина", []string{"shcherbina"}},
		{"Южный", []string{"yuzhnyy", "iuzhnyy"}},
		{"Ярослав", []string{"yaroslav", "iaroslav"}},
		{"Йога", []string{"yoga"}},
		{"Ыбраев", []string{"ybraev"}},
		{"Ельцин", []string{"eltsin", "eltsyn"}},
	}

	for _, tc := range cases {
		got := sanctions.TransliterateCyrillic(tc.input)
		if len(got) < len(tc.expected) {
			t.Fatalf("TransliterateCyrillic(%q) = %v, want at least %v", tc.input, got, tc.expected)
		}
		for i, exp := range tc.expected {
			if got[i] != exp {
				t.Errorf("TransliterateCyrillic(%q)[%d] = %q, want %q", tc.input, i, got[i], exp)
			}
		}
	}
}

func TestTransliterateCyrillic_Ukrainian(t *testing.T) {
	cases := []struct {
		input    string
		expected []string
	}{
		{"Володимир Зеленський", []string{"volodimir zelenskiy", "volodymyr zelenskyy"}},
		{"Євген", []string{"yevgen", "ievgen"}},
		{"Ірина", []string{"irina", "iryna"}},
		{"Їжак", []string{"yizhak", "jizhak"}},
		{"Ґрунт", []string{"grunt"}},
	}

	for _, tc := range cases {
		got := sanctions.TransliterateCyrillic(tc.input)
		if len(got) < len(tc.expected) {
			t.Fatalf("TransliterateCyrillic(%q) = %v, want at least %v", tc.input, got, tc.expected)
		}
		for i, exp := range tc.expected {
			if got[i] != exp {
				t.Errorf("TransliterateCyrillic(%q)[%d] = %q, want %q", tc.input, i, got[i], exp)
			}
		}
	}
}

func TestTransliterateCyrillic_HardSoftSigns(t *testing.T) {
	// Hard and soft signs should be removed.
	got := sanctions.TransliterateCyrillic("объект")
	for _, v := range got {
		if v != "obekt" {
			t.Errorf("expected 'obekt', got %q", v)
		}
	}
}
