# VM Workspace File Sharing via Vsock Pre-population

## Context

When running `dhg exec --vm`, user code executes inside a Firecracker microVM that has no access to host files. Scripts that reference local data files (e.g., `read_csv("./tests/sample_data.csv")`) fail with `NoSuchFileException`. This makes the VM mode impractical for any workflow involving external data files.

The goal: make workspace files accessible to the VM with **zero impact** on startup or exec time for the no-files case.

**Baseline benchmarks:**
- `basic_script.py` (no files): ~10.6s total, 27ms restore
- `basic_import.py` (reads CSV): fails with `NoSuchFileException`

## Approach: Extend Vsock JSON Protocol with File Payload

Extend the existing `VsockRequest` to include a `files` map. The host auto-detects file references in the user's code, reads those files, and sends them base64-encoded alongside the code. The VM runner writes them to disk before executing.

### Why This Approach

- **Zero startup impact** — file transfer piggybacks on the existing vsock request
- **Zero overhead for no-files case** — `omitempty` means JSON is byte-identical
- **Minimal code** — ~30 lines Go, ~15 lines Python, no new dependencies
- **No snapshot changes** — works with existing snapshots, no re-prepare needed
- **Transparent to user code** — files appear at expected paths, no code changes needed

### Path Mapping

The VM's CWD is `/` (set at boot, frozen in snapshot). User code paths resolve relative to `/`:

```
Host CWD:     /workspace
Code says:    read_csv("./tests/sample_data.csv")
Host file:    /workspace/tests/sample_data.csv
VM target:    /tests/sample_data.csv         (= "/" + CWD-relative path)
VM resolves:  ./tests/sample_data.csv → /tests/sample_data.csv ✓
```

## Implementation

### 1. Add `Files` field to `VsockRequest`

**File:** `src/internal/vm/machine_linux.go` (line ~514)

```go
type VsockRequest struct {
    Code          string            `json:"code"`
    ShowTables    bool              `json:"show_tables"`
    ShowTableMeta bool              `json:"show_table_meta"`
    Files         map[string]string `json:"files,omitempty"` // rel path → base64 content
}
```

### 2. Create file collection logic

**New file:** `src/internal/exec/collect_files.go` (no build tag — testable on all platforms)

- `collectFiles(code string, verbose bool, stderr io.Writer) map[string]string`
- Uses regex to extract string literals from Python code
- Filters by recognized data extensions: `.csv`, `.tsv`, `.json`, `.jsonl`, `.parquet`, `.arrow`, `.txt`, `.xml`, `.yaml`, `.yml`, `.py`
- Resolves each candidate against `os.Getwd()`
- Skips absolute paths, non-existent files, directories
- Reads + base64 encodes files within a 50 MiB total limit
- Returns `nil` when no files found (triggers `omitempty`)

### 3. Wire into `runVM()`

**File:** `src/internal/exec/exec_vm_linux.go` (before line ~118)

```go
files := collectFiles(userCode, cfg.Verbose, cfg.Stderr)

req := &vm.VsockRequest{
    Code:          userCode,
    ShowTables:    cfg.ShowTables,
    ShowTableMeta: cfg.ShowTableMeta,
    Files:         files,
}
```

### 4. VM runner writes files before execution

**File:** `src/internal/vm/vm_runner.py` — modify `handle_request()` (after line ~159)

```python
import base64  # add to top-level imports

# In handle_request, after extracting show_table_meta:
files = request.get("files", {})
for rel_path, b64_content in files.items():
    abs_path = "/" + rel_path.lstrip("/")
    parent = os.path.dirname(abs_path)
    if parent and parent != "/":
        os.makedirs(parent, exist_ok=True)
    with open(abs_path, "wb") as f:
        f.write(base64.b64decode(b64_content))
```

Add `write_files_ms` to the existing `_timing` dict. Update the verbose timing keys in `exec_vm_linux.go` to include it.

### 5. Unit tests

**New file:** `src/internal/exec/collect_files_test.go`

Test cases:
- No string literals in code → returns nil
- String with `.csv` extension that exists → included with correct base64 and relative path key
- String that doesn't exist on disk → skipped silently
- Non-data extension (`.so`, `.exe`) → skipped
- Subdirectory paths (`tests/sample.csv`) → correct relative key
- Size limit enforcement
- Regex extraction of single/double quoted strings

## Files Modified

| File | Change |
|------|--------|
| `src/internal/vm/machine_linux.go` | Add `Files` field to `VsockRequest` |
| `src/internal/exec/collect_files.go` | **New.** `collectFiles()`, regex, extension set |
| `src/internal/exec/collect_files_test.go` | **New.** Unit tests |
| `src/internal/exec/exec_vm_linux.go` | Call `collectFiles()`, pass to request |
| `src/internal/vm/vm_runner.py` | Write files before code execution, timing |

## Verification

```bash
# Build
CGO_ENABLED=0 make build && cp dhg ~/.local/bin/dhg

# Regression — must not regress timing
time DHG_HOME=/workspace/.dhg dhg exec --vm -v ./tests/basic_script.py

# Feature — must work (currently fails)
time DHG_HOME=/workspace/.dhg dhg exec --vm -v ./tests/basic_import.py

# Unit tests
make test
```
