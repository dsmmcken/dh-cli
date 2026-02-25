package screens

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dsmmcken/dh-cli/go_src/internal/versions"
)

type versionsLoadedMsg struct {
	versions []string
	err      error
}

type VersionPickerScreen struct {
	keys     pickerKeyMap
	spinner  spinner.Model
	loading  bool
	versions []string
	cursor   int
	err      error
	dhgHome  string
	width    int
	height   int
}

type pickerKeyMap struct {
	Up    key.Binding
	Down  key.Binding
	Enter key.Binding
	Quit  key.Binding
}

func NewVersionPickerScreen(dhgHome string) VersionPickerScreen {
	s := spinner.New()
	s.Spinner = spinner.Dot
	return VersionPickerScreen{
		keys: pickerKeyMap{
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
				key.WithHelp("enter", "install"),
			),
			Quit: key.NewBinding(
				key.WithKeys("q", "ctrl+c"),
				key.WithHelp("q", "quit"),
			),
		},
		spinner: s,
		loading: true,
		dhgHome: dhgHome,
	}
}

func (m VersionPickerScreen) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, fetchVersions())
}

func fetchVersions() tea.Cmd {
	return func() tea.Msg {
		vers, err := versions.FetchRemoteVersions(10)
		return versionsLoadedMsg{versions: vers, err: err}
	}
}

func (m VersionPickerScreen) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case versionsLoadedMsg:
		m.loading = false
		m.versions = msg.versions
		m.err = msg.err
		return m, nil

	case spinner.TickMsg:
		if m.loading {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
		return m, nil

	case tea.KeyMsg:
		if m.loading {
			if key.Matches(msg, m.keys.Quit) {
				return m, tea.Quit
			}
			return m, nil
		}

		switch {
		case key.Matches(msg, m.keys.Up):
			if m.cursor > 0 {
				m.cursor--
			}
		case key.Matches(msg, m.keys.Down):
			if m.cursor < len(m.versions)-1 {
				m.cursor++
			}
		case key.Matches(msg, m.keys.Enter):
			if len(m.versions) > 0 {
				selected := m.versions[m.cursor]
				return m, pushScreen(NewInstallProgressScreen(m.dhgHome, selected))
			}
		case key.Matches(msg, m.keys.Quit):
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m VersionPickerScreen) View() string {
	var b strings.Builder

	b.WriteString("  Step 2 of 3 — Deephaven Version\n\n")

	if m.loading {
		b.WriteString(fmt.Sprintf("  Fetching versions...  %s\n", m.spinner.View()))
		return b.String()
	}

	if m.err != nil {
		b.WriteString(fmt.Sprintf("  Error fetching versions: %s\n", m.err))
		return b.String()
	}

	b.WriteString("  Select a version to install:\n\n")

	for i, v := range m.versions {
		label := v
		if i == 0 {
			label += "    latest"
		}
		if i == m.cursor {
			b.WriteString(lipgloss.NewStyle().Foreground(colorPrimary).Bold(true).Render("  > " + label))
		} else {
			b.WriteString("    " + label)
		}
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(colorDim).Render("  ↑/k up • ↓/j down • enter install • q quit"))

	return b.String()
}
