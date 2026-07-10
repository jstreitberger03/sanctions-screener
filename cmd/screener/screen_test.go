package main

import "testing"

func TestIsHeaderRow(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		// Recognized headers (case-insensitive, trimmed).
		{"name lowercase", "name", true},
		{"name uppercase", "NAME", true},
		{"name mixed case", "Name", true},
		{"name with spaces", "  name  ", true},
		{"full_name", "full_name", true},
		{"full_name uppercase", "FULL_NAME", true},
		{"fullname", "fullname", true},
		{"fullname uppercase", "FULLNAME", true},
		{"entity_name", "entity_name", true},
		{"entity_name uppercase", "ENTITY_NAME", true},

		// Non-headers — must return false.
		{"actual name", "John Smith", false},
		{"empty string", "", false},
		{"whitespace only", "   ", false},
		{"numeric id", "12345", false},
		{"partial match", "name_extra", false},
		{"similar but different", "named", false},
		{"csv header other col", "nationality", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isHeaderRow(tt.input)
			if got != tt.expected {
				t.Errorf("isHeaderRow(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}
