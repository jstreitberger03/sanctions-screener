# Design: sanctions-screener

**Date:** 2026-07-08
**Goal:** GitHub profile showcase — Go-based sanctions screening library, CLI, and API
**Status:** Approved

## Overview

`sanctions-screener` is a Go module that screens names against OFAC/EU/UN sanctions lists. It ships as three consumption modes: a library (`pkg/`), a CLI (`cmd/screener/`), and a REST API (`cmd/api/`). The project follows standard Go conventions (`cmd/pkg` layout) to demonstrate production-grade Go architecture.

## Repository Structure

```
sanctions-screener/
├── cmd/
│   ├── screener/          # CLI entrypoint (cobra)
│   └── api/               # REST API server entrypoint
├── pkg/
│   ├── models/            # Shared types: Person, Match, ScreeningResult
│   ├── sanctions/         # Sanctions list parser (OFAC SDN CSV, EU JSON)
│   ├── screening/         # Fuzzy matching engine (Jaro-Winkler + Levenshtein)
│   └── ingest/            # Watchlist import + SQLite cache
├── api/
│   └── v1/                # OpenAPI 3.1 spec
├── config/                # Example YAML config + defaults
├── data/                  # Sample OFAC SDN data (small subset for demo)
├── internal/
│   └── server/            # HTTP server setup, middleware, routes
├── go.mod
├── go.sum
├── Makefile
├── Dockerfile
└── README.md
```

## Core Packages

### `pkg/models`

Shared types used across all packages.

```go
type Person struct {
    ID          string
    Name        string
    Aliases     []string
    DOB         *time.Time
    Nationality string
    ListType    string   // "SDN", "EU-consolidated", "UN"
    Roles       []string // "politically exposed", "terrorism"
}

type Match struct {
    Person    Person
    Score     float64  // 0.0 - 1.0
    MatchType string   // "exact", "fuzzy", "alias"
    InputName string
}

type ScreeningResult struct {
    Matches       []Match
    ScreeningTime time.Duration
    InputName     string
    Threshold     float64
}
```

### `pkg/sanctions`

Parses sanctions lists from file. Supports:
- OFAC SDN CSV format
- EU Consolidated List JSON

Functions:
- `Load(path string, format string) ([]Person, error)` — parse file into Person slice
- `Normalize(name string) string` — lowercase, remove diacritics, standardize whitespace

### `pkg/screening`

Fuzzy matching engine. Core function:
- `Screen(name string, list []Person, threshold float64) []Match`

Matching strategy (ordered by precedence):
1. Exact match → score 1.0
2. Alias exact match → score 0.95
3. Jaro-Winkler similarity → score = jw_distance (primary fuzzy metric, used for names >3 chars)
4. Levenshtein distance → score = 1 - (distance / max_len) (fallback for short names ≤3 chars)
5. Initial matching → score = jw_distance on expanded form (e.g., "J. Smith" → "John Smith")

Scoring weights are not configurable in v1 — hardcoded for simplicity. Configurability is a scope exclusion.

### `pkg/ingest`

Handles the full import pipeline: parse + normalize + cache.
- `ImportOFAC(path string) ([]Person, error)` — parse OFAC SDN CSV via `pkg/sanctions`, then cache to SQLite
- `ImportEU(path string) ([]Person, error)` — parse EU consolidated JSON via `pkg/sanctions`, then cache to SQLite
- `LoadCached(list string) ([]Person, error)` — load from SQLite cache

Note: `pkg/sanctions` owns parsing and normalization only. `pkg/ingest` owns the full pipeline including caching.

## CLI (`cmd/screener`)

cobra-based CLI with these commands:

```bash
# Screen a single name
screener screen --name "John Smith" --list ofac --threshold 0.8

# Bulk screen from CSV file
screener screen --file transactions.csv --threshold 0.85 --output results.json

# Import/update sanctions list
screener ingest --source ofac --data ./data/sdn.csv

# Start API server
screener serve --port 8080 --config config.yaml

# Show version
screener version
```

## REST API (`cmd/api`)

chi-based HTTP server.

### Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/health` | Health check |
| POST | `/api/v1/screen` | Screen a single name |
| POST | `/api/v1/screen/batch` | Bulk screening |
| GET | `/api/v1/lists` | List available sanctions lists |
| GET | `/api/v1/lists/{id}/count` | Entry count per list |

### Request/Response

**POST /api/v1/screen**

Request:
```json
{
  "name": "Mohammed Al-Rashid",
  "threshold": 0.8,
  "lists": ["ofac", "eu"]
}
```

Response:
```json
{
  "matches": [
    {
      "person_id": "SDN-12345",
      "name": "Mohammed Al-Rashid",
      "score": 0.92,
      "match_type": "fuzzy",
      "list": "ofac",
      "nationality": "SY"
    }
  ],
  "screening_time_ms": 12
}
```

## Tech Stack

| Dependency | Purpose |
|------------|---------|
| `github.com/go-chi/chi/v5` | HTTP router |
| `github.com/spf13/cobra` | CLI framework |
| `github.com/spf13/viper` | YAML config |
| `github.com/agnivade/levenshtein` | Levenshtein distance |
| `github.com/google/jaro-winkler` | Jaro-Winkler similarity |
| `github.com/mattn/go-sqlite3` | SQLite for caching |
| `github.com/golangci/golangci-lint` | Linting |

## Testing

- **Unit tests:** `pkg/sanctions`, `pkg/screening` — table-driven tests
- **Integration tests:** API handler tests via `httptest`
- **Benchmarks:** `pkg/screening` — performance under load
- **Commands:** `make test`, `make bench`, `make lint`

## Docker

Multi-stage build:
```dockerfile
FROM golang:1.22-alpine AS builder
# ... build stage
FROM alpine:3.19
# ... runtime stage
```

Usage:
```bash
docker build -t aml-screener .
docker run -p 8080:8080 aml-screener serve
```

## README Highlights

- Go + CI + License badges
- Feature list with CLI screenshot/GIF
- Architecture diagram (ASCII or mermaid)
- Quick Start (3 commands)
- API examples with curl
- "Why this exists" — short domain context

## Scope Exclusions

- No web UI (out of scope for showcase)
- No real-time streaming screening
- No authentication (demo purposes)
- No ML-based matching (keeps it focused on Go skills)
