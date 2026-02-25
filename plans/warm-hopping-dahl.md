# Plan: Add `dh lint`, `dh format`, and `dh typecheck` subcommands

## Overview

Add dev-tooling subcommands to the `dh` CLI:
- `dh lint` → `ruff check`
- `dh format` → `ruff format`
- `dh typecheck` → `ty check`

All three shell out to the respective tool and pass through the exit code.

## Usage

```bash
dh lint                    # ruff check src/ tests/
dh lint --fix              # ruff check --fix src/ tests/
dh format                  # ruff format src/ tests/
dh format --check          # ruff format --check src/ tests/
dh typecheck               # ty check
```

Extra args can be passed after `--`:
```bash
dh lint -- --select E501
dh typecheck -- --python-version 3.13
```

## Changes

### 1. `pyproject.toml` — add dependencies and config

Add to `[project.optional-dependencies] dev`:
```toml
dev = [
    "pytest>=8.0",
    "pytest-timeout>=2.0",
    "ruff>=0.8.0",
    "ty>=0.0.1a7",
]
```

Add ruff configuration:
```toml
[tool.ruff]
target-version = "py313"
line-length = 120
src = ["src", "tests"]

[tool.ruff.lint]
select = ["E", "F", "I", "W"]
```

### 2. `src/deephaven_cli/cli.py` — add subcommands

**Add three subparsers** after `kill_parser`, before `args = parser.parse_args()` (~line 366):

- `lint` parser with `--fix` flag and `extra` remainder args
- `format` parser with `--check` flag and `extra` remainder args
- `typecheck` parser with `extra` remainder args

**Add dispatch** in routing block (~line 399):
```python
elif args.command == "lint":
    return run_lint(fix=args.fix, extra=args.extra)
elif args.command == "format":
    return run_format(check=args.check, extra=args.extra)
elif args.command == "typecheck":
    return run_typecheck(extra=args.extra)
```

**Add three run functions** (near `run_list`/`run_kill`):

```python
def _strip_separator(extra: list[str] | None) -> list[str]:
    """Strip leading '--' from extra args."""
    return [a for a in (extra or []) if a != "--"]


def run_lint(fix: bool = False, extra: list[str] | None = None) -> int:
    cmd = ["ruff", "check", "src/", "tests/"]
    if fix:
        cmd.append("--fix")
    cmd.extend(_strip_separator(extra))
    return subprocess.run(cmd).returncode


def run_format(check: bool = False, extra: list[str] | None = None) -> int:
    cmd = ["ruff", "format", "src/", "tests/"]
    if check:
        cmd.append("--check")
    cmd.extend(_strip_separator(extra))
    return subprocess.run(cmd).returncode


def run_typecheck(extra: list[str] | None = None) -> int:
    cmd = ["ty", "check"]
    cmd.extend(_strip_separator(extra))
    return subprocess.run(cmd).returncode
```

**Add `import subprocess`** at top (verify it's not already imported).

### 3. `README.md` — add to Development section

Add lint/format/typecheck examples to the existing Development section.

## Files to modify

- `pyproject.toml` — ruff + ty dependencies, ruff config
- `src/deephaven_cli/cli.py` — 3 subparsers, routing, 3 run functions + helper
- `README.md` — document commands

## Verification

1. `uv pip install -e ".[dev]"` — reinstall with ruff and ty
2. `dh --help` — should list lint, format, typecheck subcommands
3. `dh lint` — runs ruff check
4. `dh lint --fix` — auto-fixes
5. `dh format --check` — checks formatting
6. `dh format` — formats files
7. `dh typecheck` — runs ty check
