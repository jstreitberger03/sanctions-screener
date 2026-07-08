# sanctions-screener

[![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

Go library and CLI for screening names against sanctions lists. Supports OFAC, EU consolidated list, and UN sanctions. Ships as a Go package, a command line tool, and a REST API.

Benchmarked against the full EU consolidated list: 5,885 sanctioned persons and organizations, 1.1 seconds per name on a MacBook M-series. Most queries return in under 10ms with the included sample data.

## What's in the box

| Consumption mode | Path | What you get |
|---|---|---|
| Go library | `pkg/screening` | Import the fuzzy matching engine into your own Go code |
| CLI | `cmd/screener` | Terminal tool. Screen names, bulk screen CSV files, import lists |
| REST API | `cmd/api` | HTTP service with JSON endpoints. Same engine, different interface |

## Data

The repo ships with an EU sanctions sample (100 entries). For production use, load the full dataset from OpenSanctions:

```bash
curl -o eu_fsf_raw.json https://data.opensanctions.org/datasets/latest/eu_fsf/entities.ftm.json
screener ingest --source jsonl --data eu_fsf_raw.json
```

The full EU sanctions file contains roughly 5,900 entries and takes about 600ms to load on startup.

### Real EU sanctions data (as of 2026-07-08)

| Metric | Count |
|---|---|
| Total sanctioned entities | 5,885 |
| Persons | 4,340 |
| Organizations | 1,545 |
| Largest bloc | Russia (1,381) |

Top sanctioned countries: Russia (1,381), Iran (414), Belarus (253), Ukraine (242), Syria (218), Afghanistan (125), North Korea (116), Myanmar (59), DR Congo (57), Pakistan (47), China (46).

## Quick start

```bash
go install github.com/jstreitberger03/sanctions-screener/cmd/screener@latest

screener ingest --source json --data data/eu_sample.json

screener screen --name "Irina Kostenko" --threshold 0.8
# [0.85] Ірина Анатоліївна КОСТЕНКО (fuzzy) -- EU
# 1 match found (threshold: 0.80)

screener serve --port 8080
```

## API

```
POST /api/v1/screen        screen a single name
POST /api/v1/screen/batch  screen multiple names
GET  /api/v1/lists         available sanctions lists and entry counts
GET  /api/v1/health        health check
```

### Example

```bash
curl -X POST http://localhost:8080/api/v1/screen \
  -H "Content-Type: application/json" \
  -d '{"name":"Irina Kostenko","threshold":0.8,"lists":["EU"]}'
```

Response:

```json
{
  "matches": [
    {
      "person_id": "NK-23dinXRmxTu4sehASYNAGE",
      "name": "Ірина Анатоліївна КОСТЕНКО",
      "score": 0.85,
      "match_type": "fuzzy",
      "list": "EU",
      "nationality": "UNKNOWN"
    }
  ],
  "screening_time_ms": 1,
  "input_name": "Irina Kostenko",
  "count": 1
}
```

## How matching works

1. **Exact match** (score 1.0). Same name, same script.
2. **Alias match** (score 0.95). Name appears in the entity's alias list.
3. **Jaro-Winkler similarity** for names longer than 3 characters. This catches typos, different transliterations, and partial name matches.
4. **Initial matching**. "J. Smith" from "John Smith" when initials are unambiguous.

Names are normalized before comparison: lowercased, diacritics stripped. Cyrillic and Latin names cross-match when aliases exist, but the engine does not do full transliteration between scripts.

## Library usage

```go
import (
    "github.com/jstreitberger03/sanctions-screener/pkg/ingest"
    "github.com/jstreitberger03/sanctions-screener/pkg/screening"
)

store, _ := ingest.NewStore("sanctions.db")
defer store.Close()

store.ImportJSONL("eu_sanctions.jsonl")
persons, _ := store.LoadCached(models.ListEU)

matches := screening.Screen("John Smith", persons, 0.8)
for _, m := range matches {
    fmt.Printf("%.2f %s\n", m.Score, m.Person.Name)
}
```

## Architecture

```
cmd/screener/    CLI, built with cobra
cmd/api/         REST API entrypoint
pkg/models/      Person, Match, ScreeningResult types
pkg/sanctions/   CSV and JSON parser, name normalization
pkg/screening/   Jaro-Winkler fuzzy matching engine
pkg/ingest/      Import pipeline and SQLite cache
internal/server/ chi HTTP server, middleware, routes
```

## Docker

```bash
docker build -t sanctions-screener .
docker run -p 8080:8080 sanctions-screener
```

## Benchmarks

Screening one name against the full 5,885-entry EU sanctions list on a MacBook M-series:

| Run | Time |
|---|---|
| 1 | 1.25s |
| 2 | 1.11s |
| 3 | 1.32s |

With the 100-entry sample shipped in this repo, queries return in under 1ms.

## Why build this

Sanctions screening is part of AML compliance. Banks, payment processors, and fintechs have to check transactions and customers against OFAC, EU, and UN sanctions lists. The algorithms behind this are not complicated: it is mostly string similarity plus good list management. This repo shows what that looks like in Go.

## License

MIT
