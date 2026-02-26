package screens

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dsmmcken/dh-cli/src/internal/tui/components"
)

type welcomeKeyMap struct {
	Enter key.Binding
	Quit  key.Binding
}

func (k welcomeKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Enter, k.Quit}
}

func (k welcomeKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{{k.Enter, k.Quit}}
}

type WelcomeScreen struct {
	keys   welcomeKeyMap
	width  int
	height int
}

func NewWelcomeScreen() WelcomeScreen {
	return WelcomeScreen{
		keys: welcomeKeyMap{
			Enter: key.NewBinding(
				key.WithKeys("enter"),
				key.WithHelp("enter", "continue"),
			),
			Quit: key.NewBinding(
				key.WithKeys("q", "ctrl+c"),
				key.WithHelp("q", "quit"),
			),
		},
	}
}

func (m WelcomeScreen) Init() tea.Cmd {
	return nil
}

func (m WelcomeScreen) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keys.Enter):
			return m, pushScreen(NewJavaCheckScreen("", true))
		case key.Matches(msg, m.keys.Quit):
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m WelcomeScreen) View() string {
	var b strings.Builder

	logo := lipgloss.NewStyle().
		Foreground(colorPrimary).
		Render(components.Logo)
	b.WriteString(logo)
	b.WriteString("\n\n")

	b.WriteString("  Welcome! Let's get your environment ready.\n\n")
	b.WriteString("  This wizard will:\n")
	b.WriteString("    1. Check for Java (or install it)\n")
	b.WriteString("    2. Install a Deephaven engine version\n")
	b.WriteString("    3. Get you started\n\n")

	b.WriteString(lipgloss.NewStyle().Foreground(colorPrimary).Bold(true).Render("  > Get Started"))
	b.WriteString("\n\n")

	b.WriteString(lipgloss.NewStyle().Foreground(colorDim).Render("  enter continue • q quit"))

	return b.String()
}
