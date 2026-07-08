package models

import "time"

type ListType string

const (
	ListOFAC ListType = "OFAC"
	ListEU   ListType = "EU"
	ListUN   ListType = "UN"
)

type Person struct {
	ID          string
	Name        string
	Aliases     []string
	DOB         *time.Time
	Nationality string
	ListType    ListType
	Roles       []string
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

type ScreeningResult struct {
	Matches       []Match        `json:"matches"`
	InputName     string         `json:"input_name"`
	Threshold     float64        `json:"threshold"`
	ScreeningTime time.Duration  `json:"screening_time_ns"`
}
