package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/dsmmcken/dh-cli/src/internal/tui/screens"
)

// AppMode determines which screen to show first.
type AppMode int

const (
	WizardMode AppMode = iota
	MenuMode
)

// App is the top-level Bubbletea model holding a screen stack.
type App struct {
	stack  []tea.Model
	width  int
	height int
}

// NewApp creates a new App with the given mode. The dhgHome parameter
// is used by screens that need to access the config directory.
func NewApp(mode AppMode, dhgHome string) App {
	var initial tea.Model
	switch mode {
	case WizardMode:
		initial = screens.NewWelcomeScreen()
	case MenuMode:
		initial = screens.NewMainMenu(dhgHome)
	}
	return App{
		stack: []tea.Model{initial},
	}
}

func (a App) Init() tea.Cmd {
	if len(a.stack) > 0 {
		return a.stack[len(a.stack)-1].Init()
	}
	return nil
}

func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		// Propagate to all screens
		for i, s := range a.stack {
			updated, _ := s.Update(msg)
			a.stack[i] = updated
		}
		return a, nil

	case screens.PushScreenMsg:
		a.stack = append(a.stack, msg.Screen)
		// Send size to new screen
		sized, cmd := msg.Screen.Update(tea.WindowSizeMsg{Width: a.width, Height: a.height})
		a.stack[len(a.stack)-1] = sized
		initCmd := a.stack[len(a.stack)-1].Init()
		return a, tea.Batch(cmd, initCmd)

	case screens.PopScreenMsg:
		if len(a.stack) <= 1 {
			return a, tea.Quit
		}
		a.stack = a.stack[:len(a.stack)-1]
		return a, nil

	case tea.KeyMsg:
		// At root screen, ctrl+c always quits
		if len(a.stack) == 1 {
			switch msg.String() {
			case "ctrl+c":
				return a, tea.Quit
			}
		}
	}

	// Forward to active screen
	if len(a.stack) > 0 {
		active := a.stack[len(a.stack)-1]
		updated, cmd := active.Update(msg)
		a.stack[len(a.stack)-1] = updated
		return a, cmd
	}

	return a, nil
}

func (a App) View() string {
	if len(a.stack) > 0 {
		return a.stack[len(a.stack)-1].View()
	}
	return ""
}

// StackLen returns the number of screens on the stack (for testing).
func (a App) StackLen() int {
	return len(a.stack)
}
