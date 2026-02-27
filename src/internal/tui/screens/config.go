package screens

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dsmmcken/dh-cli/src/internal/config"
)

type configLoadedMsg struct {
	cfg  *config.Config
	path string
	err  error
}

type configKeyMap struct {
	Back key.Binding
	Quit key.Binding
}

func (k configKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Back, k.Quit}
}

func (k configKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{{k.Back, k.Quit}}
}

type ConfigScreen struct {
	keys    configKeyMap
	cfg     *config.Config
	cfgPath string
	loading bool
	err     error
	dhHome string
	width   int
	height  int
}

func NewConfigScreen(dhHome string) ConfigScreen {
	return ConfigScreen{
		keys: configKeyMap{
			Back: key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
			Quit: key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
		},
		loading: true,
		dhHome: dhHome,
	}
}

func (m ConfigScreen) Init() tea.Cmd {
	return func() tea.Msg {
		cfg, err := config.Load()
		return configLoadedMsg{cfg: cfg, path: config.ConfigPath(), err: err}
	}
}

func (m ConfigScreen) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case configLoadedMsg:
		m.loading = false
		m.cfg = msg.cfg
		m.cfgPath = msg.path
		m.err = msg.err
		return m, nil

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keys.Back):
			return m, popScreen()
		case key.Matches(msg, m.keys.Quit):
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m ConfigScreen) View() string {
	var b strings.Builder

	b.WriteString("  Configuration\n\n")

	if m.loading {
		b.WriteString("  Loading...\n")
		return b.String()
	}

	if m.err != nil {
		b.WriteString(fmt.Sprintf("  Error: %s\n", m.err))
		return b.String()
	}

	b.WriteString(fmt.Sprintf("  Config file: %s\n\n", lipgloss.NewStyle().Foreground(colorDim).Render(m.cfgPath)))

	if m.cfg != nil {
		b.WriteString(fmt.Sprintf("  default_version:        %s\n", valueOrNone(m.cfg.DefaultVersion)))
		b.WriteString(fmt.Sprintf("  install.python_version: %s\n", valueOrNone(m.cfg.Install.PythonVersion)))
		if len(m.cfg.Install.Plugins) > 0 {
			b.WriteString(fmt.Sprintf("  install.plugins:        %s\n", strings.Join(m.cfg.Install.Plugins, ", ")))
		} else {
			b.WriteString("  install.plugins:        (none)\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(colorDim).Render("  Use 'dh config set <key> <value>' to modify settings."))
	b.WriteString("\n\n")
	b.WriteString(lipgloss.NewStyle().Foreground(colorDim).Render("  esc back • q quit"))

	return b.String()
}

func valueOrNone(s string) string {
	if s == "" {
		return "(not set)"
	}
	return s
}
