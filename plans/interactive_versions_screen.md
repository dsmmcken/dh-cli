# Plan: Interactive VersionsScreen for TUI

## Context

The TUI's "Manage Versions" screen (`VersionsScreen` in `src/deephaven_cli/tui/app.py:508-562`) is currently a read-only static text display. The CLI has full version management via `dh install`, `dh uninstall`, `dh use`, and `dh versions --remote`, but none of this is accessible from the TUI. This plan transforms the screen into an interactive version manager matching all CLI capabilities.

## Gap Analysis

| CLI Command | CLI Functionality | TUI Status |
|---|---|---|
| `dh versions` | List installed versions with default marker + install date | **Exists** (static text) |
| `dh versions --remote` | Show available PyPI versions not yet installed | **Missing** |
| `dh install [VERSION]` | Install a version (resolve latest, validate, create venv) | **Missing** |
| `dh uninstall VERSION` | Remove version, clean up default | **Missing** |
| `dh use VERSION` | Set global default version | **Missing** |

## Approach

Follow the `ServersScreen` pattern (lines 565-685) — it's the most recent interactive screen and uses `OptionList` for selectable items with keyboard actions and a status bar.

## Changes (single file: `src/deephaven_cli/tui/app.py`)

### 1. Modify `InstallProgressScreen` (lines 261-335)

Add `pop_count` parameter so it can return to VersionsScreen instead of always pushing DoneScreen (wizard flow).

- `__init__`: Add `pop_count: int = 0` param, `self._done = False`
- `_install_done`: If `pop_count > 0`, pop that many screens and refresh the landing screen. If `pop_count == 0`, push `DoneScreen` (existing wizard behavior preserved).
- Add escape binding + `action_go_back_if_done()` so user can escape after errors
- Also set first installed version as default (existing logic preserved)

### 2. Add `RemoteVersionPickerScreen` (new class, ~70 lines, insert before VersionsScreen)

Launched when user presses `i` on VersionsScreen. Fetches PyPI versions and presents them in an `OptionList`.

- `on_mount` → `run_worker(_fetch_versions, thread=True)` to fetch from PyPI
- Shows versions in OptionList, already-installed versions shown as disabled with "(installed)" label
- Enter on a version → pushes `InstallProgressScreen(version, pop_count=2)` (pops back through picker + progress to VersionsScreen)
- Escape → pop back to VersionsScreen

### 3. Rewrite `VersionsScreen` (lines 508-562 → ~130 lines)

**Layout** (mirrors ServersScreen):
- `#versions-title`: Bold header
- `#version-list`: `OptionList` with selectable installed versions
- `#versions-hint`: Key binding hints
- `#versions-status`: Feedback messages

**Key bindings:**
| Key | Action | Description |
|---|---|---|
| `d` | `set_default` | Set highlighted version as global default |
| `u` / `delete` | `uninstall` | Remove highlighted version |
| `i` | `install_new` | Push RemoteVersionPickerScreen |
| `r` | `toggle_remote` | Toggle showing available PyPI versions in list |
| `escape` | `go_back` | Return to main menu |
| `enter` | (on remote item) | Install the highlighted remote version |

**State:** `self._versions` (installed list), `self._remote_versions` (PyPI minus installed), `self._show_remote` (toggle)

**Actions:**
- `action_set_default()` → calls `set_default_version()` from `manager/config.py`, refreshes list
- `action_uninstall()` → calls `uninstall_version()` from `manager/versions.py`, handles default cleanup (mirrors `cli.py:run_uninstall` logic), refreshes list
- `action_install_new()` → pushes `RemoteVersionPickerScreen`
- `action_toggle_remote()` → toggles `_show_remote`, fetches from PyPI via worker thread on first toggle, appends remote versions to OptionList as dim text with disabled separator/header
- `on_option_list_option_selected()` → if a remote version is highlighted, install it directly

## Key Files

- **Modify:** `src/deephaven_cli/tui/app.py` — VersionsScreen rewrite, InstallProgressScreen tweak, new RemoteVersionPickerScreen
- **Reuse (read-only):** `manager/versions.py` (`install_version`, `uninstall_version`, `list_installed_versions`), `manager/config.py` (`set_default_version`, `get_default_version`, `load_config`, `save_config`, `get_latest_installed_version`), `manager/pypi.py` (`fetch_available_versions`)

## Verification

1. Run `dh` (launches TUI) → navigate to "Manage versions"
2. Verify installed versions are listed with default marker and install dates
3. Press `d` on a non-default version → verify it becomes default
4. Press `r` → verify remote PyPI versions appear below installed ones
5. Press `r` again → verify remote versions disappear
6. Press `i` → verify picker screen shows PyPI versions, installed ones disabled
7. Select a version in picker → verify install progress, then return to versions screen with new version listed
8. Press `u` on a version → verify it's removed from the list
9. Press `escape` → verify return to main menu
