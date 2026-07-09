package ingest

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/jstreitberger03/sanctions-screener/pkg/models"
	"github.com/jstreitberger03/sanctions-screener/pkg/sanctions"
	_ "github.com/mattn/go-sqlite3"
)

type Store struct {
	db *sql.DB
}

func NewStore(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS sanctions_list (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			aliases TEXT,
			dob TEXT,
			nationality TEXT,
			list_type TEXT NOT NULL,
			roles TEXT
		)
	`); err != nil {
		return nil, fmt.Errorf("create table: %w", err)
	}

	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) ImportOFAC(path string) ([]models.Person, error) {
	persons, err := sanctions.Load(path, sanctions.FormatCSV)
	if err != nil {
		return nil, fmt.Errorf("import ofac: %w", err)
	}
	return persons, s.cache(persons)
}

func (s *Store) ImportEU(path string) ([]models.Person, error) {
	persons, err := sanctions.Load(path, sanctions.FormatJSON)
	if err != nil {
		return nil, fmt.Errorf("import eu: %w", err)
	}
	return persons, s.cache(persons)
}

func (s *Store) ImportJSONL(path string) ([]models.Person, error) {
	persons, err := sanctions.Load(path, sanctions.FormatJSONL)
	if err != nil {
		return nil, fmt.Errorf("import jsonl: %w", err)
	}
	return persons, s.cache(persons)
}

func (s *Store) ImportJSON(path string) ([]models.Person, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	var persons []models.Person
	if err := json.Unmarshal(data, &persons); err != nil {
		return nil, fmt.Errorf("parse json: %w", err)
	}

	return persons, s.cache(persons)
}

func (s *Store) LoadCached(list models.ListType) ([]models.Person, error) {
	rows, err := s.db.Query(
		"SELECT id, name, aliases, dob, nationality, list_type, roles FROM sanctions_list WHERE list_type = ?",
		string(list),
	)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}
	defer rows.Close()

	var persons []models.Person
	for rows.Next() {
		var p models.Person
		var aliasesStr, dobStr, rolesStr string
		var listTypeStr string

		if err := rows.Scan(&p.ID, &p.Name, &aliasesStr, &dobStr, &p.Nationality, &listTypeStr, &rolesStr); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}

		p.ListType = models.ListType(listTypeStr)

		if aliasesStr != "" {
			if err := json.Unmarshal([]byte(aliasesStr), &p.Aliases); err != nil {
				log.Printf("unmarshal aliases for %s: %v", p.ID, err)
			}
		}
		if rolesStr != "" {
			if err := json.Unmarshal([]byte(rolesStr), &p.Roles); err != nil {
				log.Printf("unmarshal roles for %s: %v", p.ID, err)
			}
		}
		if dobStr != "" {
			if dob, err := time.Parse("2006-01-02", dobStr); err == nil {
				p.DOB = &dob
			}
		}

		persons = append(persons, p)
	}

	return persons, rows.Err()
}

func (s *Store) cache(persons []models.Person) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() // no-op after Commit

	stmt, err := tx.Prepare(`
		INSERT OR REPLACE INTO sanctions_list
		(id, name, aliases, dob, nationality, list_type, roles)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("prepare: %w", err)
	}
	defer stmt.Close()

	for _, p := range persons {
		aliasesJSON, err := json.Marshal(p.Aliases)
		if err != nil {
			return fmt.Errorf("marshal aliases for %s: %w", p.ID, err)
		}
		rolesJSON, err := json.Marshal(p.Roles)
		if err != nil {
			return fmt.Errorf("marshal roles for %s: %w", p.ID, err)
		}

		var dobStr string
		if p.DOB != nil {
			dobStr = p.DOB.Format("2006-01-02")
		}

		if _, err := stmt.Exec(p.ID, p.Name, string(aliasesJSON), dobStr, p.Nationality, string(p.ListType), string(rolesJSON)); err != nil {
			return fmt.Errorf("insert: %w", err)
		}
	}

	return tx.Commit()
}
