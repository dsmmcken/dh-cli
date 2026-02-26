# Behaviour Tests — Rules

These tests verify the `dhg` CLI as a **black box**. They are the contract between the tool and its users. This includes both non-interactive CLI commands and the interactive TUI.

Never assume a test failure is expected or okay. If a test fails, it means the tool is not working as intended and needs to be fixed. If you find yourself saying "oh that test always fails, it's just flaky", you are masking a real problem that needs attention.

## Absolute Rules

1. **No implementation knowledge.** Tests must never import, reference, or assume anything about the Go source code, internal packages, struct names, function signatures, or file layout of `src/`. If you find yourself needing to know how something is implemented, you are writing a unit test — put it in `unit_tests/` instead.

2. **Only invoke the compiled binary.** The only way to interact with `dhg` is by running it as a subprocess. No importing Go packages. No calling functions directly.

3. **Assert on public interface only.** You may assert on:
   - **stdout/stderr** content (human text or JSON)
   - **Exit codes** (0, 1, 2, 3, 4, 130)
   - **Files on disk** created/modified by `dhg` (in `~/.dhg/`, `.dhgrc`, etc.)
   - **Absence of output** (e.g. `--quiet` suppresses text, `--json` has no ANSI)
   - **Rendered TUI screen content** (text visible in the virtual terminal buffer)
   - **TUI screen transitions** (what appears after sending keystrokes)

4. **100% feature coverage.** Every command, flag, flag combination, output mode, error condition, TUI screen, and TUI interaction documented in the spec must have a corresponding behaviour test. If a feature exists but has no behaviour test, it is not considered shipped.

5. **Tests must be deterministic.** No reliance on network, system state, or timing. Use isolated temp directories for `~/.dhg/` via `--config-dir` or `DHG_HOME` env var. Mock external dependencies (PyPI, Adoptium API) via environment variables or fixture files where the binary supports it.

---

## Two Test Formats

### 1. CLI Command Tests — testscript (`.txtar`)

For non-interactive commands. Uses `testscript` (`github.com/rogpeppe/go-internal/testscript`). Each `.txtar` file tests one feature area.

```
# Comment describing what this test verifies
exec dhg <command> [flags]
stdout 'expected regex pattern'
stderr 'expected regex pattern'
! stderr .                          # assert no stderr

# For expected failures, prefix with !
! exec dhg kill 99999
stderr 'not found'

# Embedded fixture files
-- config.toml --
default_version = "42.0"
```

**File location:** `testdata/scripts/*.txtar`

**Naming:** One file per feature area: `version.txtar`, `install.txtar`, `java.txtar`, etc. Happy path first, then edge cases, then error cases.

### 2. TUI Tests — go-expect + vt10x

For interactive TUI screens. Spawn the binary in a pseudo-terminal, send keystrokes, assert on rendered screen content.

**Libraries:**
- `github.com/Netflix/go-expect` — Expect-style PTY interaction (send input, wait for output)
- `github.com/hinshun/vt10x` — VT100 terminal emulator (parses ANSI escape sequences into a screen buffer)

**How it works:**
1. Spawn `dhg` in a PTY via go-expect with a vt10x virtual terminal attached
2. Use `console.ExpectString("text")` to wait for specific text to appear
3. Use `console.Send("j")` or `console.SendLine("")` to send keystrokes
4. Read `vt.String()` to get the current rendered screen (ANSI already parsed into plain text)
5. Assert on screen content with `strings.Contains` or golden file comparison

**File location:** `tui_test.go`

**Test naming convention:**
```go
func TestTUI_<Screen>_<Behaviour>(t *testing.T)

// Examples:
func TestTUI_MainMenu_Renders(t *testing.T)
func TestTUI_MainMenu_NavigateWithJ(t *testing.T)
func TestTUI_MainMenu_QuitWithQ(t *testing.T)
func TestTUI_SetupWizard_FullFlow(t *testing.T)
func TestTUI_VersionsScreen_EscGoesBack(t *testing.T)
```

**What to assert on TUI screens:**
- Text content visible on screen (menu items, labels, status text)
- Screen transitions (entering a sub-screen shows expected content, esc returns)
- Navigation (pressing j/k moves the cursor, enter activates)
- Help bar content (pressing ? shows expanded keybindings)
- Clean exit (pressing q terminates the process)

**What NOT to assert on TUI screens:**
- Exact pixel/character positions (too brittle)
- Specific ANSI color codes (use golden files for visual regression instead)
- Internal model state (that's a unit test)

**Golden files** for visual regression: Capture the full vt10x screen buffer as text, store in `testdata/golden/*.golden`. Update with `go test -update`.

**Isolation:** Every TUI test gets its own `DHG_HOME` temp directory so tests don't interfere with each other or the real `~/.dhg/`.

**Timeouts:** Use reasonable expect timeouts (5s default). If a screen doesn't appear within the timeout, the test fails with the last screen content for debugging.

---

## What Goes Here vs `unit_tests/`

| Question | Behaviour test | Unit test |
|----------|---------------|-----------|
| Does `dhg versions --json` output valid JSON? | Yes | No |
| Does the config TOML parser handle missing keys? | No | Yes |
| Does `dhg doctor` report all checks? | Yes | No |
| Does the `/proc/net/tcp` parser handle IPv6? | No | Yes |
| Does `--quiet` suppress human text? | Yes | No |
| Does the main menu show all 5 items? | Yes (TUI test) | No |
| Does pressing j move the cursor down? | Yes (TUI test) | No |
| Does the TUI model update cursor on KeyDown? | No | Yes (teatest) |
| Does the setup wizard complete end-to-end? | Yes (TUI test) | No |
| Does the Huh form validate Java version input? | No | Yes |
| Does `dhg install 42.0` create the version dir? | Yes | No |
| Does version resolution follow the precedence order? | No | Yes |
| Does pressing esc return to the previous screen? | Yes (TUI test) | No |
| Does the screen stack pop correctly? | No | Yes (teatest) |

**Rule of thumb:** If you're testing *what the user sees and does*, it's a behaviour test. If you're testing *how the code works internally*, it's a unit test.

Behaviour tests for the TUI answer: "If I press these keys, do I see the right thing?" Unit tests for the TUI answer: "When this message arrives, does the model state change correctly?"
