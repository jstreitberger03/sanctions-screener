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
	FormatCSV  Format = "csv"
	FormatJSON Format = "json"
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
		"ú", "u", "ù", "u", "û", "u", "ü", "u",
		"ý", "y", "ÿ", "y",
		"ñ", "n", "ç", "c",
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
	case FormatJSON:
		return parseJSON(data)
	case FormatCSV:
		return parseCSV(strings.NewReader(string(data)))
	default:
		return nil, fmt.Errorf("unsupported format: %s", format)
	}
}

type ofacEntry struct {
	Number      string `json:"number"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	Programs    string `json:"programs"`
	Nationality string `json:"nationality"`
	DOB         string `json:"dob"`
	Remarks     string `json:"remarks"`
}

func parseJSON(data []byte) ([]models.Person, error) {
	var entries []ofacEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("parse json: %w", err)
	}

	var persons []models.Person
	for _, e := range entries {
		p := models.Person{
			ID:          e.Number,
			Name:        e.Name,
			Nationality: e.Nationality,
			ListType:    models.ListOFAC,
		}
		if e.DOB != "" {
			dob, err := time.Parse("2006-01-02", e.DOB)
			if err == nil {
				p.DOB = &dob
			}
		}
		persons = append(persons, p)
	}
	return persons, nil
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
