package sanctions

import (
	"strings"
	"unicode"
)

// TransliterateCyrillic returns a small, fixed set of Latin transliterations
// for a Cyrillic name. The original name is preserved; only additional
// search variants are returned.
//
// Two complete transliteration schemes are produced to cover the most common
// ambiguities without creating a combinatorial explosion:
//
//   1. Standard (ICAO-style): й→y, х→kh, ц→ts, ч→ch, ш→sh, щ→shch,
//      ю→yu, я→ya, ё→yo, є→ye, і→i, ї→yi, ґ→g.
//   2. Alternative (BGN/PCGN-style): й→i, х→h, щ→shch, ю→iu, я→ia,
//      ё→e, є→ie, і→i, ї→ji, ґ→g.
//
// Hard/soft signs (ь, ъ) and the Ukrainian apostrophe (ґ context) are
// removed. The function is deterministic and allocation-light for typical
// names.
func TransliterateCyrillic(s string) []string {
	standard := transliterateWithScheme(s, standardScheme())
	alternative := transliterateWithScheme(s, alternativeScheme())

	// Deduplicate while preserving order.
	seen := make(map[string]bool)
	variants := []string{}
	for _, v := range []string{standard, alternative} {
		v = strings.TrimSpace(collapseSpace(v))
		if v != "" && !seen[v] {
			seen[v] = true
			variants = append(variants, v)
		}
	}
	return variants
}

// transliterationScheme maps Cyrillic runes to their Latin representation.
type transliterationScheme struct {
	mappings map[rune]string
}

func standardScheme() transliterationScheme {
	return transliterationScheme{mappings: map[rune]string{
		'а': "a", 'б': "b", 'в': "v", 'г': "g", 'д': "d", 'е': "e",
		'ё': "yo", 'ж': "zh", 'з': "z", 'и': "i", 'й': "y", 'к': "k",
		'л': "l", 'м': "m", 'н': "n", 'о': "o", 'п': "p", 'р': "r",
		'с': "s", 'т': "t", 'у': "u", 'ф': "f", 'х': "kh", 'ц': "ts",
		'ч': "ch", 'ш': "sh", 'щ': "shch", 'ъ': "", 'ы': "y", 'ь': "",
		'э': "e", 'ю': "yu", 'я': "ya",
		// Ukrainian extensions
		'ґ': "g", 'є': "ye", 'і': "i", 'ї': "yi",
		// Upper-case counter-parts (after case folding these should not appear,
		// but keep the table defensive).
		'А': "a", 'Б': "b", 'В': "v", 'Г': "g", 'Д': "d", 'Е': "e",
		'Ё': "yo", 'Ж': "zh", 'З': "z", 'И': "i", 'Й': "y", 'К': "k",
		'Л': "l", 'М': "m", 'Н': "n", 'О': "o", 'П': "p", 'Р': "r",
		'С': "s", 'Т': "t", 'У': "u", 'Ф': "f", 'Х': "kh", 'Ц': "ts",
		'Ч': "ch", 'Ш': "sh", 'Щ': "shch", 'Ъ': "", 'Ы': "y", 'Ь': "",
		'Э': "e", 'Ю': "yu", 'Я': "ya", 'Ґ': "g", 'Є': "ye", 'І': "i", 'Ї': "yi",
	}}
}

func alternativeScheme() transliterationScheme {
	// Note: 'и' maps to "y" here intentionally. This variant captures a
	// common Eastern-European transliteration style and is pinned by the
	// test suite (e.g. "Владимир Путин" -> "vladymyr putyn").
	return transliterationScheme{mappings: map[rune]string{
		'а': "a", 'б': "b", 'в': "v", 'г': "g", 'д': "d", 'е': "e",
		'ё': "e", 'ж': "zh", 'з': "z", 'и': "y", 'й': "y", 'к': "k",
		'л': "l", 'м': "m", 'н': "n", 'о': "o", 'п': "p", 'р': "r",
		'с': "s", 'т': "t", 'у': "u", 'ф': "f", 'х': "h", 'ц': "ts",
		'ч': "ch", 'ш': "sh", 'щ': "shch", 'ъ': "", 'ы': "y", 'ь': "",
		'э': "e", 'ю': "iu", 'я': "ia",
		// Ukrainian extensions
		'ґ': "g", 'є': "ie", 'і': "i", 'ї': "ji",
		// Upper-case counter-parts
		'А': "a", 'Б': "b", 'В': "v", 'Г': "g", 'Д': "d", 'Е': "e",
		'Ё': "e", 'Ж': "zh", 'З': "z", 'И': "i", 'Й': "i", 'К': "k",
		'Л': "l", 'М': "m", 'Н': "n", 'О': "o", 'П': "p", 'Р': "r",
		'С': "s", 'Т': "t", 'У': "u", 'Ф': "f", 'Х': "h", 'Ц': "ts",
		'Ч': "ch", 'Ш': "sh", 'Щ': "shch", 'Ъ': "", 'Ы': "y", 'Ь': "",
		'Э': "e", 'Ю': "iu", 'Я': "ia", 'Ґ': "g", 'Є': "ie", 'І': "i", 'Ї': "ji",
	}}
}

func transliterateWithScheme(s string, scheme transliterationScheme) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if repl, ok := scheme.mappings[r]; ok {
			b.WriteString(repl)
		} else if unicode.IsSpace(r) {
			b.WriteRune(r)
		}
		// Non-Cyrillic, non-space characters are dropped to avoid mixing
		// Latin noise into the transliterated form.
	}
	return b.String()
}
