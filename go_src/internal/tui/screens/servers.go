package screens

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dsmmcken/dh-cli/go_src/internal/discovery"
)

type serversLoadedMsg struct {
	servers []discovery.Server
	err     error
}

type serversKeyMap struct {
	Up   key.Binding
	Down key.Binding
	Kill key.Binding
	Open key.Binding
	Help key.Binding
	Back key.Binding
	Quit key.Binding
}

func (k serversKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Kill, k.Open, k.Help, k.Back}
}

func (k serversKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down},
		{k.Kill, k.Open},
		{k.Help, k.Back, k.Quit},
	}
}

type ServersScreen struct {
	keys    serversKeyMap
	help    help.Model
	servers []discovery.Server
	cursor  int
	loading bool
	err     error
	width   int
	height  int
}

func NewServersScreen() ServersScreen {
	return ServersScreen{
		keys: serversKeyMap{
			Up:   key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
			Down: key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
			Kill: key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "kill")),
			Open: key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "open browser")),
			Help: key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "more")),
			Back: key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
			Quit: key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
		},
		help:    help.New(),
		loading: true,
	}
}

func (m ServersScreen) Init() tea.Cmd {
	return discoverServers()
}

func discoverServers() tea.Cmd {
	return func() tea.Msg {
		servers, err := discovery.Discover()
		return serversLoadedMsg{servers: servers, err: err}
	}
}

func (m ServersScreen) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.help.Width = msg.Width
		return m, nil

	case serversLoadedMsg:
		m.loading = false
		m.servers = msg.servers
		m.err = msg.err
		return m, nil

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keys.Up):
			if m.cursor > 0 {
				m.cursor--
			}
		case key.Matches(msg, m.keys.Down):
			if m.cursor < len(m.servers)-1 {
				m.cursor++
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

func (m ServersScreen) View() string {
	var b strings.Builder

	b.WriteString("  Running Servers\n\n")

	if m.loading {
		b.WriteString("  Discovering...\n")
		return b.String()
	}

	if m.err != nil {
		b.WriteString(fmt.Sprintf("  Error: %s\n", m.err))
		b.WriteString("\n")
		b.WriteString(m.help.View(m.keys))
		return b.String()
	}

	primary := lipgloss.AdaptiveColor{Light: "#874BFD", Dark: "#7D56F4"}
	dim := lipgloss.AdaptiveColor{Light: "#999999", Dark: "#666666"}

	if len(m.servers) == 0 {
		b.WriteString(lipgloss.NewStyle().Foreground(dim).Render("  No servers found."))
		b.WriteString("\n")
	} else {
		for i, s := range m.servers {
			detail := fmt.Sprintf(":%d", s.Port)
			if s.PID > 0 {
				detail += fmt.Sprintf("  pid %d", s.PID)
			}
			detail += "  " + s.Source
			if s.Script != "" {
				detail += "   " + s.Script
			}
			if s.ContainerID != "" {
				detail += "   " + s.ContainerID
			}

			if i == m.cursor {
				b.WriteString(lipgloss.NewStyle().Foreground(primary).Bold(true).Render("  > " + detail))
			} else {
				b.WriteString("    " + detail)
			}
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(m.help.View(m.keys))

	return b.String()
}
