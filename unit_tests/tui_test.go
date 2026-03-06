package tests

import (
	"os"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/dsmmcken/dh-cli/src/internal/discovery"
	"github.com/dsmmcken/dh-cli/src/internal/repl"
	"github.com/dsmmcken/dh-cli/src/internal/tui"
	"github.com/dsmmcken/dh-cli/src/internal/tui/screens"
	"github.com/dsmmcken/dh-cli/src/internal/versions"
	"github.com/dsmmcken/dh-cli/src/internal/vm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMainMenu_InitialCursor(t *testing.T) {
	m := screens.NewMainMenu(t.TempDir())
	assert.Equal(t, 0, m.Cursor())
	assert.Equal(t, 6, m.ItemCount())
}

func TestMainMenu_CursorMovesDown(t *testing.T) {
	m := screens.NewMainMenu(t.TempDir())
	updated, _ := m.Update(tea.KeyPressMsg{Text: "j"})
	menu := updated.(screens.MainMenu)
	assert.Equal(t, 1, menu.Cursor())
}

func TestMainMenu_CursorMovesUp(t *testing.T) {
	m := screens.NewMainMenu(t.TempDir())
	// Move down first
	updated, _ := m.Update(tea.KeyPressMsg{Text: "j"})
	// Then up
	updated, _ = updated.Update(tea.KeyPressMsg{Text: "k"})
	menu := updated.(screens.MainMenu)
	assert.Equal(t, 0, menu.Cursor())
}

func TestMainMenu_CursorWrapsDown(t *testing.T) {
	m := screens.NewMainMenu(t.TempDir())
	// Move down 6 times (past last item, wraps to 0)
	var model tea.Model = m
	for i := 0; i < 6; i++ {
		model, _ = model.Update(tea.KeyPressMsg{Text: "j"})
	}
	menu := model.(screens.MainMenu)
	assert.Equal(t, 0, menu.Cursor())
}

func TestMainMenu_CursorWrapsUp(t *testing.T) {
	m := screens.NewMainMenu(t.TempDir())
	// Move up from position 0 should wrap to last
	updated, _ := m.Update(tea.KeyPressMsg{Text: "k"})
	menu := updated.(screens.MainMenu)
	assert.Equal(t, 5, menu.Cursor())
}

func TestMainMenu_ViewContainsItems(t *testing.T) {
	m := screens.NewMainMenu(t.TempDir())
	// Set a size first
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	view := updated.View().Content
	assert.Contains(t, view, "Manage versions")
	assert.Contains(t, view, "Running servers")
	assert.Contains(t, view, "Java status")
	assert.Contains(t, view, "Environment doctor")
	assert.Contains(t, view, "Configuration")
	assert.Contains(t, view, "VM management")
}

func TestMainMenu_ViewHidesLogoWhenShort(t *testing.T) {
	m := screens.NewMainMenu(t.TempDir())
	// Set height < 20
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 15})
	view := updated.View().Content
	assert.NotContains(t, view, "____")
	assert.Contains(t, view, "Manage versions")
}

func TestMainMenu_ViewShowsDescWhenTall(t *testing.T) {
	m := screens.NewMainMenu(t.TempDir())
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	view := updated.View().Content
	assert.Contains(t, view, "Install, remove, and switch between Deephaven versions")
}

func TestMainMenu_ViewHidesDescWhenShort(t *testing.T) {
	m := screens.NewMainMenu(t.TempDir())
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 12})
	view := updated.View().Content
	assert.NotContains(t, view, "Install, remove, and switch between Deephaven versions")
}

func TestApp_StackPushPop(t *testing.T) {
	app := tui.NewApp(tui.MenuMode, t.TempDir())
	assert.Equal(t, 1, app.StackLen())

	// Simulate a push
	newScreen := screens.NewDoneScreen("1.0")
	updated, _ := app.Update(screens.PushScreenMsg{Screen: newScreen})
	app2 := updated.(tui.App)
	assert.Equal(t, 2, app2.StackLen())

	// Simulate a pop
	updated, _ = app2.Update(screens.PopScreenMsg{})
	app3 := updated.(tui.App)
	assert.Equal(t, 1, app3.StackLen())
}

func TestApp_PopAtRootQuits(t *testing.T) {
	app := tui.NewApp(tui.MenuMode, t.TempDir())

	// Pop at root should produce a quit command
	_, cmd := app.Update(screens.PopScreenMsg{})
	assert.NotNil(t, cmd)
}

func TestWelcomeScreen_ViewContents(t *testing.T) {
	m := screens.NewWelcomeScreen()
	view := m.View().Content
	assert.Contains(t, view, "Welcome")
	assert.Contains(t, view, "Get Started")
	assert.Contains(t, view, "Java")
}

func TestSetupCommandRegistered(t *testing.T) {
	out, err := execRoot(t, "setup", "--help")
	assert.NoError(t, err)
	assert.Contains(t, out, "non-interactive")
}

// --- VersionsScreen tests ---

func TestMergeVersions_RemoteOnly(t *testing.T) {
	remote := []versions.RemoteVersion{
		{Version: "41.1", Date: "2026-01-29"},
		{Version: "41.0", Date: "2026-01-06"},
		{Version: "0.40.9", Date: "2026-01-28"},
	}
	entries := screens.MergeVersions(remote, nil, "")

	require.Len(t, entries, 3)
	assert.Equal(t, "41.1", entries[0].Version)
	assert.False(t, entries[0].Installed)
	assert.Equal(t, "2026-01-29", entries[0].DateStr)
	assert.Equal(t, "41.0", entries[1].Version)
	assert.Equal(t, "0.40.9", entries[2].Version)
}

func TestMergeVersions_InstalledMarked(t *testing.T) {
	remote := []versions.RemoteVersion{
		{Version: "41.1", Date: "2026-01-29"},
		{Version: "41.0", Date: "2026-01-06"},
		{Version: "0.40.9", Date: "2026-01-28"},
	}
	installed := []versions.InstalledVersion{
		{Version: "41.0", InstalledAt: time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC)},
	}
	entries := screens.MergeVersions(remote, installed, "41.0")

	require.Len(t, entries, 3)
	// 41.1 not installed
	assert.False(t, entries[0].Installed)
	assert.False(t, entries[0].IsDefault)
	// 41.0 installed + default — date comes from PyPI, not install time
	assert.True(t, entries[1].Installed)
	assert.True(t, entries[1].IsDefault)
	assert.Equal(t, "2026-01-06", entries[1].DateStr)
	// 0.40.9 not installed
	assert.False(t, entries[2].Installed)
}

func TestMergeVersions_InstalledNotInRemote(t *testing.T) {
	remote := []versions.RemoteVersion{
		{Version: "41.1", Date: "2026-01-29"},
		{Version: "41.0", Date: "2026-01-06"},
	}
	installed := []versions.InstalledVersion{
		{Version: "0.35.0", InstalledAt: time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)},
	}
	entries := screens.MergeVersions(remote, installed, "0.35.0")

	// 0.35.0 should be prepended before the remote list, no PyPI date available
	require.Len(t, entries, 3)
	assert.Equal(t, "0.35.0", entries[0].Version)
	assert.True(t, entries[0].Installed)
	assert.True(t, entries[0].IsDefault)
	assert.Equal(t, "", entries[0].DateStr)
	assert.Equal(t, "41.1", entries[1].Version)
	assert.Equal(t, "41.0", entries[2].Version)
}

func TestVersionsScreen_ViewShowsLoading(t *testing.T) {
	m := screens.NewVersionsScreen(t.TempDir())
	view := m.View().Content
	assert.Contains(t, view, "Loading...")
}

func TestVersionsScreen_ViewShowsTitle(t *testing.T) {
	m := screens.NewVersionsScreen(t.TempDir())
	view := m.View().Content
	assert.Contains(t, view, "Versions")
}

func TestVersionsScreen_NoRemovedKeyBindings(t *testing.T) {
	m := screens.NewVersionsScreen(t.TempDir())
	view := m.View().Content
	assert.NotContains(t, view, "toggle remote")
	assert.NotContains(t, view, "add new")
}

func TestVersionsScreen_HasExpectedKeyBindings(t *testing.T) {
	entries := []screens.VersionEntry{
		{Version: "41.1", Installed: false},
	}
	m := versionsScreenWithEntries(entries, "")
	// Trigger full help to see all bindings
	updated, _ := m.Update(tea.KeyPressMsg{Text: "?"})
	view := updated.View().Content
	assert.Contains(t, view, "set default")
	assert.Contains(t, view, "install")
	assert.Contains(t, view, "uninstall")
}

// helper to build a VersionsScreen pre-loaded with entries (bypasses async load).
func versionsScreenWithEntries(entries []screens.VersionEntry, dflt string) screens.VersionsScreen {
	m := screens.NewVersionsScreen("")
	// Simulate the loaded message
	loaded := tea.Msg(screens.VersionsListLoadedMsg{Entries: entries, Dflt: dflt})
	updated, _ := m.Update(loaded)
	return updated.(screens.VersionsScreen)
}

func TestVersionsScreen_EnterOnInstalledSetsDefault(t *testing.T) {
	entries := []screens.VersionEntry{
		{Version: "41.1", Installed: true},
		{Version: "41.0", Installed: true, IsDefault: true},
	}
	m := versionsScreenWithEntries(entries, "41.0")

	// Cursor is on 41.1 (index 0), press enter
	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	vs := updated.(screens.VersionsScreen)

	// Should set default locally (no push screen)
	assert.Nil(t, cmd)
	assert.True(t, vs.Entries()[0].IsDefault)
	assert.False(t, vs.Entries()[1].IsDefault)
}

func TestVersionsScreen_EnterOnNotInstalledPushesInstall(t *testing.T) {
	entries := []screens.VersionEntry{
		{Version: "41.1", Installed: false},
	}
	m := versionsScreenWithEntries(entries, "")

	// Press enter on uninstalled version
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	// Should produce a PushScreenMsg (install)
	assert.NotNil(t, cmd)
}

func TestVersionsScreen_IOnNotInstalledPushesInstall(t *testing.T) {
	entries := []screens.VersionEntry{
		{Version: "41.1", Installed: false},
	}
	m := versionsScreenWithEntries(entries, "")

	_, cmd := m.Update(tea.KeyPressMsg{Text: "i"})
	assert.NotNil(t, cmd)
}

func TestVersionsScreen_IOnInstalledDoesNothing(t *testing.T) {
	entries := []screens.VersionEntry{
		{Version: "41.1", Installed: true},
	}
	m := versionsScreenWithEntries(entries, "")

	_, cmd := m.Update(tea.KeyPressMsg{Text: "i"})
	assert.Nil(t, cmd)
}

func TestVersionsScreen_UOnInstalledUninstalls(t *testing.T) {
	tmp := t.TempDir()
	// Create a fake installed version directory
	vDir := tmp + "/versions/0.36.0"
	require.NoError(t, os.MkdirAll(vDir, 0o755))
	require.NoError(t, versions.WriteMeta(vDir, &versions.Meta{InstalledAt: time.Now()}))

	entries := []screens.VersionEntry{
		{Version: "0.36.0", Installed: true, IsDefault: true, DateStr: "2025-01-01"},
	}
	m := screens.NewVersionsScreen(tmp)
	loaded := tea.Msg(screens.VersionsListLoadedMsg{Entries: entries, Dflt: "0.36.0"})
	updated, _ := m.Update(loaded)
	vs := updated.(screens.VersionsScreen)

	// Press u to uninstall
	updated, cmd := vs.Update(tea.KeyPressMsg{Text: "u"})
	vs = updated.(screens.VersionsScreen)

	assert.Nil(t, cmd)
	assert.False(t, vs.Entries()[0].Installed)
	assert.False(t, vs.Entries()[0].IsDefault)
	assert.Equal(t, "", vs.Entries()[0].DateStr)
}

func TestVersionsScreen_UOnNotInstalledDoesNothing(t *testing.T) {
	entries := []screens.VersionEntry{
		{Version: "41.1", Installed: false},
	}
	m := versionsScreenWithEntries(entries, "")

	updated, cmd := m.Update(tea.KeyPressMsg{Text: "u"})
	vs := updated.(screens.VersionsScreen)

	assert.Nil(t, cmd)
	assert.False(t, vs.Entries()[0].Installed)
}

// --- ServersScreen tests ---

func serversScreenWithServers(servers []discovery.Server) screens.ServersScreen {
	m := screens.NewServersScreen()
	loaded := tea.Msg(screens.ServersLoadedMsg{Servers: servers})
	updated, _ := m.Update(loaded)
	return updated.(screens.ServersScreen)
}

func TestServersScreen_ViewShowsDiscovering(t *testing.T) {
	m := screens.NewServersScreen()
	view := m.View().Content
	assert.Contains(t, view, "Discovering...")
}

func TestServersScreen_ViewShowsTitle(t *testing.T) {
	m := screens.NewServersScreen()
	view := m.View().Content
	assert.Contains(t, view, "Running Deephaven Servers")
}

func TestServersScreen_ViewShowsNoServers(t *testing.T) {
	m := serversScreenWithServers(nil)
	view := m.View().Content
	assert.Contains(t, view, "No servers found")
}

func TestServersScreen_ViewShowsServerList(t *testing.T) {
	servers := []discovery.Server{
		{Port: 10000, PID: 1234, Source: "java"},
		{Port: 8080, PID: 5678, Source: "dh serve"},
	}
	m := serversScreenWithServers(servers)
	view := m.View().Content
	assert.Contains(t, view, ":10000")
	assert.Contains(t, view, ":8080")
	assert.Contains(t, view, "dh serve")
}

func TestServersScreen_HasExpectedKeyBindings(t *testing.T) {
	m := serversScreenWithServers(nil)
	updated, _ := m.Update(tea.KeyPressMsg{Text: "?"})
	view := updated.View().Content
	assert.Contains(t, view, "kill")
	assert.Contains(t, view, "open browser")
}

func TestServersScreen_OpenSetsStatus(t *testing.T) {
	// Replace browser launcher with no-op so the test doesn't open a real browser.
	orig := screens.OpenBrowser
	screens.OpenBrowser = func(string) {}
	t.Cleanup(func() { screens.OpenBrowser = orig })

	servers := []discovery.Server{
		{Port: 10000, PID: 1234, Source: "java"},
	}
	m := serversScreenWithServers(servers)

	updated, _ := m.Update(tea.KeyPressMsg{Text: "o"})
	ss := updated.(screens.ServersScreen)
	assert.Contains(t, ss.Status(), "Opened http://localhost:10000")
}

func TestServersScreen_OpenOnEmptyDoesNothing(t *testing.T) {
	m := serversScreenWithServers(nil)
	updated, _ := m.Update(tea.KeyPressMsg{Text: "o"})
	ss := updated.(screens.ServersScreen)
	assert.Equal(t, "", ss.Status())
}

func TestServersScreen_KillOnEmptyDoesNothing(t *testing.T) {
	m := serversScreenWithServers(nil)
	updated, cmd := m.Update(tea.KeyPressMsg{Text: "x"})
	ss := updated.(screens.ServersScreen)
	assert.Nil(t, cmd)
	assert.Equal(t, "", ss.Status())
}

func TestServersScreen_PollTickTriggersRefresh(t *testing.T) {
	m := serversScreenWithServers(nil)
	_, cmd := m.Update(screens.ServersPollTickMsg{})
	// Poll tick should produce commands (refresh + next tick)
	assert.NotNil(t, cmd)
}

// --- REPL input tests ---

func TestInput_ShiftEnterInsertsNewline(t *testing.T) {
	input := repl.NewInput(t.TempDir() + "/history")
	input.SetWidth(80)

	// Type some text first.
	input, _ = input.Update(tea.KeyPressMsg{Text: "a"})
	assert.Equal(t, "a", input.Value())

	// Shift+enter should insert a newline, not submit.
	shiftEnter := tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModShift}
	input, cmd := input.Update(shiftEnter)
	assert.Contains(t, input.Value(), "\n", "shift+enter should insert a newline")
	// No InputCompleteMsg should be produced (i.e., input should not be submitted).
	assert.Nil(t, cmd, "shift+enter should not produce a command (no submit)")
}

func TestInput_CtrlOInsertsNewline(t *testing.T) {
	input := repl.NewInput(t.TempDir() + "/history")
	input.SetWidth(80)

	// Type some text first.
	input, _ = input.Update(tea.KeyPressMsg{Text: "x"})

	// Ctrl+o is the fallback for newline insertion.
	ctrlO := tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl}
	input, cmd := input.Update(ctrlO)
	assert.Contains(t, input.Value(), "\n", "ctrl+o should insert a newline")
	assert.Nil(t, cmd, "ctrl+o should not produce a command (no submit)")
}

// --- VMManageScreen tests ---

func TestVMManageScreen_ViewShowsLoading(t *testing.T) {
	m := screens.NewVMManageScreen(t.TempDir())
	view := m.View().Content
	assert.Contains(t, view, "Loading...")
	assert.True(t, m.Loading())
}

func TestVMManageScreen_ViewShowsTitle(t *testing.T) {
	m := screens.NewVMManageScreen(t.TempDir())
	view := m.View().Content
	assert.Contains(t, view, "VM Management")
}

func TestVMManageScreen_ShowsPrereqs(t *testing.T) {
	m := screens.NewVMManageScreen("")
	loaded := screens.VMStatusLoadedMsg{
		Prereqs: []*vm.PrereqError{
			{Check: "platform", Message: "VM mode requires Linux with KVM support"},
		},
	}
	updated, _ := m.Update(loaded)
	vs := updated.(screens.VMManageScreen)
	view := vs.View().Content
	assert.Contains(t, view, "Prerequisites")
	assert.Contains(t, view, "platform")
}

func TestVMManageScreen_ShowsAllPrereqsMet(t *testing.T) {
	m := screens.NewVMManageScreen("")
	loaded := screens.VMStatusLoadedMsg{}
	updated, _ := m.Update(loaded)
	vs := updated.(screens.VMManageScreen)
	view := vs.View().Content
	assert.Contains(t, view, "All prerequisites met")
}

func TestVMManageScreen_ShowsNoSnapshots(t *testing.T) {
	m := screens.NewVMManageScreen("")
	loaded := screens.VMStatusLoadedMsg{}
	updated, _ := m.Update(loaded)
	vs := updated.(screens.VMManageScreen)
	view := vs.View().Content
	assert.Contains(t, view, "No snapshots found")
}

func TestVMManageScreen_ShowsPoolNotRunning(t *testing.T) {
	m := screens.NewVMManageScreen("")
	loaded := screens.VMStatusLoadedMsg{}
	updated, _ := m.Update(loaded)
	vs := updated.(screens.VMManageScreen)
	view := vs.View().Content
	assert.Contains(t, view, "not running")
}

func TestVMManageScreen_ShowsPoolRunning(t *testing.T) {
	m := screens.NewVMManageScreen("")
	loaded := screens.VMStatusLoadedMsg{
		PoolInfo: &vm.PoolStatus{
			Running:     true,
			PID:         12345,
			Version:     "0.36.3",
			Ready:       1,
			TargetSize:  1,
			IdleSeconds: 42,
		},
	}
	updated, _ := m.Update(loaded)
	vs := updated.(screens.VMManageScreen)
	view := vs.View().Content
	assert.Contains(t, view, "running")
	assert.Contains(t, view, "12345")
	assert.Contains(t, view, "0.36.3")
}

func TestVMManageScreen_CursorNavigation(t *testing.T) {
	m := screens.NewVMManageScreen("")
	loaded := screens.VMStatusLoadedMsg{
		Snapshots: []screens.SnapshotEntry{
			{Version: "0.36.3", Ready: true},
			{Version: "0.36.2", Ready: false},
		},
	}
	updated, _ := m.Update(loaded)
	vs := updated.(screens.VMManageScreen)
	assert.Equal(t, 0, vs.Cursor())

	// Move down
	updated, _ = vs.Update(tea.KeyPressMsg{Text: "j"})
	vs = updated.(screens.VMManageScreen)
	assert.Equal(t, 1, vs.Cursor())

	// Move down again (should clamp, not wrap)
	updated, _ = vs.Update(tea.KeyPressMsg{Text: "j"})
	vs = updated.(screens.VMManageScreen)
	assert.Equal(t, 1, vs.Cursor())

	// Move up
	updated, _ = vs.Update(tea.KeyPressMsg{Text: "k"})
	vs = updated.(screens.VMManageScreen)
	assert.Equal(t, 0, vs.Cursor())
}

func TestVMManageScreen_EscGoesBack(t *testing.T) {
	m := screens.NewVMManageScreen("")
	loaded := screens.VMStatusLoadedMsg{}
	updated, _ := m.Update(loaded)
	vs := updated.(screens.VMManageScreen)
	_, cmd := vs.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	assert.NotNil(t, cmd)
}

func TestVMManageScreen_HasExpectedKeyBindings(t *testing.T) {
	m := screens.NewVMManageScreen("")
	loaded := screens.VMStatusLoadedMsg{}
	updated, _ := m.Update(loaded)
	vs := updated.(screens.VMManageScreen)
	// Toggle full help
	updated, _ = vs.Update(tea.KeyPressMsg{Text: "?"})
	view := updated.View().Content
	assert.Contains(t, view, "prepare")
	assert.Contains(t, view, "delete")
	assert.Contains(t, view, "start pool")
	assert.Contains(t, view, "stop pool")
}

func TestVMManageScreen_PollTickTriggersRefresh(t *testing.T) {
	m := screens.NewVMManageScreen("")
	loaded := screens.VMStatusLoadedMsg{}
	updated, _ := m.Update(loaded)
	vs := updated.(screens.VMManageScreen)
	_, cmd := vs.Update(screens.VMPollTickMsg{})
	assert.NotNil(t, cmd)
}

func TestVMManageScreen_DeleteOnEmptyDoesNothing(t *testing.T) {
	m := screens.NewVMManageScreen("")
	loaded := screens.VMStatusLoadedMsg{}
	updated, _ := m.Update(loaded)
	vs := updated.(screens.VMManageScreen)
	updated, cmd := vs.Update(tea.KeyPressMsg{Text: "d"})
	assert.Nil(t, cmd)
	vs2 := updated.(screens.VMManageScreen)
	assert.Equal(t, "", vs2.Status())
}

func TestVMManageScreen_SnapshotsInView(t *testing.T) {
	m := screens.NewVMManageScreen("")
	loaded := screens.VMStatusLoadedMsg{
		Snapshots: []screens.SnapshotEntry{
			{Version: "0.36.3", Ready: true},
			{Version: "0.36.2", Ready: false},
		},
	}
	updated, _ := m.Update(loaded)
	vs := updated.(screens.VMManageScreen)
	view := vs.View().Content
	assert.Contains(t, view, "0.36.3")
	assert.Contains(t, view, "0.36.2")
	assert.Contains(t, view, "ready")
	assert.Contains(t, view, "incomplete")
}
