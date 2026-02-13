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

// VersionEntry represents a single version row in the combined list.
type VersionEntry struct {
	Version   string
	Installed bool
	IsDefault bool
	DateStr   string // PyPI release date (YYYY-MM-DD), empty if unknown
}

// VersionsListLoadedMsg is the message sent when version data finishes loading.
// Exported for testing.
type VersionsListLoadedMsg struct {
	Entries []VersionEntry
	Dflt    string
	Err     error
}

type versionsKeyMap struct {
	Up        key.Binding
	Down      key.Binding
	Enter     key.Binding
	Install   key.Binding
	Uninstall key.Binding
	Help      key.Binding
	Back      key.Binding
	Quit      key.Binding
}

func (k versionsKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Enter, k.Install, k.Uninstall, k.Help, k.Back}
}

func (k versionsKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down},
		{k.Enter, k.Install, k.Uninstall},
		{k.Help, k.Back, k.Quit},
	}
}

type VersionsScreen struct {
	keys    versionsKeyMap
	help    help.Model
	entries []VersionEntry
	dflt    string
	cursor  int
	loading bool
	err     error
	dhgHome string
	width   int
	height  int
}

func NewVersionsScreen(dhgHome string) VersionsScreen {
	return VersionsScreen{
		keys: versionsKeyMap{
			Up:        key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
			Down:      key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
			Enter:     key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "set default")),
			Install:   key.NewBinding(key.WithKeys("i"), key.WithHelp("i", "install")),
			Uninstall: key.NewBinding(key.WithKeys("u"), key.WithHelp("u", "uninstall")),
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

// Entries returns the combined version entries (for testing).
func (m VersionsScreen) Entries() []VersionEntry {
	return m.entries
}

// Cursor returns the current cursor position (for testing).
func (m VersionsScreen) Cursor() int {
	return m.cursor
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
		if err != nil {
			return VersionsListLoadedMsg{Err: err}
		}

		remote, remoteErr := versions.FetchRemoteVersionsWithDates(20)
		if remoteErr != nil {
			return VersionsListLoadedMsg{Err: remoteErr}
		}

		entries := MergeVersions(remote, installed, dflt)
		return VersionsListLoadedMsg{Entries: entries, Dflt: dflt}
	}
}

// MergeVersions builds a combined list from remote versions and local installs.
// Remote versions form the base list (already sorted descending). Installed
// versions that don't appear in the remote list are prepended.
func MergeVersions(remote []versions.RemoteVersion, installed []versions.InstalledVersion, dflt string) []VersionEntry {
	installedSet := make(map[string]bool, len(installed))
	for _, iv := range installed {
		installedSet[iv.Version] = true
	}

	seen := make(map[string]bool, len(remote))
	var entries []VersionEntry

	for _, rv := range remote {
		seen[rv.Version] = true
		e := VersionEntry{Version: rv.Version, DateStr: rv.Date}
		if installedSet[rv.Version] {
			e.Installed = true
		}
		if rv.Version == dflt {
			e.IsDefault = true
		}
		entries = append(entries, e)
	}

	// Prepend any installed versions not in remote (e.g. yanked or very old).
	var extra []VersionEntry
	for _, iv := range installed {
		if !seen[iv.Version] {
			e := VersionEntry{Version: iv.Version, Installed: true}
			if iv.Version == dflt {
				e.IsDefault = true
			}
			extra = append(extra, e)
		}
	}
	return append(extra, entries...)
}

func (m VersionsScreen) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.help.Width = msg.Width
		return m, nil

	case VersionsListLoadedMsg:
		m.loading = false
		m.entries = msg.Entries
		m.dflt = msg.Dflt
		m.err = msg.Err
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
			if m.cursor < len(m.entries)-1 {
				m.cursor++
			}
		case key.Matches(msg, m.keys.Enter):
			if len(m.entries) > 0 {
				e := m.entries[m.cursor]
				if e.Installed {
					// Already installed: just set as default
					_ = config.Set("default_version", e.Version)
					m.dflt = e.Version
					for i := range m.entries {
						m.entries[i].IsDefault = m.entries[i].Version == e.Version
					}
				} else {
					// Not installed: install then set as default
					return m, pushScreen(NewInstallProgressScreen(m.dhgHome, e.Version))
				}
			}
		case key.Matches(msg, m.keys.Install):
			if len(m.entries) > 0 && !m.entries[m.cursor].Installed {
				return m, pushScreen(NewInstallProgressScreen(m.dhgHome, m.entries[m.cursor].Version))
			}
		case key.Matches(msg, m.keys.Uninstall):
			if len(m.entries) > 0 && m.entries[m.cursor].Installed {
				v := m.entries[m.cursor].Version
				_ = versions.Uninstall(m.dhgHome, v)
				m.entries[m.cursor].Installed = false
				m.entries[m.cursor].DateStr = ""
				if m.entries[m.cursor].IsDefault {
					m.entries[m.cursor].IsDefault = false
					m.dflt = ""
					_ = config.Set("default_version", "")
				}
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

	b.WriteString("  Versions\n\n")

	if m.loading {
		b.WriteString("  Loading...\n")
		return b.String()
	}

	if m.err != nil {
		b.WriteString(fmt.Sprintf("  Error: %s\n", m.err))
		return b.String()
	}

	if len(m.entries) == 0 {
		b.WriteString("  No versions available.\n")
	} else {
		// Find widest version string for alignment.
		maxLen := 0
		for _, e := range m.entries {
			if len(e.Version) > maxLen {
				maxLen = len(e.Version)
			}
		}

		for i, e := range m.entries {
			marker := "  "
			if e.IsDefault {
				marker = "★ "
			}
			label := fmt.Sprintf("%s%-*s", marker, maxLen, e.Version)
			if e.IsDefault {
				label += "  " + lipgloss.NewStyle().Foreground(colorPrimary).Render("default")
			} else {
				label += "  " + strings.Repeat(" ", len("default"))
			}
			if e.Installed {
				label += "  " + lipgloss.NewStyle().Foreground(colorSuccess).Render("installed")
			} else {
				label += "  " + strings.Repeat(" ", len("installed"))
			}
			if e.DateStr != "" {
				label += "  " + lipgloss.NewStyle().Foreground(colorDim).Render(e.DateStr)
			}

			if i == m.cursor {
				b.WriteString(lipgloss.NewStyle().Foreground(colorPrimary).Bold(true).Render("  > " + label))
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
