# Purge Legacy Python CLI

## Context

The CLI has been fully rewritten in Go (`go_src/`). The legacy Python CLI code (`src/`, `tests/`, `pyproject.toml`, etc.) is no longer needed and should be removed to avoid confusion.

## Items to Delete

### Git-tracked (will appear in commit)
- `src/` — entire legacy Python CLI source (`deephaven_cli/`)
- `tests/` — Python pytest tests
- `pyproject.toml` — Python package config
- `uv.lock` — Python dependency lock
- `README.md` — root README describing the old Python `dh` tool

### Local artifacts (not in git, clean up if present)
- `.ruff_cache/`
- `.pytest_cache/`

## Keep
- `go_src/` — all Go source (including `go_src/internal/vm/vm_runner.py` which runs inside the VM)
- `GO_README.md` — documents the Go `dhg` CLI
- `plans/` — design docs

## After deletion
- Rename `GO_README.md` → `README.md` so the repo root has a proper README

## Verification
- `git status` shows only deletions + the README rename
- `cd go_src && go build ./cmd/dhg` still builds
- `cd go_unit_tests && go test ./...` still passes
