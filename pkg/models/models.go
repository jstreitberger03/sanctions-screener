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
)

type Match struct {
	Person    Person    `json:"person"`
	Score     float64   `json:"score"`
	MatchType MatchType `json:"match_type"`
	InputName string    `json:"input_name"`
}
