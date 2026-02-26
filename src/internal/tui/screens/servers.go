package screens

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dsmmcken/dh-cli/src/internal/discovery"
)

const serverPollInterval = 3 * time.Second

// ServersLoadedMsg is the message sent when server discovery completes.
// Exported for testing.
type ServersLoadedMsg struct {
	Servers []discovery.Server
	Err     error
}

// ServersPollTickMsg is the periodic poll tick message. Exported for testing.
type ServersPollTickMsg struct{}

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
	status  string // transient status message (e.g. "Killed ...", "Opened ...")
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
	return tea.Batch(discoverServers(), pollServersTick())
}

// Servers returns the current server list (for testing).
func (m ServersScreen) Servers() []discovery.Server {
	return m.servers
}

// Status returns the current status message (for testing).
func (m ServersScreen) Status() string {
	return m.status
}

func discoverServers() tea.Cmd {
	return func() tea.Msg {
		servers, err := discovery.Discover()
		return ServersLoadedMsg{Servers: servers, Err: err}
	}
}

func pollServersTick() tea.Cmd {
	return tea.Tick(serverPollInterval, func(_ time.Time) tea.Msg {
		return ServersPollTickMsg{}
	})
}

func (m ServersScreen) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.help.Width = msg.Width
		return m, nil

	case ServersLoadedMsg:
		m.loading = false
		m.servers = msg.Servers
		m.err = msg.Err
		// Clamp cursor if the list shrunk
		if m.cursor >= len(m.servers) {
			m.cursor = max(0, len(m.servers)-1)
		}
		return m, nil

	case ServersPollTickMsg:
		return m, tea.Batch(discoverServers(), pollServersTick())

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
			if m.cursor < len(m.servers)-1 {
				m.cursor++
			}
		case key.Matches(msg, m.keys.Kill):
			if len(m.servers) > 0 {
				s := m.servers[m.cursor]
				if err := discovery.Kill(s.Port); err != nil {
					m.status = fmt.Sprintf("Error: %s", err)
				} else {
					m.status = fmt.Sprintf("Killed server on port %d", s.Port)
				}
				// Refresh immediately after kill
				return m, discoverServers()
			}
		case key.Matches(msg, m.keys.Open):
			if len(m.servers) > 0 {
				s := m.servers[m.cursor]
				url := fmt.Sprintf("http://localhost:%d", s.Port)
				OpenBrowser(url)
				m.status = fmt.Sprintf("Opened %s", url)
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

	b.WriteString("  Running Deephaven Servers\n\n")

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

	if len(m.servers) == 0 {
		b.WriteString(lipgloss.NewStyle().Foreground(colorDim).Render("  No servers found."))
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
				b.WriteString(lipgloss.NewStyle().Foreground(colorPrimary).Bold(true).Render("  > " + detail))
			} else {
				b.WriteString("    " + detail)
			}
			b.WriteString("\n")
		}
	}

	if m.status != "" {
		b.WriteString("\n")
		b.WriteString("  " + lipgloss.NewStyle().Foreground(colorSuccess).Render(m.status))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(m.help.View(m.keys))

	return b.String()
}

// OpenBrowser opens the given URL in the default browser, with WSL support.
// Exported as a var so tests can replace it with a no-op.
var OpenBrowser = openBrowserImpl

func openBrowserImpl(url string) {
	if isWSL() {
		exec.Command("cmd.exe", "/c", "start", url).Start()
		return
	}
	switch runtime.GOOS {
	case "darwin":
		exec.Command("open", url).Start()
	default:
		exec.Command("xdg-open", url).Start()
	}
}

func isWSL() bool {
	data, err := os.ReadFile("/proc/version")
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(string(data)), "microsoft")
}
