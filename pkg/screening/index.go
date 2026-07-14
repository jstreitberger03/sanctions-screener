package screening

import (
	"sort"
	"strings"
	"unicode"

	"github.com/jstreitberger03/sanctions-screener/pkg/models"
	"github.com/jstreitberger03/sanctions-screener/pkg/sanctions"
)

// variantEntry holds a single normalized variant of a name (primary or alias)
// along with pre-computed tokens.
type variantEntry struct {
	Text      string
	Tokens    []string
	Label     string
	IsAlias   bool
	AliasIndex int // index within Person.Aliases, -1 for primary
}

// Entry holds a Person alongside pre-computed normalized variants to avoid
// repeated normalization per query. Entries are immutable after
// BuildIndex populates them.
type Entry struct {
	Person   models.Person
	Variants []variantEntry
	Initials string
}

// Index is an immutable, read-only screening index over a list of Persons.
// It combines pre-normalized entries with a token inverted index for
// candidate pruning. Safe for concurrent read access after BuildIndex.
type Index struct {
	entries  []Entry
	inverted map[tokenKey][]int // token → entry indices
}

// tokenKey is a normalized prefix token used for candidate pruning.
type tokenKey string

// tokenRuneLen is the length of the prefix used for token indexing.
const tokenRuneLen = 2

// BuildIndex constructs an Index from a list of persons, pre-normalizing
// all names and aliases into search variants and inserting token prefixes
// into the inverted index. The returned Index is ready for use with
// ScreenIndex and is safe for concurrent reads.
func BuildIndex(list []models.Person) *Index {
	idx := &Index{
		entries:  make([]Entry, len(list)),
		inverted: make(map[tokenKey][]int, len(list)*2),
	}

	for i, person := range list {
		entry := Entry{Person: person}
		entry.Variants = buildVariants(person)
		entry.Initials = extractInitials(person.Name)
		idx.entries[i] = entry

		// Insert tokens from every variant into the inverted index.
		tokenKeys := make(map[tokenKey]bool)
		for _, v := range entry.Variants {
			for _, tk := range prefixTokens(v.Tokens) {
				tokenKeys[tk] = true
			}
		}
		for tk := range tokenKeys {
			postings := idx.inverted[tk]
			if len(postings) == 0 || postings[len(postings)-1] != i {
				idx.inverted[tk] = append(postings, i)
			}
		}
	}

	return idx
}

// buildVariants returns all search variants for a person (primary + aliases).
func buildVariants(person models.Person) []variantEntry {
	var variants []variantEntry
	for _, sv := range sanctions.NormalizeVariants(person.Name) {
		variants = append(variants, variantEntry{
			Text:       sv.Text,
			Tokens:     sv.Tokens,
			Label:      sv.Label,
			IsAlias:    false,
			AliasIndex: -1,
		})
	}
	for aIdx, alias := range person.Aliases {
		for _, sv := range sanctions.NormalizeVariants(alias) {
			variants = append(variants, variantEntry{
				Text:       sv.Text,
				Tokens:     sv.Tokens,
				Label:      sv.Label,
				IsAlias:    true,
				AliasIndex: aIdx,
			})
		}
	}
	return variants
}

// prefixTokens returns the first tokenRuneLen runes of each token for use
// as inverted-index keys.
func prefixTokens(tokens []string) []tokenKey {
	if len(tokens) == 0 {
		return nil
	}
	keys := make([]tokenKey, 0, len(tokens))
	for _, t := range tokens {
		runes := []rune(t)
		n := tokenRuneLen
		if len(runes) < n {
			n = len(runes)
		}
		if n > 0 {
			keys = append(keys, tokenKey(string(runes[:n])))
		}
	}
	return keys
}

// mustScanAll reports whether ScreenIndex must scan all entries
// conservatively.
func mustScanAll(normalized string, tokens []tokenKey) bool {
	return normalized == "" || len(tokens) == 0
}

// ScreenIndex screens a single name against a pre-built Index. It extracts
// query tokens, collects candidate entries from the inverted index, and runs
// the full matching logic on each candidate.
func ScreenIndex(name string, idx *Index, threshold float64) []models.Match {
	if err := ValidateThreshold(threshold); err != nil {
		return nil
	}

	queryVariants := sanctions.NormalizeQueryVariants(name)
	var normalized string
	if len(queryVariants) > 0 {
		normalized = queryVariants[0].Text
	}

	// Collect candidate indices from the inverted index.
	var candidates []int
	seen := make(map[int]bool)

	var tokens []tokenKey
	for _, qv := range queryVariants {
		tokens = append(tokens, prefixTokens(qv.Tokens)...)
	}

	if mustScanAll(normalized, tokens) {
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
		// Fallback: if no candidates found, scan all entries.
		if len(candidates) == 0 {
			for i := range idx.entries {
				candidates = append(candidates, i)
			}
		}
	}

	var matches []models.Match
	for _, ci := range candidates {
		entry := &idx.entries[ci]
		m := matchEntry(name, queryVariants, entry, threshold)
		if m != nil {
			matches = append(matches, *m)
		}
	}

	if len(matches) > 1 {
		sort.Slice(matches, func(i, j int) bool {
			if matches[i].Score != matches[j].Score {
				return matches[i].Score > matches[j].Score
			}
			return matches[i].Person.ID < matches[j].Person.ID
		})
	}

	return matches
}

// extractInitials returns the first letter of each whitespace-delimited word.
func extractInitials(name string) string {
	var initials []rune
	for _, p := range strings.Fields(name) {
		for _, r := range p {
			if unicode.IsLetter(r) {
				initials = append(initials, unicode.ToLower(r))
				break
			}
		}
	}
	return string(initials)
}
