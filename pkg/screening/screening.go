package screening

import (
	"sort"
	"strings"
	"unicode"

	"github.com/jstreitberger03/sanctions-screener/pkg/models"
	"github.com/jstreitberger03/sanctions-screener/pkg/sanctions"
)

const (
	minScoreExact   = 1.0
	minScoreAlias   = 0.95
	shortNameLength = 3
)

func Screen(name string, list []models.Person, threshold float64) []models.Match {
	normalized := sanctions.Normalize(name)
	var matches []models.Match

	for _, person := range list {
		m := matchPerson(normalized, name, person, threshold)
		if m != nil {
			matches = append(matches, *m)
		}
	}

	sort.Slice(matches, func(i, j int) bool {
		return matches[i].Score > matches[j].Score
	})

	return matches
}

func matchPerson(normalized, input string, person models.Person, threshold float64) *models.Match {
	bestScore := 0.0
	bestType := models.MatchFuzzy

	if score := jaroWinkler(normalized, sanctions.Normalize(person.Name)); score > bestScore {
		bestScore = score
	}

	for _, alias := range person.Aliases {
		if score := jaroWinkler(normalized, sanctions.Normalize(alias)); score > bestScore {
			bestScore = score
		}
	}

	if bestScore >= minScoreExact {
		bestType = models.MatchExact
		bestScore = minScoreExact
	}

	for _, alias := range person.Aliases {
		if sanctions.Normalize(alias) == normalized {
			bestScore = minScoreAlias
			bestType = models.MatchAlias
			break
		}
	}

	initials := extractInitials(person.Name)
	if initials != "" && initialsMatch(normalized, initials) {
		if s := jaroWinkler(normalized, sanctions.Normalize(expandInitials(initials, person.Name))); s > bestScore {
			bestScore = s
			bestType = models.MatchInit
		}
	}

	if bestScore < threshold {
		return nil
	}

	return &models.Match{
		Person:    person,
		Score:     bestScore,
		MatchType: bestType,
		InputName: input,
	}
}

func jaroWinkler(s1, s2 string) float64 {
	if s1 == s2 {
		return 1.0
	}

	len1, len2 := len(s1), len(s2)
	if len1 == 0 || len2 == 0 {
		return 0.0
	}

	matchDist := max(len1, len2)/2 - 1
	if matchDist < 0 {
		matchDist = 0
	}

	m1 := make([]bool, len1)
	m2 := make([]bool, len2)

	matches := 0
	for i := 0; i < len1; i++ {
		start := max(0, i-matchDist)
		end := min(len2, i+matchDist+1)
		for j := start; j < end; j++ {
			if m2[j] {
				continue
			}
			if s1[i] == s2[j] {
				m1[i] = true
				m2[j] = true
				matches++
				break
			}
		}
	}

	if matches == 0 {
		return 0.0
	}

	transpositions := 0
	k := 0
	for i := 0; i < len1; i++ {
		if !m1[i] {
			continue
		}
		for !m2[k] {
			k++
		}
		if s1[i] != s2[k] {
			transpositions++
		}
		k++
	}

	jaro := (float64(matches)/float64(len1) +
		float64(matches)/float64(len2) +
		float64(matches-transpositions/2)/float64(matches)) / 3.0

	prefixLen := 0
	for i := 0; i < min(min(len1, len2), 4); i++ {
		if s1[i] == s2[i] {
			prefixLen++
		} else {
			break
		}
	}

	return jaro + float64(prefixLen)*0.1*(1.0-jaro)
}

func extractInitials(name string) string {
	parts := strings.Fields(name)
	var initials []rune
	for _, p := range parts {
		for _, r := range p {
			if unicode.IsLetter(r) {
				initials = append(initials, r)
				break
			}
		}
	}
	return string(initials)
}

func initialsMatch(input, initials string) bool {
	normalized := sanctions.Normalize(initials)
	return strings.HasPrefix(input, normalized) || jaroWinkler(input, normalized) >= 0.9
}

func expandInitials(initials, fullName string) string {
	parts := strings.Fields(fullName)
	if len(initials) == 0 || len(parts) == 0 {
		return fullName
	}

	used := make(map[int]bool)
	var expanded []string
	initRunes := []rune(initials)

	for _, ir := range initRunes {
		found := false
		for i, p := range parts {
			if used[i] || len(p) == 0 {
				continue
			}
			firstRune := []rune(p)[0]
			if unicode.ToLower(ir) == unicode.ToLower(firstRune) {
				expanded = append(expanded, p)
				used[i] = true
				found = true
				break
			}
		}
		if !found {
			expanded = append(expanded, string(ir))
		}
	}

	for i, p := range parts {
		if !used[i] {
			expanded = append(expanded, p)
		}
	}

	return strings.Join(expanded, " ")
}
