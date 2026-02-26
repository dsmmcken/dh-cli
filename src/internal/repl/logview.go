package repl

import (
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dsmmcken/dh-cli/src/internal/tui"
)

// LogEntryType identifies the kind of log entry for styling.
type LogEntryType string

const (
	LogCommand LogEntryType = "command"
	LogStdout  LogEntryType = "stdout"
	LogStderr  LogEntryType = "stderr"
	LogError   LogEntryType = "error"
	LogResult  LogEntryType = "result"
	LogInfo    LogEntryType = "info"
)

// LogEntry represents a single styled entry in the log view.
type LogEntry struct {
	Type LogEntryType
	Text string
}

// LogViewModel is a scrollable log output component.
type LogViewModel struct {
	entries  []LogEntry
	viewport viewport.Model
	width    int
	height   int
	ready    bool
}

// NewLogView creates an empty log view.
func NewLogView() LogViewModel {
	return LogViewModel{
		entries: []LogEntry{},
	}
}

// SetSize updates the viewport dimensions.
func (m *LogViewModel) SetSize(width, height int) {
	m.width = width
	m.height = height
	if !m.ready {
		m.viewport = viewport.New(width, height)
		m.viewport.YPosition = 0
		m.ready = true
	} else {
		m.viewport.Width = width
		m.viewport.Height = height
	}
	m.renderContent()
}

// AppendEntry adds a log entry and auto-scrolls to bottom.
func (m *LogViewModel) AppendEntry(entry LogEntry) {
	m.entries = append(m.entries, entry)
	m.renderContent()
	m.viewport.GotoBottom()
}

// AppendEntries adds multiple entries at once.
func (m *LogViewModel) AppendEntries(entries []LogEntry) {
	m.entries = append(m.entries, entries...)
	m.renderContent()
	m.viewport.GotoBottom()
}

func (m *LogViewModel) renderContent() {
	if !m.ready {
		return
	}
	var lines []string
	for _, e := range m.entries {
		styled := m.styleEntry(e)
		lines = append(lines, styled)
	}
	m.viewport.SetContent(strings.Join(lines, "\n"))
}

func (m *LogViewModel) styleEntry(e LogEntry) string {
	switch e.Type {
	case LogCommand:
		parts := strings.Split(e.Text, "\n")
		for i, l := range parts {
			if i == 0 {
				parts[i] = tui.StyleDim.Render("> " + l)
			} else {
				parts[i] = tui.StyleDim.Render("  " + l)
			}
		}
		return strings.Join(parts, "\n")
	case LogStdout:
		return e.Text
	case LogStderr:
		return tui.StyleWarning.Render(e.Text)
	case LogError:
		return tui.StyleError.Render(e.Text)
	case LogResult:
		return lipgloss.NewStyle().Foreground(tui.ColorPrimary).Render(e.Text)
	case LogInfo:
		return tui.StyleDim.Render(e.Text)
	default:
		return e.Text
	}
}

// Update handles viewport-specific messages (scroll, mouse).
func (m LogViewModel) Update(msg tea.Msg) (LogViewModel, tea.Cmd) {
	if !m.ready {
		return m, nil
	}
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

// View renders the viewport.
func (m LogViewModel) View() string {
	if !m.ready {
		return "Initializing..."
	}
	return m.viewport.View()
}
