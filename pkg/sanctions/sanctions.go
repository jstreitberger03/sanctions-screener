package sanctions

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"golang.org/x/text/unicode/norm"

	"github.com/jstreitberger03/sanctions-screener/pkg/models"
)

// diacriticReplacer strips common diacritics. Created once at package init to avoid
// the expensive trie construction inside the hot path of Normalize.
var diacriticReplacer = strings.NewReplacer(
	"ä", "a", "ö", "o", "ü", "u", "ß", "ss",
	"á", "a", "à", "a", "â", "a", "ã", "a",
	"é", "e", "è", "e", "ê", "e", "ë", "e",
	"í", "i", "ì", "i", "î", "i", "ï", "i",
	"ó", "o", "ò", "o", "ô", "o", "õ", "o",
	"ú", "u", "ù", "u", "û", "u",
	"ý", "y", "ÿ", "y",
	"ñ", "n", "ç", "c",
	"ă", "a", "ą", "a", "ć", "c", "č", "c",
	"ď", "d", "đ", "d", "ę", "e", "ě", "e",
	"ğ", "g", "ı", "i", "ł", "l", "ń", "n",
	"ň", "n", "ř", "r", "ś", "s", "š", "s",
	"ţ", "t", "ť", "t", "ž", "z",
)

type Format string

const (
	FormatCSV   Format = "csv"
	FormatJSON  Format = "json"
	FormatJSONL Format = "jsonl"
)

// Normalize lowercases, trims whitespace, and strips common Latin
// diacritics. As the first step, an NFC pass unifies decomposed
// (NFD) input into the composed (NFC) form so the byte-sequence
// diacriticReplacer that follows catches both. NFC/NFD are
// canonically equivalent per the Unicode Consortium — visually
// identical inputs always normalize to the same output, so an NFD
// query now matches the NFC version of the same list entry
// exactly (previously these collapsed into two distinct normalized
// strings and could not match).
//
// Edge case worth knowing: precomposed Latin letters whose NFD form
// also has no entry in diacriticReplacer (ǵ → "g" + U+0301, ĳ, ǔ,
// etc.) compose from their NFD spelling and pass through unchanged.
// The replacer only covers common Western diacritics; rarer
// precomposed letters stay as-is rather than collapsing to their
// base character.
func Normalize(name string) string {
	name = norm.NFC.String(name)
	name = strings.ToLower(name)
	name = strings.TrimSpace(name)
	name = diacriticReplacer.Replace(name)
	return name
}

func Load(path string, format Format) ([]models.Person, error) {
	return LoadWithType(path, format, models.ListEU)
}

// LoadWithType works like Load but allows specifying a default list type
// for formats that don't carry their own list metadata (e.g. FTM JSONL).
// For formats with an explicit list field (JSON array "list" key), the
// explicit value overrides the default.
func LoadWithType(path string, format Format, defaultListType models.ListType) ([]models.Person, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	switch format {
	case FormatJSON, FormatJSONL:
		return parseJSON(data, defaultListType)
	case FormatCSV:
		return parseCSV(strings.NewReader(string(data)))
	default:
		return nil, fmt.Errorf("unsupported format: %s", format)
	}
}

type flexStringSlice []string

func (f *flexStringSlice) UnmarshalJSON(b []byte) error {
	if len(b) > 0 && b[0] == '[' {
		var s []string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*f = s
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	*f = []string{s}
	return nil
}

type simpleEntry struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Aliases     []string        `json:"aliases"`
	Nationality string          `json:"nationality"`
	ListType    string          `json:"list"`
	Type        string          `json:"type"`
	Programs    flexStringSlice `json:"programs"`
}

func parseJSON(data []byte, defaultListType models.ListType) ([]models.Person, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("empty data")
	}

	if trimmed[0] == '[' {
		var entries []simpleEntry
		if err := json.Unmarshal(data, &entries); err != nil {
			return nil, fmt.Errorf("parse json array: %w", err)
		}
		return fromSimple(entries, defaultListType), nil
	}

	// JSONL path — delegate to the streaming typed-struct parser.
	return parseJSONL(data, defaultListType)
}

func fromSimple(entries []simpleEntry, defaultListType models.ListType) []models.Person {
	persons := make([]models.Person, 0, len(entries))
	for _, e := range entries {
		lt := defaultListType
		if e.ListType != "" {
			lt = models.ListType(e.ListType)
		}
		persons = append(persons, models.Person{
			ID:          e.ID,
			Name:        e.Name,
			Aliases:     e.Aliases,
			Nationality: e.Nationality,
			ListType:    lt,
			Roles:       []string(e.Programs),
		})
	}
	return persons
}

func parseCSV(r *strings.Reader) ([]models.Person, error) {
	reader := csv.NewReader(r)
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("parse csv: %w", err)
	}

	if len(records) < 2 {
		return nil, fmt.Errorf("empty csv")
	}

	header := records[0]
	colMap := make(map[string]int)
	for i, h := range header {
		colMap[strings.TrimSpace(h)] = i
	}

	var persons []models.Person
	for _, row := range records[1:] {
		p := models.Person{
			ListType: models.ListOFAC,
		}
		if idx, ok := colMap["id"]; ok && idx < len(row) {
			p.ID = strings.TrimSpace(row[idx])
		}
		if idx, ok := colMap["name"]; ok && idx < len(row) {
			p.Name = strings.TrimSpace(row[idx])
		}
		if idx, ok := colMap["nationality"]; ok && idx < len(row) {
			p.Nationality = strings.TrimSpace(row[idx])
		}
		persons = append(persons, p)
	}
	return persons, nil
}
