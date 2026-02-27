# Rename CLI: dhg → dh

## Context

The tool has stabilized and should drop the "g" suffix — `dh` becomes `dh` on the command line. This is a mechanical rename touching binary name, config paths, env vars, string literals, tests, and docs.

## Scope

### Binary & Package
- `src/cmd/dhg/` directory → `src/cmd/dh/`
- Makefile `BINARY := dh` → `dh`, `PKG_NAME := dh-cli` → `dh-cli`
- `.gitignore` entries

### Config Paths & Env Vars
- `~/.dh/` → `~/.dh/`
- `.dhrc` → `.dhrc`
- `DH_HOME` → `DH_HOME`, `DH_VERSION` → `DH_VERSION`, `DH_JSON` → `DH_JSON`
- `DH_VM_NO_UFFD` → `DH_VM_NO_UFFD`, `DH_VM_EAGER_MB` → `DH_VM_EAGER_MB`, `DH_VM_EAGER_UFFD` → `DH_VM_EAGER_UFFD`

### Go Source (string literals & function names)
- `cmd/root.go`: Use, Long, Version strings
- `config/dhgrc.go` → `config/dhrc.go`: rename file, constant `dhgrcFile`, functions `FindDHGRC`/`ReadDHGRC`/`WriteDHGRC`
- `config/config.go`: `DHGHome()` function, `DH_HOME` env var, `~/.dh` default
- `config/resolve.go`: `DH_VERSION` env var
- All `cmd/*.go`: help text, usage examples, error messages
- `vm/rootfs_linux.go`: Docker image/container names, temp dir prefixes
- `vm/prereqs_linux.go`: hint strings
- `vm/machine_linux.go`: comments
- `exec/exec.go`, `exec/exec_vm_linux.go`: env var references
- `exec/runner.py`, `repl/repl_runner.py`, `vm/vm_runner.py`: docstrings
- `internal/cmd/serve.go`: `__DH_READY__` sentinel → `__DH_READY__`
- TUI screens: done.go, config.go, javacheck.go

### Tests
- All 16 `.txtar` files: `exec dh` → `exec dh`, `.dhrc` → `.dhrc`, `DH_HOME` → `DH_HOME`, output matchers
- `behaviour_tests/cli_test.go`: binary build path, command map key, env vars
- `unit_tests/cmd_test.go`: output assertions
- `unit_tests/config_test.go`: function names, file references

### Documentation
- `README.md`: all command examples, env var table, config paths
- `CLAUDE.md`: build commands, env vars, paths
- Plan files: rename `dhg_phase*` → `dh_phase*` files, update content

### Items NOT changing
- Go module path `github.com/dsmmcken/dh-cli/src` (already correct)
- GitHub repo name

## Verification
1. `make build` succeeds, produces `dh` binary
2. `make test` — all unit + behaviour tests pass
3. `make vet` passes
4. `./dh --version` prints `dh v...`
5. `DH_HOME=/tmp/test-dh ./dh doctor --json` uses new env var
