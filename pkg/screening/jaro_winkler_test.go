package screening

import (
	"math"
	"testing"
)

func TestJaroWinkler_SameStrings(t *testing.T) {
	if got := jaroWinkler("john", "john"); got != 1.0 {
		t.Errorf("identical strings: expected 1.0, got %.4f", got)
	}
}

func TestJaroWinkler_EmptyBoth(t *testing.T) {
	// Empty strings are equal, so s1==s2 guard returns 1.0.
	if got := jaroWinkler("", ""); got != 1.0 {
		t.Errorf("both empty: expected 1.0, got %.4f", got)
	}
}

func TestJaroWinkler_OneEmpty(t *testing.T) {
	if got := jaroWinkler("john", ""); got != 0.0 {
		t.Errorf("one empty: expected 0.0, got %.4f", got)
	}
	if got := jaroWinkler("", "john"); got != 0.0 {
		t.Errorf("other empty: expected 0.0, got %.4f", got)
	}
}

func TestJaroWinkler_CompletelyDifferent(t *testing.T) {
	// No characters in common → JW = 0.
	if got := jaroWinkler("abc", "xyz"); got != 0.0 {
		t.Errorf("totally different: expected 0.0, got %.4f", got)
	}
}

func TestJaroWinkler_ShortStrings(t *testing.T) {
	// Single character, same.
	if got := jaroWinkler("a", "a"); got != 1.0 {
		t.Errorf("single same char: expected 1.0, got %.4f", got)
	}
	// Single character, different.
	if got := jaroWinkler("a", "b"); got != 0.0 {
		t.Errorf("single different char: expected 0.0, got %.4f", got)
	}
}

func TestJaroWinkler_MaxPrefix(t *testing.T) {
	// All 4 prefix chars match → prefixLen=4, boosting JW above Jaro.
	// jaro("abcd", "abcdef") = (4/4 + 4/6 + 4/4)/3 = (1 + 0.6667 + 1)/3 = 0.8889
	// JW = 0.8889 + 4*0.1*(1-0.8889) = 0.8889 + 0.4*0.1111 = 0.8889 + 0.0444 = 0.9333
	got := jaroWinkler("abcd", "abcdef")
	expected := 0.9333
	if math.Abs(got-expected) > 0.002 {
		t.Errorf("max prefix (4): expected ~%.4f, got %.4f", expected, got)
	}

	// All 4 prefix chars match on equal-length strings.
	got2 := jaroWinkler("abcd", "abce")
	// jaro: 3/4 matches per side, 0 transpositions.
	// jaro = (3/4 + 3/4 + 3/3)/3 = (0.75 + 0.75 + 1)/3 = 0.8333
	// prefixLen=3 (a,b,c). JW = 0.8333 + 3*0.1*0.1667 = 0.8333 + 0.05 = 0.8833
	expected2 := 0.8833
	if math.Abs(got2-expected2) > 0.002 {
		t.Errorf("prefix=3: expected ~%.4f, got %.4f", expected2, got2)
	}
}

func TestJaroWinkler_Transposition(t *testing.T) {
	// Classic example: "martha" vs "marhta" (transposed 't' and 'h').
	// jaro: 6/6 matches, 1 transposition pair (2 mismatches).
	// jaro = (6/6 + 6/6 + (6-2/2)/6)/3 = (1 + 1 + 5/6)/3 = 0.9444
	// prefixLen=3 (m,a,r). JW = 0.9444 + 3*0.1*0.0556 = 0.9444 + 0.0167 = 0.9611
	got := jaroWinkler("martha", "marhta")
	expected := 0.9611
	if math.Abs(got-expected) > 0.002 {
		t.Errorf("martha/marhta: expected ~%.4f, got %.4f", expected, got)
	}
}

func TestJaroWinkler_DwayneDuane(t *testing.T) {
	// "dwayne" vs "duane" — 4 matches, 0 transpositions.
	// jaro = (4/6 + 4/5 + 4/4)/3 = (0.6667 + 0.8 + 1)/3 = 0.8222
	// prefixLen=1 (d). JW = 0.8222 + 0.1*0.1778 = 0.8400
	got := jaroWinkler("dwayne", "duane")
	expected := 0.8400
	if math.Abs(got-expected) > 0.002 {
		t.Errorf("dwayne/duane: expected ~%.4f, got %.4f", expected, got)
	}
}

func TestJaroWinkler_NoPrefixMatch(t *testing.T) {
	// "abc" vs "xabc" — first chars differ, prefixLen=0, no JW boost.
	// jaro: 3/3 for s1, 3/4 for s2, 3/3. jaro = (1 + 0.75 + 1)/3 = 0.9167
	// prefixLen=0. JW = 0.9167
	got := jaroWinkler("abc", "xabc")
	expected := 0.9167
	if math.Abs(got-expected) > 0.003 {
		t.Errorf("no prefix match: expected ~%.4f, got %.4f", expected, got)
	}
}

func TestJaroWinkler_PrefixCappedAtFour(t *testing.T) {
	// "abcde" vs "abcdef" — without the cap, prefixLen would be 5.
	// jaro: all 5 chars match. jaro = (5/5 + 5/6 + 5/5)/3 = (1 + 0.8333 + 1)/3 = 0.9444
	// prefixLen capped at 4. JW = 0.9444 + 4*0.1*(1-0.9444) = 0.9444 + 0.0222 = 0.9667
	got := jaroWinkler("abcde", "abcdef")
	expected := 0.9667
	if math.Abs(got-expected) > 0.002 {
		t.Errorf("prefix cap: expected ~%.4f, got %.4f (should be capped at 4, not 5)", expected, got)
	}
}

func TestJaroWinkler_Subset(t *testing.T) {
	// "ab" is a subset of "abc" but not a prefix.
	// len1=2, len2=3, matchDist=0 (max/2-1 = 1-1=0)
	// Only same-position chars match: 'a'=='a', 'b'=='b'. matches=2.
	// jaro = (2/2 + 2/3 + 2/2)/3 = (1 + 0.6667 + 1)/3 = 0.8889
	// prefixLen=2. JW = 0.8889 + 2*0.1*0.1111 = 0.9111
	got := jaroWinkler("ab", "abc")
	expected := 0.9111
	if math.Abs(got-expected) > 0.002 {
		t.Errorf("subset: expected ~%.4f, got %.4f", expected, got)
	}
}

func BenchmarkJaroWinkler(b *testing.B) {
	for b.Loop() {
		jaroWinkler("martha", "marhta")
	}
}

func TestHaveOverlap(t *testing.T) {
	tests := []struct {
		a, b     string
		expected bool
	}{
		// Both ASCII, share bytes.
		{"john", "johnson", true},
		{"abc", "cde", true},
		// Both ASCII, no common bytes.
		{"abc", "xyz", false},
		// ASCII vs pure non-ASCII (Cyrillic).
		{"john", "путин", false},
		{"putin", "путин", false},
		// Both non-ASCII (conservative: returns true).
		{"путин", "медведев", true},
		{"путин", "путин", true},
		// Mixed ASCII + non-ASCII vs pure non-ASCII.
		{"café", "путин", false}, // 'é' is non-ASCII, but 'c','a','f' are ASCII → hasASCIIa=true, hasASCIIb=false
		// Empty strings.
		{"", "", true},     // both non-ASCII → true (conservative)
		{"", "abc", false}, // hasASCIIa=false, hasASCIIb=true → no overlap
	}

	for _, tt := range tests {
		got := haveOverlap(tt.a, tt.b)
		if got != tt.expected {
			t.Errorf("haveOverlap(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.expected)
		}
	}
}

func BenchmarkHaveOverlap(b *testing.B) {
	for b.Loop() {
		haveOverlap("john smith", "путин владимир")
	}
}
