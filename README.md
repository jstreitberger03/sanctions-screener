# sanctions-screener

[![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

Go-based sanctions screening tool. Screen names against OFAC/EU/UN sanctions lists via library, CLI, or REST API.

## Features

- **Library**: `import "github.com/jstreitberger03/sanctions-screener/pkg/screening"` — use the fuzzy matching engine directly
- **CLI**: `screener screen --name "John Smith"` — quick terminal-based screening
- **REST API**: `POST /api/v1/screen` — HTTP endpoint for integration
- **Sanctions lists**: OFAC SDN (CSV), EU Consolidated (JSON), extensible format
- **Fuzzy matching**: Jaro-Winkler similarity with alias and initial matching
- **SQLite caching**: Fast repeated access to imported lists

## Quick Start

```bash
# Install
go install github.com/jstreitberger03/sanctions-screener/cmd/screener@latest

# Import sample data
screener ingest --source json --data data/sdn_sample.json

# Screen a name
screener screen --name "Mohammed Al Rashid" --threshold 0.8

# Start API
screener serve --port 8080
```

## API

Start the server:
```bash
screener serve --port 8080
```

### Endpoints

```bash
# Health check
curl http://localhost:8080/health

# Screen a single name
curl -X POST http://localhost:8080/api/v1/screen \
  -H "Content-Type: application/json" \
  -d '{"name":"Mohammed Al-Rashid","threshold":0.8,"lists":["OFAC"]}'

# Bulk screening
curl -X POST http://localhost:8080/api/v1/screen/batch \
  -H "Content-Type: application/json" \
  -d '{"names":["John Smith","Ali Khan"],"threshold":0.8,"lists":["OFAC"]}'

# List available sanctions lists
curl http://localhost:8080/api/v1/lists
```

### Response

```json
{
  "matches": [
    {
      "person_id": "SDN-001",
      "name": "Mohammed Al-Rashid",
      "score": 0.92,
      "match_type": "fuzzy",
      "list": "OFAC",
      "nationality": "SY"
    }
  ],
  "screening_time_ms": 1,
  "input_name": "Mohammed Al-Rashid",
  "count": 1
}
```

## Architecture

```
cmd/
  screener/     CLI (cobra)
  api/          API server entrypoint
pkg/
  models/       Person, Match, ScreeningResult types
  sanctions/    OFAC/EU list parser + name normalization
  screening/    Jaro-Winkler fuzzy matching engine
  ingest/       Import pipeline + SQLite cache
internal/
  server/       HTTP server, middleware, routes
```

## Library Usage

```go
import (
    "github.com/jstreitberger03/sanctions-screener/pkg/ingest"
    "github.com/jstreitberger03/sanctions-screener/pkg/screening"
)

store, _ := ingest.NewStore("sanctions.db")
persons, _ := store.ImportOFAC("data/sdn.csv")
matches := screening.Screen("John Smith", persons, 0.8)
```

## Docker

```bash
docker build -t sanctions-screener .
docker run -p 8080:8080 sanctions-screener
```

## Why This Exists

Sanctions screening is a critical component in AML/CFT compliance. Financial institutions must check transactions and customers against sanctions lists maintained by OFAC (US), the EU, and the UN. This tool demonstrates how to build a performant, embeddable screening engine in Go — the kind of component that powers real compliance systems.

## License

MIT
