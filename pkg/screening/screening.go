package screening

import (
	"runtime"
	"sort"
	"strings"
	"sync"
	"unicode"

	"github.com/jstreitberger03/sanctions-screener/pkg/models"
	"github.com/jstreitberger03/sanctions-screener/pkg/sanctions"
)

const (
	minScoreExact      = 1.0
	minScoreAlias      = 0.95
	minConcurrentSize  = 100 // split list into goroutines above this threshold
)

// Concurrency controls how many goroutines are used for concurrent screening.
// Defaults to GOMAXPROCS. Set to 1 to disable concurrency entirely.
var Concurrency = runtime.GOMAXPROCS(0)

func Screen(name string, list []models.Person, threshold float64) []models.Match {
	if len(list) <= minConcurrentSize {
		return screenSequential(name, list, threshold)
	}
	return screenConcurrent(name, list, threshold)
}

// screenSequential is the single-threaded screening path for small lists.
func screenSequential(name string, list []models.Person, threshold float64) []models.Match {
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

// screenConcurrent splits the list across Concurrency goroutines, each
// screening its chunk independently, then merges results via a channel.
func screenConcurrent(name string, list []models.Person, threshold float64) []models.Match {
	n := Concurrency
	if n < 1 {
		n = 1
	}
	chunkSize := (len(list) + n - 1) / n

	type result struct {
		matches []models.Match
		index   int
	}
	ch := make(chan result, n)
	var wg sync.WaitGroup

	for i := range n {
		start := i * chunkSize
		end := start + chunkSize
		if end > len(list) {
			end = len(list)
		}
		if start >= end {
			break
		}

		wg.Add(1)
		go func(idx int, chunk []models.Person) {
			defer wg.Done()
			ch <- result{matches: screenSequential(name, chunk, threshold), index: idx}
		}(i, list[start:end])
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	// Collect results ordered by chunk index, then merge.
	results := make([][]models.Match, n)
	for r := range ch {
		results[r.index] = r.matches
	}

	var all []models.Match
	for _, m := range results {
		all = append(all, m...)
	}

	if len(all) > 1 {
		sort.Slice(all, func(i, j int) bool {
			return all[i].Score > all[j].Score
		})
	}

	return all
}

func matchPerson(normalized, input string, person models.Person, threshold float64) *models.Match {
	normName := sanctions.Normalize(person.Name)

	// 1. Exact match on primary name (string equality, not JW score)
	if normName == normalized {
		return &models.Match{
			Person:    person,
			Score:     minScoreExact,
			MatchType: models.MatchExact,
			InputName: input,
		}
	}

	// Pre-normalize aliases once for both exact and fuzzy checks
	normAliases := make([]string, len(person.Aliases))
	for i, alias := range person.Aliases {
		normAliases[i] = sanctions.Normalize(alias)
	}

	// 2. Exact match on alias (respects threshold — no fuzzy fallback)
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

	// 3. Fuzzy matching via Jaro-Winkler
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

	// 4. Initial matching (e.g. "J. Smith" → "John Smith")
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

// haveOverlap returns true if the two strings could share at least one
// character. Uses an ASCII byte bitmap for speed; when both strings are
// pure non-ASCII (e.g. Cyrillic), returns true conservatively.
func haveOverlap(a, b string) bool {
	hasASCIIa, hasASCIIb := false, false
	var seen [128]bool
	for i := range len(a) {
		if a[i] < 128 {
			seen[a[i]] = true
			hasASCIIa = true
		}
	}
	for i := range len(b) {
		if b[i] < 128 {
			hasASCIIb = true
			if seen[b[i]] {
				return true
			}
		}
	}
	if hasASCIIa != hasASCIIb {
		return false
	}
	return !hasASCIIa
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
	return strings.HasPrefix(input, normalized) ||
		(haveOverlap(input, normalized) && jaroWinkler(input, normalized) >= 0.9)
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
