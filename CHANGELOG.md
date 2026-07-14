# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [1.2.0] - 2026-07-14

### Added

- **Cross-script matching** in `pkg/sanctions/translit.go`: Cyrillic names are now transliterated into Latin search variants, enabling matches such as `Vladimir Putin` ↔ `Владимир Путин`.
  - Two transliteration schemes are generated (ICAO-style and BGN/PCGN-style) to cover common ambiguities without combinatorial explosion.
  - Supports Russian and Ukrainian Cyrillic characters, including `ґ`, `є`, `і`, `ї`.
- **Token-based name matching** in `pkg/screening/screening.go`: order-independent token comparison with best-match assignment.
  - Handles reversed name order (`Smith John` ↔ `John Smith`).
  - Tolerates missing or extra middle names (`John Smith` ↔ `John Paul Smith`).
  - Supports initial matching (`J. P. Smith`, `JP Smith`).
  - Applies a small surname boost to the last token.
- **Explainable matches**: every `Match` now includes an optional `Explain` field (`models.MatchExplain`) with:
  - normalized input and matched list variants,
  - matching method (`exact_primary`, `exact_alias`, `transliterated_exact`, `token`, `fuzzy`),
  - normalization path (`base`, `no_punct`, `translit`),
  - alias/transliteration flags,
  - token and string sub-scores.
- **Inverted-token candidate index** in `pkg/screening/index.go`: `BuildIndex` pre-computes normalized variants and a token prefix index for fast candidate retrieval, with a safe linear-scan fallback for small datasets.
- **Threshold validation** via `screening.ValidateThreshold`: rejects thresholds `<= 0` or `> 1` with a clear error instead of silently returning empty results.
- **Comprehensive test coverage** in `pkg/sanctions/translit_test.go`, `pkg/sanctions/normalize_test.go`, and `pkg/screening/*_test.go` for:
  - cross-script Latin ↔ Cyrillic cases,
  - name reordering and middle-name variants,
  - punctuation and diacritic normalization,
  - negative cases (short names, generic tokens, invalid thresholds).

### Changed

- **Unicode normalization pipeline** rewritten in `pkg/sanctions/normalize.go`:
  - NFC composition,
  - Unicode case folding,
  - whitespace trimming and collapsing,
  - NFD-based diacritic stripping,
  - punctuation variants (`J. P. Smith` → `j p smith` and `jp smith`),
  - special-case mappings (`ß → ss`, `ł → l`, `Ł → L`).
- **Screening engine** refactored to separate candidate generation from final scoring.
- **Batch API handler** in `internal/server/server.go` refactored to a fixed worker pool with `sync.WaitGroup` and local panic recovery; small batches (`< 8` names) still run sequentially.
- **README** rewritten to accurately describe the new matching pipeline, performance characteristics, and API behavior.

### Fixed

- Cross-script matches no longer fail due to ASCII-only pre-filters.
- Reversed name order and punctuation variants now score above the default threshold.
- `spacedInitialsVariant` only splits all-uppercase short tokens (e.g. `JP` → `j p`), avoiding false splits of normal short names.
- CSV batch screening count now excludes the header row.
- `make bench-full` no longer fails when `time` output does not contain the expected `real` line.

## [1.1.0] - 2026-07-10

### Added

- Self-sustained setup with `make install-hooks`.
- Pre-commit hook that blocks accidental binary commits.

### Fixed

- Lint fixes across the codebase.

## [1.0.0] - 2026-07-10

### Added

- Production-grade sanctions screener with Go library, CLI, and REST API.
- OFAC, EU, and UN sanctions list support.
- SQLite caching with transactional imports.
- Jaro-Winkler fuzzy matching with ASCII bitmap pre-filter.
- Initials expansion matching.
- OpenAPI 3.1 spec and chi-based HTTP server.
- Docker support.
- CI pipeline via GitHub Actions.
