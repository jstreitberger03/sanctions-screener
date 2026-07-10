package sanctions

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/jstreitberger03/sanctions-screener/pkg/models"
)

// ftmEntity is a typed struct for FollowTheMoney (OpenSanctions) JSONL entries.
// It replaces map[string]any parsing for the JSONL path, giving the compiler
// type-safety, reducing allocations, and enabling faster json.Unmarshal.
type ftmEntity struct {
	ID         string     `json:"id"`
	Schema     string     `json:"schema"`
	Properties ftmProps   `json:"properties"`
}

type ftmProps struct {
	Name      []string `json:"name"`
	Alias     []string `json:"alias"`
	Country   []string `json:"country"`
	BirthDate []string `json:"birthDate"`
	ProgramID []string `json:"programId"`
}

// parseJSONL streams a JSONL (newline-delimited JSON) byte slice line-by-line
// using bufio.Scanner, unmarshalling each non-empty line as an ftmEntity.
// Lines that fail to parse or whose schema is neither "Person" nor
// "Organization" are silently skipped (logged).
func parseJSONL(data []byte, defaultListType models.ListType) ([]models.Person, error) {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	// 64 KB initial buffer, 1 MB max — sufficient for all real-world FTM entries.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var persons []models.Person
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var entity ftmEntity
		if err := json.Unmarshal(line, &entity); err != nil {
			log.Printf("skipping unparseable line: %v", err)
			continue
		}
		if entity.Schema != "Person" && entity.Schema != "Organization" {
			continue
		}
		p := ftmToPerson(entity, defaultListType)
		if p != nil {
			persons = append(persons, *p)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan jsonl: %w", err)
	}
	return persons, nil
}

// ftmToPerson converts a typed ftmEntity to a models.Person. It replaces the
// former openSanctionsToPerson which used map[string]any access.
func ftmToPerson(entity ftmEntity, defaultListType models.ListType) *models.Person {
	props := entity.Properties

	if len(props.Name) == 0 || props.Name[0] == "" {
		return nil
	}

	aliases := make([]string, 0, len(props.Alias))
	for _, a := range props.Alias {
		if a != "" {
			aliases = append(aliases, a)
		}
	}

	nationality := "unknown"
	if len(props.Country) > 0 && props.Country[0] != "" {
		nationality = strings.ToUpper(props.Country[0])
	}

	var dob *time.Time
	if len(props.BirthDate) > 0 && props.BirthDate[0] != "" {
		if t, err := time.Parse("2006-01-02", props.BirthDate[0]); err == nil {
			dob = &t
		}
	}

	roles := make([]string, 0, len(props.ProgramID))
	for _, p := range props.ProgramID {
		if p != "" {
			roles = append(roles, p)
		}
	}
	switch entity.Schema {
	case "Person":
		roles = append(roles, "individual")
	case "Organization":
		roles = append(roles, "entity")
	}

	return &models.Person{
		ID:          entity.ID,
		Name:        props.Name[0],
		Aliases:     aliases,
		Nationality: nationality,
		ListType:    defaultListType,
		Roles:       roles,
		DOB:         dob,
	}
}
