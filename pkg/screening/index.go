package screening

import (
	"sort"
	"strings"

	"github.com/jstreitberger03/sanctions-screener/pkg/models"
	"github.com/jstreitberger03/sanctions-screener/pkg/sanctions"
)

// Entry holds a Person alongside pre-computed normalized fields to avoid
// repeated Normalize calls per query. Entries are immutable after
// BuildIndex populates them.
type Entry struct {
	Person       models.Person
	NormName     string   // pre-normalized primary name
	NormAliases  []string // pre-normalized aliases
	Initials     string   // extractInitials(person.Name)
	NormInitials string   // pre-normalized initials (avoids per-query Normalize)
}

// Index is an immutable, read-only screening index over a list of Persons.
// It combines pre-normalized entries with a token inverted index for
// candidate pruning. Safe for concurrent read access after BuildIndex.
type Index struct {
	entries  []Entry
	inverted map[tokenKey][]int // token → entry indices
}

// tokenKey is a normalized prefix token used for candidate pruning.
// The first 2 runes of each normalized word are used; shorter words use
// the full word. Entry and alias tokens are inserted into the inverted
// map. ScreenIndex collects candidate entries by looking up query tokens.
type tokenKey string

// tokenRuneLen is the length of the try used for token prefixing.
const tokenRuneLen = 2

// BuildIndex constructs an Index from a list of persons, pre-normalizing
// all names, aliases, and initials, and inserting token prefixes into the
// inverted index. The returned Index is ready for use with ScreenIndex
// and is safe for concurrent reads.
func BuildIndex(list []models.Person) *Index {
	idx := &Index{
		entries:  make([]Entry, len(list)),
		inverted: make(map[tokenKey][]int, len(list)*2),
	}

	for i, person := range list {
		normName := sanctions.Normalize(person.Name)
		normAliases := make([]string, len(person.Aliases))
		for j, alias := range person.Aliases {
			normAliases[j] = sanctions.Normalize(alias)
		}
		initials := extractInitials(person.Name)

		entry := Entry{
			Person:       person,
			NormName:     normName,
			NormAliases:  normAliases,
			Initials:     initials,
			NormInitials: sanctions.Normalize(initials),
		}
		idx.entries[i] = entry

		// Insert tokens from the normalized name and all aliases into
		// the inverted index for candidate lookup.
		tokenKeys := make(map[tokenKey]bool)
		for _, tk := range extractTokens(normName) {
			tokenKeys[tk] = true
		}
		for _, na := range normAliases {
			for _, tk := range extractTokens(na) {
				tokenKeys[tk] = true
			}
		}
		for tk := range tokenKeys {
			// Deduplicate entry indices in the posting list.
			postings := idx.inverted[tk]
			if len(postings) == 0 || postings[len(postings)-1] != i {
				idx.inverted[tk] = append(postings, i)
			}
		}
	}

	return idx
}

// extractTokens splits a normalized string into prefix tokens. Each
// whitespace-delimited field contributes one token: the first tokenRuneLen
// runes of the field, or the full field if shorter. Returns nil for empty
// input.
func extractTokens(s string) []tokenKey {
	if s == "" {
		return nil
	}
	fields := strings.Fields(s)
	tokens := make([]tokenKey, 0, len(fields))
	for _, f := range fields {
		if f == "" {
			continue
		}
		runes := []rune(f)
		n := tokenRuneLen
		if len(runes) < n {
			n = len(runes)
		}
		tokens = append(tokens, tokenKey(string(runes[:n])))
	}
	return tokens
}

// mustScanAll reports whether ScreenIndex must scan all entries
// conservatively: empty queries, queries with no 2-character tokens, or
// when the token lookup returns no candidates (single-char query).
func mustScanAll(normalized string, tokens []tokenKey, idx *Index) bool {
	if normalized == "" {
		return true
	}
	if len(tokens) == 0 {
		return true
	}
	return false
}

// ScreenIndex screens a single name against a pre-built Index. It extracts
// query tokens, collects candidate entries from the inverted index, and
// runs the full matching logic (exact, alias, fuzzy, initials) on each
// candidate. When the query is empty or has no usable tokens, all entries
// are scanned conservatively to preserve existing match behavior.
func ScreenIndex(name string, idx *Index, threshold float64) []models.Match {
	normalized := sanctions.Normalize(name)
	tokens := extractTokens(normalized)

	// Collect candidate indices from the inverted index.
	var candidates []int
	seen := make(map[int]bool)

	if mustScanAll(normalized, tokens, idx) {
		// Conservative full scan for empty/no-token queries — preserves
		// TestEmptyInputs behavior.
		for i := range idx.entries {
			candidates = append(candidates, i)
		}
	} else {
		for _, tk := range tokens {
			for _, i := range idx.inverted[tk] {
				if !seen[i] {
					seen[i] = true
					candidates = append(candidates, i)
				}
			}
		}
		// If no candidates were found from tokens (misspelled query with
		// no shared token) scan all — let haveOverlap/JW sort it out so
		// we don't silently return zero when Screen would find something.
		if len(candidates) == 0 {
			for i := range idx.entries {
				candidates = append(candidates, i)
			}
		}
	}

	var matches []models.Match
	for _, ci := range candidates {
		entry := &idx.entries[ci]
		m := matchEntry(normalized, name, entry, threshold)
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

// matchEntry is the pre-normalized analog of matchPerson. It performs
// the same matching logic (exact, alias, fuzzy, initials) but uses the
// pre-computed NormName, NormAliases, and Initials fields on Entry,
// avoiding per-query Normalize allocations.
func matchEntry(normalized, input string, e *Entry, threshold float64) *models.Match {
	// 1. Exact match on primary name (string equality).
	if e.NormName == normalized {
		return &models.Match{
			Person:    e.Person,
			Score:     minScoreExact,
			MatchType: models.MatchExact,
			InputName: input,
		}
	}

	// 2. Exact match on alias (respects threshold — no fuzzy fallback).
	for _, normAlias := range e.NormAliases {
		if normAlias == normalized {
			if minScoreAlias >= threshold {
				return &models.Match{
					Person:    e.Person,
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

	if haveOverlap(normalized, e.NormName) {
		if score := jaroWinkler(normalized, e.NormName); score > bestScore {
			bestScore = score
		}
	}

	for _, normAlias := range e.NormAliases {
		if haveOverlap(normalized, normAlias) {
			if score := jaroWinkler(normalized, normAlias); score > bestScore {
				bestScore = score
			}
		}
	}

	// 4. Initial matching (e.g. "J. Smith" → "John Smith").
	if e.Initials != "" {
		normInitials := e.NormInitials // pre-computed at build time
		matched := strings.HasPrefix(normalized, normInitials) ||
			(haveOverlap(normalized, normInitials) && jaroWinkler(normalized, normInitials) >= minInitialsSimilarity)
		if matched {
			expanded := expandInitials(e.Initials, e.Person.Name)
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
		Person:    e.Person,
		Score:     bestScore,
		MatchType: bestType,
		InputName: input,
	}
}
