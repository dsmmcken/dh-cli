package screens

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dsmmcken/dh-cli/src/internal/config"
	"github.com/dsmmcken/dh-cli/src/internal/java"
	"github.com/dsmmcken/dh-cli/src/internal/versions"
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
)

type checkResult struct {
	name   string
	status string // "ok", "warning", "error"
	detail string
}

type doctorResultMsg struct {
	checks []checkResult
}

type doctorKeyMap struct {
	Refresh key.Binding
	Back    key.Binding
	Quit    key.Binding
}

func (k doctorKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Refresh, k.Back, k.Quit}
}

func (k doctorKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{{k.Refresh, k.Back, k.Quit}}
}

type DoctorScreen struct {
	keys    doctorKeyMap
	spinner spinner.Model
	loading bool
	checks  []checkResult
	dhgHome string
	width   int
	height  int
}

func NewDoctorScreen(dhgHome string) DoctorScreen {
	s := spinner.New()
	s.Spinner = spinner.Dot
	return DoctorScreen{
		keys: doctorKeyMap{
			Refresh: key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
			Back:    key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
			Quit:    key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
		},
		spinner: s,
		loading: true,
		dhgHome: dhgHome,
	}
}

func (m DoctorScreen) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.runChecks())
}

func (m DoctorScreen) runChecks() tea.Cmd {
	dhgHome := m.dhgHome
	return func() tea.Msg {
		var checks []checkResult

		// Java check
		info, err := java.Detect(dhgHome)
		if err != nil || !info.Found {
			checks = append(checks, checkResult{name: "Java", status: "error", detail: "not found"})
		} else {
			checks = append(checks, checkResult{name: "Java", status: "ok", detail: fmt.Sprintf("%s (%s)", info.Version, info.Source)})
		}

		// Versions check
		installed, err := versions.ListInstalled(dhgHome)
		if err != nil {
			checks = append(checks, checkResult{name: "Versions", status: "warning", detail: fmt.Sprintf("could not list: %s", err)})
		} else if len(installed) == 0 {
			checks = append(checks, checkResult{name: "Versions", status: "warning", detail: "0 installed"})
		} else {
			checks = append(checks, checkResult{name: "Versions", status: "ok", detail: fmt.Sprintf("%d installed", len(installed))})
		}

		// Default version check
		cfg, err := config.Load()
		if err != nil {
			checks = append(checks, checkResult{name: "Default", status: "error", detail: "could not load config"})
		} else if cfg.DefaultVersion == "" {
			checks = append(checks, checkResult{name: "Default", status: "error", detail: "not set"})
		} else {
			checks = append(checks, checkResult{name: "Default", status: "ok", detail: cfg.DefaultVersion})
		}

		// Disk space
		checks = append(checks, checkDiskSpaceTUI(dhgHome))

		return doctorResultMsg{checks: checks}
	}
}

func checkDiskSpaceTUI(dhgHome string) checkResult {
	var stat unix.Statfs_t
	target := dhgHome
	if _, err := os.Stat(target); err != nil {
		target = filepath.Dir(target)
	}
	if err := unix.Statfs(target, &stat); err != nil {
		return checkResult{name: "Disk", status: "warning", detail: fmt.Sprintf("could not check: %s", err)}
	}
	freeBytes := stat.Bavail * uint64(stat.Bsize)
	freeGB := float64(freeBytes) / (1024 * 1024 * 1024)
	status := "ok"
	if freeGB < 5.0 {
		status = "warning"
	}
	return checkResult{name: "Disk", status: status, detail: fmt.Sprintf("%.1f GB free", freeGB)}
}

func (m DoctorScreen) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case doctorResultMsg:
		m.loading = false
		m.checks = msg.checks
		return m, nil

	case spinner.TickMsg:
		if m.loading {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
		return m, nil

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keys.Refresh):
			m.loading = true
			return m, tea.Batch(m.spinner.Tick, m.runChecks())
		case key.Matches(msg, m.keys.Back):
			return m, popScreen()
		case key.Matches(msg, m.keys.Quit):
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m DoctorScreen) View() string {
	var b strings.Builder

	b.WriteString("  Environment Health\n\n")

	if m.loading {
		b.WriteString(fmt.Sprintf("  Running checks...  %s\n", m.spinner.View()))
		return b.String()
	}

	var warnings, errors int
	for _, c := range m.checks {
		var symbol string
		switch c.status {
		case "ok":
			symbol = lipgloss.NewStyle().Foreground(colorSuccess).Render("✓")
		case "warning":
			symbol = lipgloss.NewStyle().Foreground(colorWarning).Render("⚠")
			warnings++
		case "error":
			symbol = lipgloss.NewStyle().Foreground(colorError).Render("✗")
			errors++
		}
		b.WriteString(fmt.Sprintf("  %s %-12s %s\n", symbol, c.name, c.detail))
	}

	b.WriteString("\n")

	if errors > 0 {
		b.WriteString(fmt.Sprintf("  Problems found (%d errors, %d warnings).\n", errors, warnings))
	} else if warnings > 0 {
		b.WriteString(fmt.Sprintf("  Everything looks good (%d warnings).\n", warnings))
	} else {
		b.WriteString("  Everything looks good.\n")
	}

	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(colorDim).Render("  r refresh • esc back • q quit"))

	return b.String()
}
