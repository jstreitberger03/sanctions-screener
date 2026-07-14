package screening

import (
	"errors"
	"strings"
	"unicode"

	"github.com/jstreitberger03/sanctions-screener/pkg/models"
	"github.com/jstreitberger03/sanctions-screener/pkg/sanctions"
)

const (
	minScoreExact = 1.0
	minScoreAlias = 0.95
	// minInitialsSimilarity is the floor for considering an initials-only
	// query close enough to a full name's initials to warrant expanding.
	minInitialsSimilarity = 0.9
)

// ValidateThreshold returns an error if threshold is not in the valid
// range (0, 1].
func ValidateThreshold(threshold float64) error {
	if threshold <= 0 || threshold > 1 {
		return errors.New("threshold must be greater than 0 and at most 1")
	}
	return nil
}

// Screen matches an input name against a list of persons and returns
// results scoring at or above the threshold, sorted by score descending.
//
// Screen builds a fresh Index on every call. For repeated screenings of the
// same list (server path, batch), prefer BuildIndex + ScreenIndex to avoid
// rebuilding the index per call.
//
// For invalid thresholds, Screen returns nil. Use ScreenErr if you need
// the validation error.
func Screen(name string, list []models.Person, threshold float64) []models.Match {
	matches, _ := ScreenErr(name, list, threshold)
	return matches
}

// ScreenErr is like Screen but returns a validation error for invalid
// thresholds instead of silently returning nil.
func ScreenErr(name string, list []models.Person, threshold float64) ([]models.Match, error) {
	if err := ValidateThreshold(threshold); err != nil {
		return nil, err
	}
	idx := BuildIndex(list)
	return ScreenIndex(name, idx, threshold), nil
}

// matchEntry scores all query variants against all entry variants and returns
// the best match if it meets the threshold.
func matchEntry(input string, queryVariants []sanctions.SearchVariant, e *Entry, threshold float64) *models.Match {
	if len(queryVariants) == 0 {
		return nil
	}

	var exactMatch, fuzzyMatch *models.Match

	for _, qv := range queryVariants {
		for _, ev := range e.Variants {
			m := scoreVariant(input, qv, ev, e, threshold)
			if m == nil {
				continue
			}
			switch m.MatchType {
			case models.MatchExact, models.MatchAlias:
				if exactMatch == nil || m.Score > exactMatch.Score {
					exactMatch = m
				}
			default:
				if fuzzyMatch == nil || m.Score > fuzzyMatch.Score {
					fuzzyMatch = m
				}
			}
		}
	}

	// Initials expansion fallback: only consider when no exact/alias match
	// was found, so that alias-exact matches are not overridden by an
	// initials-only expansion.
	if exactMatch == nil {
		if initialsScore, initialsMatch := tryInitialsMatch(input, e, threshold); initialsMatch != nil {
			if initialsScore > 0 && (fuzzyMatch == nil || initialsScore > fuzzyMatch.Score) {
				fuzzyMatch = initialsMatch
			}
		}
	}

	// Exact/alias matches take precedence over fuzzy matches.
	if exactMatch != nil {
		return exactMatch
	}
	if fuzzyMatch != nil && fuzzyMatch.Score >= threshold {
		return fuzzyMatch
	}
	return nil
}

// scoreVariant compares a single query variant against a single entry variant.
func scoreVariant(input string, qv sanctions.SearchVariant, ev variantEntry, e *Entry, threshold float64) *models.Match {
	isTranslit := qv.Label == sanctions.VariantTranslit || ev.Label == sanctions.VariantTranslit

	// Exact match.
	if qv.Text == ev.Text {
		if ev.IsAlias {
			// Preserve legacy alias-exact score (0.95) and threshold gating.
			if minScoreAlias < threshold {
				return nil
			}
			return newMatch(e.Person, minScoreAlias, models.MatchAlias, input, qv.Text, ev.Text, ev.Label, "exact_alias", true, isTranslit)
		}
		if isTranslit {
			return newMatch(e.Person, minScoreExact, models.MatchFuzzy, input, qv.Text, ev.Text, ev.Label, "transliterated_exact", false, true)
		}
		return newMatch(e.Person, minScoreExact, models.MatchExact, input, qv.Text, ev.Text, ev.Label, "exact_primary", false, false)
	}

	// Alias threshold gate: if this is an alias variant and the exact path
	// did not fire, do not fall through to fuzzy (preserves existing
	// threshold-gating behavior).
	if ev.IsAlias {
		return nil
	}

	// Transliterated exact match.
	if isTranslit && qv.Text == ev.Text {
		return newMatch(e.Person, minScoreExact, models.MatchFuzzy, input, qv.Text, ev.Text, ev.Label, "transliterated_exact", false, true)
	}

	// Token-based fuzzy match (order-independent).
	tokenScore, _, _ := tokenMatch(qv.Tokens, ev.Tokens)
	if tokenScore >= threshold && tokenScore > 0 {
		m := newMatch(e.Person, tokenScore, models.MatchFuzzy, input, qv.Text, ev.Text, ev.Label, "token", false, isTranslit)
		m.Explain.TokenScore = tokenScore
		return m
	}

	// Full-string Jaro-Winkler fuzzy match.
	jwScore := jaroWinkler(qv.Text, ev.Text)
	if jwScore >= threshold && jwScore > 0 {
		m := newMatch(e.Person, jwScore, models.MatchFuzzy, input, qv.Text, ev.Text, ev.Label, "fuzzy", false, isTranslit)
		m.Explain.StringScore = jwScore
		return m
	}

	return nil
}

// newMatch creates a Match with its Explain populated from the given
// variant comparison. It centralizes the repetitive construction of the
// Explain struct.
func newMatch(person models.Person, score float64, matchType models.MatchType, input,
	inputVariant, matchedVariant, normalization, method string, isAlias, isTranslit bool) *models.Match {
	return &models.Match{
		Person:    person,
		Score:     score,
		MatchType: matchType,
		InputName: input,
		Explain: &models.MatchExplain{
			InputVariant:   inputVariant,
			MatchedVariant: matchedVariant,
			Method:         method,
			Normalization:  normalization,
			IsAlias:        isAlias,
			IsTranslit:     isTranslit,
		},
	}
}

// tokenMatch computes an order-independent token similarity between two
// token slices. It uses greedy best-match assignment and penalizes missing
// or extra tokens. The last token of each slice is treated as the surname
// and receives a small boost when it matches well.
func tokenMatch(query, target []string) (score float64, missing, extra int) {
	if len(query) == 0 || len(target) == 0 {
		return 0, len(query), len(target)
	}

	used := make([]bool, len(target))
	total := 0.0
	matches := 0

	for qi, qt := range query {
		bestScore := -1.0
		bestIdx := -1
		for ti, tt := range target {
			if used[ti] {
				continue
			}
			s := tokenPairScore(qt, tt)
			// Surname boost: the last token of each side is likely the family
			// name and is weighted slightly higher.
			if qi == len(query)-1 && ti == len(target)-1 {
				if s+0.05 < 1.0 {
					s = s + 0.05
				} else {
					s = 1.0
				}
			}
			if s > bestScore {
				bestScore = s
				bestIdx = ti
			}
		}
		if bestIdx >= 0 && bestScore > 0 {
			used[bestIdx] = true
			total += bestScore
			matches++
		}
	}

	missing = len(query) - matches
	extra = len(target) - matches
	maxTokens := max(len(query), len(target))
	if maxTokens == 0 {
		return 0, missing, extra
	}

	// Penalize missing/extra tokens proportionally.
	penalty := float64(missing+extra) * 0.15
	score = total/float64(maxTokens) - penalty
	if score < 0 {
		score = 0
	}
	return score, missing, extra
}

// tokenPairScore returns a similarity between two individual tokens.
func tokenPairScore(a, b string) float64 {
	if a == b {
		return 1.0
	}
	// Initial match: one token is a single letter and matches the first
	// letter of the other token.
	if len(a) == 1 || len(b) == 1 {
		if strings.HasPrefix(b, a) || strings.HasPrefix(a, b) {
			return 0.85
		}
	}
	// Prefix match.
	if strings.HasPrefix(a, b) || strings.HasPrefix(b, a) {
		return 0.9
	}
	// Jaro-Winkler for typos.
	return jaroWinkler(a, b)
}

// tryInitialsMatch attempts to match an initials-only query against a
// full name. It mirrors the legacy initials path: the normalized input is
// compared to the person's initials; if it is close enough, the initials are
// expanded into the full name and rescored.
func tryInitialsMatch(input string, e *Entry, threshold float64) (float64, *models.Match) {
	if e.Initials == "" {
		return 0, nil
	}
	normalized := sanctions.Normalize(input)
	if jaroWinkler(normalized, e.Initials) < minInitialsSimilarity {
		return 0, nil
	}
	expanded := expandInitials(e.Initials, e.Person.Name)
	score := jaroWinkler(normalized, sanctions.Normalize(expanded))
	if score < threshold {
		return 0, nil
	}
	return score, &models.Match{
		Person:    e.Person,
		Score:     score,
		MatchType: models.MatchInit,
		InputName: input,
	}
}

// expandInitials expands a set of initials into the full name tokens that
// start with those initials.
func expandInitials(initials, fullName string) string {
	parts := strings.Fields(fullName)
	if len(initials) == 0 || len(parts) == 0 {
		return fullName
	}

	var usedArray [8]bool
	used := usedArray[:]
	if len(parts) > len(usedArray) {
		used = make([]bool, len(parts))
	}
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

// jwScratchMax is the rune/byte length threshold for stack-allocated match
// tracking arrays in jaroWinkler.
const jwScratchMax = 128

// isASCII returns true if all bytes of s are < 0x80.
func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

// jaroWinkler computes the Jaro-Winkler similarity between two strings.
func jaroWinkler(s1, s2 string) float64 {
	if s1 == s2 {
		return 1.0
	}

	if isASCII(s1) && isASCII(s2) {
		return jaroWinklerASCII(s1, s2)
	}

	r1 := []rune(s1)
	r2 := []rune(s2)
	len1, len2 := len(r1), len(r2)
	if len1 == 0 || len2 == 0 {
		return 0.0
	}

	var m1stack, m2stack [jwScratchMax]bool
	var m1, m2 []bool
	if len1 <= jwScratchMax && len2 <= jwScratchMax {
		m1 = m1stack[:len1]
		m2 = m2stack[:len2]
	} else {
		m1 = make([]bool, len1)
		m2 = make([]bool, len2)
	}

	return jaroCoreRune(r1, r2, m1, m2)
}

// jaroWinklerASCII computes Jaro-Winkler on pure-ASCII strings.
func jaroWinklerASCII(s1, s2 string) float64 {
	len1, len2 := len(s1), len(s2)
	if len1 == 0 || len2 == 0 {
		return 0.0
	}

	var m1stack, m2stack [jwScratchMax]bool
	var m1, m2 []bool
	if len1 <= jwScratchMax && len2 <= jwScratchMax {
		m1 = m1stack[:len1]
		m2 = m2stack[:len2]
	} else {
		m1 = make([]bool, len1)
		m2 = make([]bool, len2)
	}

	prefixLen := 0
	for i := 0; i < min(min(len1, len2), 4); i++ {
		if s1[i] == s2[i] {
			prefixLen++
		} else {
			break
		}
	}

	matchDist := max(len1, len2)/2 - 1
	if matchDist < 0 {
		matchDist = 0
	}

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

	return jaro + float64(prefixLen)*0.1*(1.0-jaro)
}

// jaroCoreRune computes Jaro-Winkler on fully converted []rune inputs.
func jaroCoreRune(r1, r2 []rune, m1, m2 []bool) float64 {
	len1, len2 := len(r1), len(r2)
	if len1 == 0 || len2 == 0 {
		return 0.0
	}

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

	for i := range m1 {
		m1[i] = false
	}
	for i := range m2 {
		m2[i] = false
	}

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



