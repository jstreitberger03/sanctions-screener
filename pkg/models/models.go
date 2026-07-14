package models

import "time"

type ListType string

const (
	ListOFAC ListType = "OFAC"
	ListEU   ListType = "EU"
	ListUN   ListType = "UN"
)

type Person struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Aliases     []string   `json:"aliases,omitempty"`
	DOB         *time.Time `json:"dob,omitempty"`
	Nationality string     `json:"nationality,omitempty"`
	ListType    ListType   `json:"list_type"`
	Roles       []string   `json:"roles,omitempty"`
}

type MatchType string

const (
	MatchExact MatchType = "exact"
	MatchAlias MatchType = "alias"
	MatchFuzzy MatchType = "fuzzy"
	MatchInit  MatchType = "initial"

	// Extended match types for explainability. They are emitted by the
	// screening engine when the corresponding path is taken.
)

// MatchExplain provides a machine-readable explanation of why a match was
// produced. It is attached to Match.Explain as an optional, backward-
// compatible extension.
type MatchExplain struct {
	InputVariant   string  `json:"input_variant"`   // normalized query variant used
	MatchedVariant string  `json:"matched_variant"` // normalized list variant matched
	Method         string  `json:"method"`          // e.g. exact, fuzzy, token, transliterated
	Normalization  string  `json:"normalization"`   // e.g. base, no_punct, translit
	IsAlias        bool    `json:"is_alias"`        // true if the match came from an alias
	IsTranslit     bool    `json:"is_translit"`     // true if transliteration was used
	TokenScore     float64 `json:"token_score"`     // token-based sub-score
	StringScore    float64 `json:"string_score"`    // full-string similarity sub-score
}

type Match struct {
	Person    Person        `json:"person"`
	Score     float64       `json:"score"`
	MatchType MatchType     `json:"match_type"`
	InputName string        `json:"input_name"`
	Explain   *MatchExplain `json:"explain,omitempty"` // optional explainability data
}
