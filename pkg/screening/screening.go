package screening

import (
	"strings"
	"unicode"

	"github.com/jstreitberger03/sanctions-screener/pkg/models"
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
//
// Screen builds a fresh Index on every call. For repeated screenings of the
// same list (server path, batch), prefer BuildIndex + ScreenIndex to avoid
// rebuilding the index per call.
func Screen(name string, list []models.Person, threshold float64) []models.Match {
	idx := BuildIndex(list)
	return ScreenIndex(name, idx, threshold)
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

// jwScratchMax is the rune/byte length threshold for stack-allocated match
// tracking arrays in jaroWinkler. Virtually all real-world names fit within
// this limit; longer strings fall back to heap-allocated slices.
const jwScratchMax = 128

// isASCII returns true if all bytes of s are < 0x80 (pure ASCII). This is
// used to select a byte-level fast path in jaroWinkler that avoids the
// []rune conversion entirely for the ~90%+ of Western sanctions names
// that are pure ASCII.
func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

// jaroWinkler computes the Jaro-Winkler similarity between two strings.
// For pure-ASCII inputs a byte-level fast path avoids []rune conversion.
// For non-ASCII (Cyrillic, Arabic, CJK, etc.) a rune-based path is used so
// that multi-byte characters are compared correctly (a multi-byte rune is
// not accidentally split into partial byte comparisons).
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

// jaroWinklerASCII computes Jaro-Winkler on pure-ASCII strings by indexing
// byte values directly, avoiding the []rune conversion that the rune path
// requires. For ASCII the byte values and rune values are identical.
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

// jaroCoreRune computes Jaro-Winkler on fully converted []rune inputs. It is
// shared by the rune path in jaroWinkler. s1/s2 are the rune slices and m1/m2
// are pre-allocated boolean tracking slices (stack or heap).
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

	// Reset tracking slices (stack arrays are zero-valued on first use, but
	// jaroWinkler can be called with reused heap slices — clear defensively).
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

	// Track used word indices without allocating a map. Name parts rarely
	// exceed 8; for longer names the extra parts are appended below by
	// checking the used-array boundary.
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
