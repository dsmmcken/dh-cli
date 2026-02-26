package screens

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dsmmcken/dh-cli/src/internal/config"
	"github.com/dsmmcken/dh-cli/src/internal/versions"
)

type installProgressMsg struct {
	message string
}

type installDoneMsg struct {
	err error
}

type InstallProgressScreen struct {
	dhgHome  string
	version  string
	progress progress.Model
	status   string
	done     bool
	err      error
	width    int
	height   int
}

func NewInstallProgressScreen(dhgHome, version string) InstallProgressScreen {
	p := progress.New(progress.WithDefaultGradient())
	return InstallProgressScreen{
		dhgHome:  dhgHome,
		version:  version,
		progress: p,
		status:   "Starting installation...",
	}
}

func (m InstallProgressScreen) Init() tea.Cmd {
	return m.doInstall()
}

func (m InstallProgressScreen) doInstall() tea.Cmd {
	dhgHome := m.dhgHome
	version := m.version
	return func() tea.Msg {
		cfg, err := config.Load()
		if err != nil {
			return installDoneMsg{err: err}
		}

		plugins := cfg.Install.Plugins
		if len(plugins) == 0 {
			plugins = []string{
				"deephaven-plugin-ui",
				"deephaven-plugin-plotly-express",
			}
		}

		pythonVer := cfg.Install.PythonVersion
		if pythonVer == "" {
			pythonVer = "3.13"
		}

		err = versions.Install(dhgHome, version, pythonVer, plugins, nil)
		if err != nil {
			return installDoneMsg{err: err}
		}

		// Set as default
		_ = config.Set("default_version", version)

		return installDoneMsg{err: nil}
	}
}

func (m InstallProgressScreen) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.progress.Width = msg.Width - 10
		if m.progress.Width < 20 {
			m.progress.Width = 20
		}
		return m, nil

	case installProgressMsg:
		m.status = msg.message
		return m, nil

	case installDoneMsg:
		m.done = true
		m.err = msg.err
		if msg.err == nil {
			return m, pushScreen(NewDoneScreen(m.version))
		}
		return m, nil

	case progress.FrameMsg:
		progressModel, cmd := m.progress.Update(msg)
		m.progress = progressModel.(progress.Model)
		return m, cmd

	case tea.KeyMsg:
		if m.done && m.err != nil {
			switch msg.String() {
			case "q", "ctrl+c", "enter", "esc":
				return m, tea.Quit
			}
		}
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m InstallProgressScreen) View() string {
	var b strings.Builder

	b.WriteString("  Step 3 of 3 — Installing\n\n")

	if m.done {
		if m.err != nil {
			b.WriteString(fmt.Sprintf("  Error: %s\n\n", m.err))
			b.WriteString(lipgloss.NewStyle().Foreground(colorDim).Render("  Press any key to exit"))
		}
		return b.String()
	}

	b.WriteString(fmt.Sprintf("  Installing Deephaven %s...\n\n", m.version))
	b.WriteString("  " + m.progress.ViewAs(0.5) + "\n\n")
	b.WriteString(fmt.Sprintf("  %s\n", m.status))

	return b.String()
}
