package sanctions

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jstreitberger03/sanctions-screener/pkg/models"
)

type Format string

const (
	FormatCSV      Format = "csv"
	FormatJSON     Format = "json"
	FormatJSONL    Format = "jsonl"
)

func Normalize(name string) string {
	name = strings.ToLower(name)
	name = strings.TrimSpace(name)
	replacer := strings.NewReplacer(
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
	name = replacer.Replace(name)
	return name
}

func Load(path string, format Format) ([]models.Person, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	switch format {
	case FormatJSON, FormatJSONL:
		return parseJSON(data)
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

func parseJSON(data []byte) ([]models.Person, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty data")
	}

	dataStr := string(data)
	if strings.TrimSpace(dataStr)[0] == '[' {
		var entries []simpleEntry
		if err := json.Unmarshal(data, &entries); err != nil {
			return nil, fmt.Errorf("parse json array: %w", err)
		}
		return fromSimple(entries), nil
	}

	var persons []models.Person
	for _, line := range strings.Split(dataStr, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var raw map[string]any
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			continue
		}
		schema, _ := raw["schema"].(string)
		if schema != "Person" && schema != "Organization" {
			continue
		}
		p := openSanctionsToPerson(raw)
		if p != nil {
			persons = append(persons, *p)
		}
	}

	return persons, nil
}

func openSanctionsToPerson(raw map[string]any) *models.Person {
	props, ok := raw["properties"].(map[string]any)
	if !ok {
		return nil
	}

	names, _ := props["name"].([]any)
	if len(names) == 0 {
		return nil
	}
	name, _ := names[0].(string)
	if name == "" {
		return nil
	}

	aliasesRaw, _ := props["alias"].([]any)
	aliases := make([]string, 0, len(aliasesRaw))
	for _, a := range aliasesRaw {
		if s, ok := a.(string); ok {
			aliases = append(aliases, s)
		}
	}

	countries, _ := props["country"].([]any)
	nationality := "unknown"
	if len(countries) > 0 {
		if c, ok := countries[0].(string); ok {
			nationality = strings.ToUpper(c)
		}
	}

	birthDate := ""
	birthDates, _ := props["birthDate"].([]any)
	if len(birthDates) > 0 {
		if bd, ok := birthDates[0].(string); ok {
			birthDate = bd
		}
	}

	var dob *time.Time
	if t, err := time.Parse("2006-01-02", birthDate); err == nil {
		dob = &t
	}

	schema, _ := raw["schema"].(string)

	programsRaw, _ := props["programId"].([]any)
	roles := make([]string, 0, len(programsRaw))
	for _, p := range programsRaw {
		if s, ok := p.(string); ok {
			roles = append(roles, s)
		}
	}

	listType := models.ListEU

	switch schema {
	case "Person":
		roles = append(roles, "individual")
	case "Organization":
		roles = append(roles, "entity")
	}

	id, _ := raw["id"].(string)

	return &models.Person{
		ID:          id,
		Name:        name,
		Aliases:     aliases,
		Nationality: nationality,
		ListType:    listType,
		Roles:       roles,
		DOB:         dob,
	}
}

func fromSimple(entries []simpleEntry) []models.Person {
	persons := make([]models.Person, 0, len(entries))
	for _, e := range entries {
		lt := models.ListEU
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
