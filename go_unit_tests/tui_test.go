package tests

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/dsmmcken/dh-cli/go_src/internal/tui"
	"github.com/dsmmcken/dh-cli/go_src/internal/tui/screens"
	"github.com/stretchr/testify/assert"
)

func TestMainMenu_InitialCursor(t *testing.T) {
	m := screens.NewMainMenu(t.TempDir())
	assert.Equal(t, 0, m.Cursor())
	assert.Equal(t, 5, m.ItemCount())
}

func TestMainMenu_CursorMovesDown(t *testing.T) {
	m := screens.NewMainMenu(t.TempDir())
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	menu := updated.(screens.MainMenu)
	assert.Equal(t, 1, menu.Cursor())
}

func TestMainMenu_CursorMovesUp(t *testing.T) {
	m := screens.NewMainMenu(t.TempDir())
	// Move down first
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	// Then up
	updated, _ = updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	menu := updated.(screens.MainMenu)
	assert.Equal(t, 0, menu.Cursor())
}

func TestMainMenu_CursorWrapsDown(t *testing.T) {
	m := screens.NewMainMenu(t.TempDir())
	// Move down 5 times (past last item)
	var model tea.Model = m
	for i := 0; i < 5; i++ {
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	}
	menu := model.(screens.MainMenu)
	assert.Equal(t, 0, menu.Cursor())
}

func TestMainMenu_CursorWrapsUp(t *testing.T) {
	m := screens.NewMainMenu(t.TempDir())
	// Move up from position 0 should wrap to last
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	menu := updated.(screens.MainMenu)
	assert.Equal(t, 4, menu.Cursor())
}

func TestMainMenu_ViewContainsItems(t *testing.T) {
	m := screens.NewMainMenu(t.TempDir())
	// Set a size first
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	view := updated.View()
	assert.Contains(t, view, "Manage versions")
	assert.Contains(t, view, "Running servers")
	assert.Contains(t, view, "Java status")
	assert.Contains(t, view, "Environment doctor")
	assert.Contains(t, view, "Configuration")
}

func TestMainMenu_ViewHidesLogoWhenShort(t *testing.T) {
	m := screens.NewMainMenu(t.TempDir())
	// Set height < 20
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 15})
	view := updated.View()
	assert.NotContains(t, view, "____")
	assert.Contains(t, view, "Manage versions")
}

func TestMainMenu_ViewShowsDescWhenTall(t *testing.T) {
	m := screens.NewMainMenu(t.TempDir())
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	view := updated.View()
	assert.Contains(t, view, "Install, remove, and switch between Deephaven versions")
}

func TestMainMenu_ViewHidesDescWhenShort(t *testing.T) {
	m := screens.NewMainMenu(t.TempDir())
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 12})
	view := updated.View()
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
	view := m.View()
	assert.Contains(t, view, "Welcome")
	assert.Contains(t, view, "Get Started")
	assert.Contains(t, view, "Java")
}

func TestSetupCommandRegistered(t *testing.T) {
	out, err := execRoot(t, "setup", "--help")
	assert.NoError(t, err)
	assert.Contains(t, out, "non-interactive")
}
