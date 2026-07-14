package sanctions

import (
	"strings"
	"unicode"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

// Variant labels describe how a search variant was derived from the
// original name. They are used for explainability and deduplication.
const (
	VariantBase      = "base"
	VariantNoPunct   = "no_punct"
	VariantTranslit  = "translit"
)

// SearchVariant is a single normalized form of a name, ready for indexing
// or comparison. The Label field records which normalization path produced
// it (base, no_punct, translit, etc.).
type SearchVariant struct {
	Text   string
	Tokens []string
	Label  string
}

// Normalize returns the primary normalized form of a name.
// It is kept for backward compatibility; new code should prefer
// NormalizeVariants when multiple search forms are needed.
func Normalize(name string) string {
	variants := NormalizeVariants(name)
	if len(variants) == 0 {
		return ""
	}
	return variants[0].Text
}

// NormalizeQueryVariants returns the same variants as NormalizeVariants plus
// initials-derived variants (e.g. "John Paul Smith" -> "jp smith" and
// "j p smith") that are useful for query expansion but are not indexed for
// list entries, to avoid false-positive exact matches between names sharing
// the same initials.
func NormalizeQueryVariants(name string) []SearchVariant {
	variants := NormalizeVariants(name)
	for _, iv := range queryInitialVariants(name) {
		variants = append(variants, iv)
	}
	variants = deduplicateVariants(variants)
	return variants
}

// queryInitialVariants returns the initials-derived variants used only for
// queries.
func queryInitialVariants(name string) []SearchVariant {
	var out []SearchVariant
	if iv := initialsVariant(name); iv.Text != "" {
		out = append(out, iv)
	}
	if sv := spacedInitialsVariant(name); sv.Text != "" {
		out = append(out, sv)
	}
	return out
}

// spacedInitialsVariant expands a compact-initial query like "JP Smith" into
// "j p smith" so that token matching can align individual initials with
// full-name tokens. Only all-uppercase short tokens are split, so "Kim"
// stays intact while "JP" becomes "j p".
func spacedInitialsVariant(name string) SearchVariant {
	fields := strings.Fields(name)
	if len(fields) < 2 {
		return SearchVariant{}
	}
	var b strings.Builder
	for i, f := range fields {
		if i > 0 {
			b.WriteByte(' ')
		}
		// Only split short tokens that look like compact initials
		// (all letters and all uppercase, e.g. "JP", "JPS").
		if isAllLetters(f) && len(f) <= 3 && len(f) > 1 && strings.ToUpper(f) == f {
			for j, r := range f {
				if j > 0 {
					b.WriteByte(' ')
				}
				b.WriteRune(unicode.ToLower(r))
			}
		} else {
			b.WriteString(strings.ToLower(f))
		}
	}
	return SearchVariant{Text: b.String(), Label: VariantBase}
}

// isAllLetters reports whether s contains only letters.
func isAllLetters(s string) bool {
	for _, r := range s {
		if !unicode.IsLetter(r) {
			return false
		}
	}
	return len(s) > 0
}

// NormalizeVariants returns a deterministic, deduplicated set of search
// variants for a name. The pipeline is:
//
//  1. NFC canonical composition.
//  2. Unicode case folding (language-independent, handles Turkish i/I,
//     Cyrillic, Greek, etc.).
//  3. Trim whitespace and collapse runs of whitespace to a single space.
//  4. NFD decomposition, strip combining diacritical marks (unicode.Mn),
//     then NFC recomposition.
//  5. Generate punctuation variants: one where punctuation is replaced by
//     a space, and one where it is removed entirely.
//  6. For variants containing Cyrillic text, generate transliterated Latin
//     variants via TransliterateCyrillic.
//
// The first returned variant is always the base form (punctuation replaced
// by spaces). Subsequent variants are no-punctuation and transliterated
// forms. The result is deduplicated so identical variants are not emitted
// twice.
func NormalizeVariants(name string) []SearchVariant {
	if len(strings.TrimSpace(name)) == 0 {
		return []SearchVariant{{Text: "", Label: VariantBase}}
	}

	// 1. Canonical composition, case folding, trim, collapse whitespace.
	s := norm.NFC.String(name)
	s = cases.Fold().String(s)
	s = strings.TrimSpace(s)
	s = collapseSpace(s)

	// 2. Strip diacritics via NFD + remove combining marks.
	s = stripDiacritics(s)

	// 3. Generate punctuation variants.
	variants := punctuationVariants(s)

	// 4. Generate Cyrillic transliterations.
	variants = appendTransliteratedVariants(variants)

	// 5. Tokenize and deduplicate.
	return deduplicateVariants(variants)
}

// collapseSpace replaces any run of whitespace characters with a single
// space and trims leading/trailing whitespace.
func collapseSpace(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	inSpace := true // suppress leading spaces
	for _, r := range s {
		if unicode.IsSpace(r) {
			if !inSpace {
				b.WriteByte(' ')
				inSpace = true
			}
			continue
		}
		b.WriteRune(r)
		inSpace = false
	}
	// Trim trailing space if any.
	res := b.String()
	if strings.HasSuffix(res, " ") {
		res = res[:len(res)-1]
	}
	return res
}

// stripDiacritics decomposes s with NFD, removes combining diacritical
// marks (unicode.Mn), recomposes with NFC, and applies a small set of
// special-case mappings that NFD alone cannot handle.
func stripDiacritics(s string) string {
	// Special cases not covered by NFD decomposition or where the base
	// character is not the desired ASCII equivalent.
	s = strings.ReplaceAll(s, "ß", "ss")
	s = strings.ReplaceAll(s, "ł", "l")
	s = strings.ReplaceAll(s, "Ł", "L")

	// NFD -> drop Mn -> NFC.
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range norm.NFD.String(s) {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		b.WriteRune(r)
	}
	return norm.NFC.String(b.String())
}

// punctuationVariants returns the base variant (punctuation -> space) and
// the no-punctuation variant (punctuation removed). Dots inside initials
// are treated as punctuation, so "J. P. Smith" becomes both
// "j p smith" and "jp smith".
func punctuationVariants(s string) []SearchVariant {
	base := replacePunctuationWithSpace(s)
	base = collapseSpace(base)

	noPunct := compactPunctuation(s)

	variants := []SearchVariant{{Text: base, Label: VariantBase}}
	if noPunct != base {
		variants = append(variants, SearchVariant{Text: noPunct, Label: VariantNoPunct})
	}
	return variants
}

// replacePunctuationWithSpace replaces common punctuation and separator
// characters with a single space.
func replacePunctuationWithSpace(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if isPunctuationOrSeparator(r) {
			b.WriteByte(' ')
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// compactPunctuation removes punctuation and then concatenates consecutive
// single-letter tokens (initials) so that "J. P. Smith" becomes "jp smith".
func compactPunctuation(s string) string {
	// First pass: remove punctuation and collapse whitespace.
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if !isPunctuationOrSeparator(r) {
			b.WriteRune(r)
		}
	}
	s = collapseSpace(b.String())

	// Second pass: concatenate consecutive single-letter tokens.
	fields := strings.Fields(s)
	var out []string
	var initials []string
	flush := func() {
		if len(initials) > 0 {
			out = append(out, strings.Join(initials, ""))
			initials = nil
		}
	}
	for _, f := range fields {
		if len(f) == 1 {
			initials = append(initials, f)
			continue
		}
		flush()
		out = append(out, f)
	}
	flush()
	return strings.Join(out, " ")
}

// isPunctuationOrSeparator reports whether r is a punctuation mark,
// apostrophe, hyphen, dot, or similar separator that should be normalized
// away for name matching.
func isPunctuationOrSeparator(r rune) bool {
	// Fast path for ASCII separators.
	if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
		return false
	}
	if r == '\'' || r == '"' || r == '-' || r == '_' || r == '.' ||
		r == ',' || r == ';' || r == ':' || r == '/' || r == '\\' ||
		r == '(' || r == ')' || r == '[' || r == ']' || r == '{' || r == '}' {
		return true
	}
	return unicode.IsPunct(r) || unicode.IsSymbol(r)
}

// initialsVariant returns a variant built from the first letter of each
// token except the last, which is kept in full. For example,
// "John Paul Smith" becomes "jp smith". This supports queries like
// "JP Smith" against full names.
func initialsVariant(s string) SearchVariant {
	fields := strings.Fields(s)
	if len(fields) < 2 {
		return SearchVariant{}
	}
	var b strings.Builder
	for i, f := range fields[:len(fields)-1] {
		if i > 0 {
			b.WriteByte(' ')
		}
		for _, r := range f {
			if unicode.IsLetter(r) {
				b.WriteRune(unicode.ToLower(r))
				break
			}
		}
	}
	if b.Len() > 0 {
		b.WriteByte(' ')
	}
	b.WriteString(fields[len(fields)-1])
	return SearchVariant{Text: b.String(), Label: VariantBase}
}

// appendTransliteratedVariants appends Latin transliterations for any
// variant that contains Cyrillic characters.
func appendTransliteratedVariants(variants []SearchVariant) []SearchVariant {
	var extra []SearchVariant
	for _, v := range variants {
		if containsCyrillic(v.Text) {
			for _, tv := range TransliterateCyrillic(v.Text) {
				extra = append(extra, SearchVariant{Text: tv, Label: VariantTranslit})
			}
		}
	}
	return append(variants, extra...)
}

// containsCyrillic reports whether s contains at least one Cyrillic rune.
func containsCyrillic(s string) bool {
	for _, r := range s {
		if isCyrillicRune(r) {
			return true
		}
	}
	return false
}

func isCyrillicRune(r rune) bool {
	return (r >= '\u0400' && r <= '\u04FF') ||
		(r >= '\u0500' && r <= '\u052F') ||
		(r >= '\u2DE0' && r <= '\u2DFF') ||
		(r >= '\uA640' && r <= '\uA69F') ||
		(r >= '\u1C80' && r <= '\u1C8F')
}

// deduplicateVariants removes duplicate texts and tokenizes each variant.
func deduplicateVariants(variants []SearchVariant) []SearchVariant {
	seen := make(map[string]bool, len(variants))
	out := make([]SearchVariant, 0, len(variants))
	for _, v := range variants {
		if seen[v.Text] {
			continue
		}
		seen[v.Text] = true
		v.Tokens = Tokenize(v.Text)
		out = append(out, v)
	}
	return out
}

// Tokenize splits a normalized name into whitespace-separated tokens.
func Tokenize(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Fields(s)
}

// IsEmptyOrPunctuationOnly reports whether s is empty or contains only
// whitespace and punctuation. It is used to short-circuit screening of
// degenerate inputs.
func IsEmptyOrPunctuationOnly(s string) bool {
	if s == "" {
		return true
	}
	hasLetter := false
	for _, r := range s {
		if unicode.IsLetter(r) {
			hasLetter = true
			break
		}
	}
	return !hasLetter
}
