package screening

import (
	"sort"
	"strings"
	"unicode"

	"github.com/jstreitberger03/sanctions-screener/pkg/models"
	"github.com/jstreitberger03/sanctions-screener/pkg/sanctions"
)

const (
	minScoreExact = 1.0
	minScoreAlias = 0.95
	// minInitialsSimilarity is the floor for considering an initials-only
	// query (e.g. "JS") close enough to a full name's initials (e.g. "JS")
	// to warrant expanding into the full form for a second pass. Deliberately
	// stiffer than the caller's threshold so we don't spend work expanding
	// initials that cannot reach it anyway.
	minInitialsSimilarity = 0.9
)

// Screen matches an input name against a list of persons and returns
// results scoring at or above the threshold, sorted by score descending.
func Screen(name string, list []models.Person, threshold float64) []models.Match {
	normalized := sanctions.Normalize(name)
	var matches []models.Match

	for _, person := range list {
		m := matchPerson(normalized, name, person, threshold)
		if m != nil {
			matches = append(matches, *m)
		}
	}

	if len(matches) > 1 {
		sort.Slice(matches, func(i, j int) bool {
			return matches[i].Score > matches[j].Score
		})
	}

	return matches
}

func matchPerson(normalized, input string, person models.Person, threshold float64) *models.Match {
	normName := sanctions.Normalize(person.Name)

	// 1. Exact match on primary name (string equality, not JW score).
	if normName == normalized {
		return &models.Match{
			Person:    person,
			Score:     minScoreExact,
			MatchType: models.MatchExact,
			InputName: input,
		}
	}

	// Pre-normalize aliases once for both exact and fuzzy checks.
	normAliases := make([]string, len(person.Aliases))
	for i, alias := range person.Aliases {
		normAliases[i] = sanctions.Normalize(alias)
	}

	// 2. Exact match on alias (respects threshold — no fuzzy fallback).
	for _, normAlias := range normAliases {
		if normAlias == normalized {
			if minScoreAlias >= threshold {
				return &models.Match{
					Person:    person,
					Score:     minScoreAlias,
					MatchType: models.MatchAlias,
					InputName: input,
				}
			}
			return nil
		}
	}

	// 3. Fuzzy matching via Jaro-Winkler.
	bestScore := 0.0
	bestType := models.MatchFuzzy

	if haveOverlap(normalized, normName) {
		if score := jaroWinkler(normalized, normName); score > bestScore {
			bestScore = score
		}
	}

	for _, normAlias := range normAliases {
		if haveOverlap(normalized, normAlias) {
			if score := jaroWinkler(normalized, normAlias); score > bestScore {
				bestScore = score
			}
		}
	}

	// 4. Initial matching (e.g. "J. Smith" → "John Smith").
	if initials := extractInitials(person.Name); initials != "" {
		normInitials := sanctions.Normalize(initials)
		matched := strings.HasPrefix(normalized, normInitials) ||
			(haveOverlap(normalized, normInitials) && jaroWinkler(normalized, normInitials) >= minInitialsSimilarity)
		if matched {
			expanded := expandInitials(initials, person.Name)
			if s := jaroWinkler(normalized, sanctions.Normalize(expanded)); s > bestScore {
				bestScore = s
				bestType = models.MatchInit
			}
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

// haveOverlap returns true if the two strings could share at least one
// character. Uses an ASCII rune bitmap for speed; when both strings are
// pure non-ASCII (e.g. Cyrillic), returns true conservatively.
func haveOverlap(a, b string) bool {
	hasASCIIa, hasASCIIb := false, false
	var seen [128]bool
	for _, r := range a {
		if r < 128 {
			seen[r] = true
			hasASCIIa = true
		}
	}
	for _, r := range b {
		if r < 128 {
			hasASCIIb = true
			if seen[r] {
				return true
			}
		}
	}
	if hasASCIIa != hasASCIIb {
		return false
	}
	return !hasASCIIa
}

// jaroWinkler computes the Jaro-Winkler similarity between two strings.
// Operates on runes so multi-byte UTF-8 names (Cyrillic, Arabic, CJK)
// are compared correctly.
func jaroWinkler(s1, s2 string) float64 {
	if s1 == s2 {
		return 1.0
	}

	r1 := []rune(s1)
	r2 := []rune(s2)
	len1, len2 := len(r1), len(r2)
	if len1 == 0 || len2 == 0 {
		return 0.0
	}

	// Prefix bonus: up to 4 matching leading characters boost the score.
	prefixLen := 0
	for i := 0; i < min(min(len1, len2), 4); i++ {
		if r1[i] == r2[i] {
			prefixLen++
		} else {
			break
		}
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
			if r1[i] == r2[j] {
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
		if r1[i] != r2[k] {
			transpositions++
		}
		k++
	}

	jaro := (float64(matches)/float64(len1) +
		float64(matches)/float64(len2) +
		float64(matches-transpositions/2)/float64(matches)) / 3.0

	return jaro + float64(prefixLen)*0.1*(1.0-jaro)
}

func extractInitials(name string) string {
	var initials []rune
	for _, p := range strings.Fields(name) {
		for _, r := range p {
			if unicode.IsLetter(r) {
				initials = append(initials, r)
				break
			}
		}
	}
	return string(initials)
}

func expandInitials(initials, fullName string) string {
	parts := strings.Fields(fullName)
	if len(initials) == 0 || len(parts) == 0 {
		return fullName
	}

	used := make(map[int]bool)
	var expanded []string

	for _, ir := range initials {
		found := false
		for i, p := range parts {
			if used[i] || len(p) == 0 {
				continue
			}
			if unicode.ToLower(ir) == unicode.ToLower([]rune(p)[0]) {
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
