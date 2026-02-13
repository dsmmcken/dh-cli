package screens

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dsmmcken/dh-cli/go_src/internal/config"
	"github.com/dsmmcken/dh-cli/go_src/internal/versions"
)

type versionsListLoadedMsg struct {
	installed []versions.InstalledVersion
	dflt      string
	err       error
}

type versionsKeyMap struct {
	Up        key.Binding
	Down      key.Binding
	SetDflt   key.Binding
	Uninstall key.Binding
	Add       key.Binding
	Remote    key.Binding
	Help      key.Binding
	Back      key.Binding
	Quit      key.Binding
}

func (k versionsKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.SetDflt, k.Uninstall, k.Help, k.Back}
}

func (k versionsKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down},
		{k.SetDflt, k.Uninstall, k.Add, k.Remote},
		{k.Help, k.Back, k.Quit},
	}
}

type VersionsScreen struct {
	keys      versionsKeyMap
	help      help.Model
	installed []versions.InstalledVersion
	dflt      string
	cursor    int
	loading   bool
	err       error
	dhgHome   string
	width     int
	height    int
}

func NewVersionsScreen(dhgHome string) VersionsScreen {
	return VersionsScreen{
		keys: versionsKeyMap{
			Up:        key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
			Down:      key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
			SetDflt:   key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "default")),
			Uninstall: key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "uninstall")),
			Add:       key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "add new")),
			Remote:    key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "toggle remote")),
			Help:      key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "more")),
			Back:      key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
			Quit:      key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
		},
		help:    help.New(),
		loading: true,
		dhgHome: dhgHome,
	}
}

func (m VersionsScreen) Init() tea.Cmd {
	return m.loadVersions()
}

func (m VersionsScreen) loadVersions() tea.Cmd {
	dhgHome := m.dhgHome
	return func() tea.Msg {
		cfg, _ := config.Load()
		dflt := ""
		if cfg != nil {
			dflt = cfg.DefaultVersion
		}
		installed, err := versions.ListInstalled(dhgHome)
		return versionsListLoadedMsg{installed: installed, dflt: dflt, err: err}
	}
}

func (m VersionsScreen) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.help.Width = msg.Width
		return m, nil

	case versionsListLoadedMsg:
		m.loading = false
		m.installed = msg.installed
		m.dflt = msg.dflt
		m.err = msg.err
		return m, nil

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keys.Up):
			if m.cursor > 0 {
				m.cursor--
			}
		case key.Matches(msg, m.keys.Down):
			if m.cursor < len(m.installed)-1 {
				m.cursor++
			}
		case key.Matches(msg, m.keys.SetDflt):
			if len(m.installed) > 0 {
				v := m.installed[m.cursor].Version
				_ = config.Set("default_version", v)
				m.dflt = v
			}
		case key.Matches(msg, m.keys.Help):
			m.help.ShowAll = !m.help.ShowAll
		case key.Matches(msg, m.keys.Back):
			return m, popScreen()
		case key.Matches(msg, m.keys.Quit):
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m VersionsScreen) View() string {
	var b strings.Builder

	b.WriteString("  Installed Versions\n\n")

	if m.loading {
		b.WriteString("  Loading...\n")
		return b.String()
	}

	if m.err != nil {
		b.WriteString(fmt.Sprintf("  Error: %s\n", m.err))
		return b.String()
	}

	primary := lipgloss.AdaptiveColor{Light: "#874BFD", Dark: "#7D56F4"}
	dim := lipgloss.AdaptiveColor{Light: "#999999", Dark: "#666666"}

	if len(m.installed) == 0 {
		b.WriteString("  No versions installed.\n")
		b.WriteString(lipgloss.NewStyle().Foreground(dim).Render("  Press 'a' to add a version."))
		b.WriteString("\n")
	} else {
		for i, v := range m.installed {
			marker := "  "
			if v.Version == m.dflt {
				marker = "★ "
			}
			label := fmt.Sprintf("%s%s", marker, v.Version)
			if !v.InstalledAt.IsZero() {
				label += "   " + lipgloss.NewStyle().Foreground(dim).Render("installed "+v.InstalledAt.Format("2006-01-02"))
			}
			if v.Version == m.dflt {
				label += " " + lipgloss.NewStyle().Foreground(dim).Render("default")
			}

			if i == m.cursor {
				b.WriteString(lipgloss.NewStyle().Foreground(primary).Bold(true).Render("  > " + label))
			} else {
				b.WriteString("    " + label)
			}
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(m.help.View(m.keys))

	return b.String()
}
