package screens

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dsmmcken/dh-cli/go_src/internal/config"
	"github.com/dsmmcken/dh-cli/go_src/internal/tui/components"
)

type menuItem struct {
	title string
	desc  string
}

type menuKeyMap struct {
	Up    key.Binding
	Down  key.Binding
	Enter key.Binding
	Help  key.Binding
	Quit  key.Binding
}

func (k menuKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Enter, k.Help, k.Quit}
}

func (k menuKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down},
		{k.Enter},
		{k.Help, k.Quit},
	}
}

var defaultMenuKeys = menuKeyMap{
	Up: key.NewBinding(
		key.WithKeys("up", "k"),
		key.WithHelp("↑/k", "up"),
	),
	Down: key.NewBinding(
		key.WithKeys("down", "j"),
		key.WithHelp("↓/j", "down"),
	),
	Enter: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "select"),
	),
	Help: key.NewBinding(
		key.WithKeys("?"),
		key.WithHelp("?", "more"),
	),
	Quit: key.NewBinding(
		key.WithKeys("q", "ctrl+c"),
		key.WithHelp("q", "quit"),
	),
}

type MainMenu struct {
	items    []menuItem
	cursor   int
	keys     menuKeyMap
	help     help.Model
	width    int
	height   int
	dhgHome  string
	status   string
}

func NewMainMenu(dhgHome string) MainMenu {
	items := []menuItem{
		{title: "Manage versions", desc: "Install, remove, and switch between Deephaven versions"},
		{title: "Running servers", desc: "View and manage active Deephaven processes"},
		{title: "Java status", desc: "Check or install Java runtime"},
		{title: "Environment doctor", desc: "Diagnose and fix setup issues"},
		{title: "Configuration", desc: "View and edit settings"},
	}

	// Build status line
	status := buildStatusLine(dhgHome)

	return MainMenu{
		items:   items,
		cursor:  0,
		keys:    defaultMenuKeys,
		help:    help.New(),
		dhgHome: dhgHome,
		status:  status,
	}
}

func buildStatusLine(dhgHome string) string {
	var parts []string

	cfg, err := config.Load()
	if err == nil && cfg.DefaultVersion != "" {
		parts = append(parts, fmt.Sprintf("Active: v%s", cfg.DefaultVersion))
	} else {
		parts = append(parts, "No version set")
	}

	return strings.Join(parts, "  |  ")
}

func (m MainMenu) Init() tea.Cmd {
	return nil
}

func (m MainMenu) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.help.Width = msg.Width
		return m, nil

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keys.Up):
			m.cursor--
			if m.cursor < 0 {
				m.cursor = len(m.items) - 1
			}
		case key.Matches(msg, m.keys.Down):
			m.cursor++
			if m.cursor >= len(m.items) {
				m.cursor = 0
			}
		case key.Matches(msg, m.keys.Enter):
			return m, m.selectItem()
		case key.Matches(msg, m.keys.Help):
			m.help.ShowAll = !m.help.ShowAll
		case key.Matches(msg, m.keys.Quit):
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m MainMenu) selectItem() tea.Cmd {
	switch m.cursor {
	case 0:
		return pushScreen(NewVersionsScreen(m.dhgHome))
	case 1:
		return pushScreen(NewServersScreen())
	case 2:
		return pushScreen(NewJavaCheckScreen(m.dhgHome, false))
	case 3:
		return pushScreen(NewDoctorScreen(m.dhgHome))
	case 4:
		return pushScreen(NewConfigScreen(m.dhgHome))
	}
	return nil
}

func (m MainMenu) View() string {
	var b strings.Builder

	showLogo := m.height >= 20
	showDesc := m.height >= 15

	if showLogo {
		logo := lipgloss.NewStyle().
			Foreground(colorPrimary).
			Render(components.Logo)
		b.WriteString(logo)
		b.WriteString("\n\n")
	}

	// Status line
	b.WriteString("  ")
	b.WriteString(lipgloss.NewStyle().
		Foreground(colorDim).
		Render(m.status))
	b.WriteString("\n\n")

	// Menu items
	for i, item := range m.items {
		if i == m.cursor {
			b.WriteString(lipgloss.NewStyle().Foreground(colorPrimary).Bold(true).Render("  > " + item.title))
		} else {
			b.WriteString("    " + item.title)
		}
		b.WriteString("\n")
		if showDesc {
			b.WriteString(lipgloss.NewStyle().Foreground(colorDim).Render("    " + item.desc))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	// Help bar
	b.WriteString(m.help.View(m.keys))

	return b.String()
}

// Cursor returns the current cursor position (for testing).
func (m MainMenu) Cursor() int {
	return m.cursor
}

// ItemCount returns the number of menu items (for testing).
func (m MainMenu) ItemCount() int {
	return len(m.items)
}
