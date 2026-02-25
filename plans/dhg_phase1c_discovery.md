# Phase 1c: Server Discovery

**Depends on:** Phase 0 (scaffold)
**Parallel with:** Phase 1a (config), Phase 1b (Java)

## Goal

Implement server discovery (find running Deephaven servers), server kill, and the `dhg list` / `dhg kill` commands.

## Files to create/modify

```
go_src/
  internal/
    discovery/
      discover.go          # Platform dispatcher + server classification
      linux.go             # /proc/net/tcp parsing
      darwin.go            # lsof parsing
      docker.go            # docker ps parsing
      kill.go              # Stop server by port
  cmd/dhg/
    list.go                # dhg list
    kill.go                # dhg kill <PORT>
```

## Internal package: `internal/discovery`

### discover.go
- `Discover() ([]Server, error)` — dispatch to platform-specific discovery + docker
- `Server` struct: `Port int`, `PID int`, `Source string`, `Script string`, `CWD string`, `ContainerID string`
- `ClassifyProcess(pid int) string` — determine source (dh serve, dh repl, docker, java, unknown)
- Merge results from process discovery + Docker discovery, deduplicate by port

### linux.go (build tag: `//go:build linux`)
- Parse `/proc/net/tcp` and `/proc/net/tcp6` for LISTEN sockets
- Map inode → PID via `/proc/<pid>/fd/` readlink
- Read `/proc/<pid>/comm` and `/proc/<pid>/cmdline` for classification
- Port logic from `discovery.py` lines 96-200

### darwin.go (build tag: `//go:build darwin`)
- Run `lsof -iTCP -sTCP:LISTEN -P -n -F pcn`
- Parse lsof field-format output
- Port logic from `discovery.py` lines 203-260

### docker.go
- Run `docker ps --format '{{.ID}}\t{{.Image}}\t{{.Ports}}'`
- Parse port mappings, filter for Deephaven-related images
- Handle `docker` not installed (return empty, no error)

### kill.go
- `Kill(port int) error` — find server on port, send signal
- For process servers: `syscall.Kill(pid, syscall.SIGTERM)`
- For Docker containers: `docker stop <container_id>`

## Commands

### `dhg list`
List running servers. Human-readable table or JSON array.

### `dhg kill <PORT>`
Stop server on port. Exit 4 if no server found on that port.

## Tests

### Unit tests (`go_unit_tests/discovery_test.go`)
- Linux `/proc/net/tcp` parsing with fixture data (hex addresses, ports, inodes)
- lsof output parsing with fixture data
- Docker `ps` output parsing
- Process classification logic
- Port deduplication

### Behaviour tests (`go_behaviour_tests/testdata/scripts/`)
- `list.txtar`: `dhg list --json` → valid JSON with `"servers"` key
- `kill.txtar`: `dhg kill 99999` → exit 4, error about server not found

## Verification

```bash
./dhg list
./dhg list --json
./dhg kill 99999   # expected: error, exit 4
```
