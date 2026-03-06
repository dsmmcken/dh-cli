package repl

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/dsmmcken/dh-cli/src/internal/tui"
	"github.com/dsmmcken/dh-cli/src/internal/repl/editline"
	"github.com/dsmmcken/dh-cli/src/internal/repl/rhistory"
)

// TabSearchMsg is emitted when the user selects a tab via Ctrl+T.
type TabSearchMsg struct {
	Selected string
}

// InputMode tracks the current input interaction mode.
type InputMode int

const (
	InputNormal    InputMode = iota
	InputTabSearch           // Ctrl+T tab search
)

// InputModel wraps an editline.Model with tab search and border rendering.
type InputModel struct {
	editor      *editline.Model
	maxHeight   int
	minHeight   int
	totalWidth  int
	historyPath string

	mode          InputMode
	tabNames      []string
	searchQuery   string
	searchMatches []string
	searchIdx     int
	executing     bool
}

// NewInput creates a new input component backed by bubbline's editline.
func NewInput(historyPath string) InputModel {
	ed := editline.New(80, 10)
	ed.Prompt = "> "
	ed.NextPrompt = "  "
	ed.Placeholder = "Enter Python code..."
	ed.MaxHistorySize = 500
	ed.DedupHistory = true
	ed.MaxHeight = 0 // no limit (we cap in Height())
	ed.CharLimit = 0
	ed.ShowLineNumbers = false

	// Enter always submits; use shift+enter for newlines.
	ed.CheckInputComplete = func(_ [][]rune, _, _ int) bool {
		return true
	}

	// Bind shift+enter (and ctrl+o fallback) for inserting newlines.
	ed.KeyMap.AlwaysNewline = key.NewBinding(
		key.WithKeys("shift+enter", "ctrl+o"),
		key.WithHelp("shift+ret", "newline"),
	)
	// Disable the help toggle — we show shortcuts in the sidebar instead.
	ed.KeyMap.MoreHelp.SetEnabled(false)

	// Load existing history.
	loadHistory(ed, historyPath)

	return InputModel{
		editor:      ed,
		maxHeight:   6,
		minHeight:   3,
		historyPath: historyPath,
		mode:        InputNormal,
	}
}

// loadHistory loads history from file, handling both the old JSON-lines
// format and the bubbline libedit format.
func loadHistory(ed *editline.Model, path string) {
	// Try bubbline's native format first.
	h, err := rhistory.LoadHistory(path)
	if err == nil && len(h) > 0 {
		ed.SetHistory(h)
		return
	}

	// Fall back to our old JSON-lines format.
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var entries []string
	for _, line := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var entry string
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			entry = line
		}
		entries = append(entries, entry)
	}
	if len(entries) > 0 {
		ed.SetHistory(entries)
	}
}

// SaveHistory persists the current history to disk.
func (m *InputModel) SaveHistory() {
	if m.historyPath == "" {
		return
	}
	dir := filepath.Dir(m.historyPath)
	os.MkdirAll(dir, 0o755)
	rhistory.SaveHistory(m.editor.GetHistory(), m.historyPath)
}

// SetWidth updates the editor width, accounting for the box border.
func (m *InputModel) SetWidth(w int) {
	m.totalWidth = w
	m.editor.SetSize(w-2, 0) // -2 for left+right border; height managed separately
}

// Reset clears the editor after submission.
func (m *InputModel) Reset() {
	m.editor.Reset()
}

// Value returns the current editor content.
func (m InputModel) Value() string {
	return m.editor.Value()
}

// SetTabNames updates the available tab names for Ctrl+T search.
func (m *InputModel) SetTabNames(names []string) {
	m.tabNames = names
}

// SetExecuting changes the visual state to indicate code is running.
func (m *InputModel) SetExecuting(v bool) {
	m.executing = v
}

// IsTabSearching returns true if the input is in tab search mode.
func (m InputModel) IsTabSearching() bool {
	return m.mode == InputTabSearch
}

// Height returns the rendered height including the border.
func (m InputModel) Height() int {
	lines := strings.Count(m.editor.Value(), "\n") + 1
	if lines > m.maxHeight {
		lines = m.maxHeight
	}
	if lines < m.minHeight {
		lines = m.minHeight
	}
	return lines + 2 // +2 for top and bottom border
}

// Update handles key input, intercepting tab search before delegating to editline.
func (m InputModel) Update(msg tea.Msg) (InputModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		if m.mode == InputTabSearch {
			return m.updateTabSearch(msg)
		}

		switch msg.String() {
		case "ctrl+t":
			m.mode = InputTabSearch
			m.searchQuery = ""
			m.searchMatches = m.tabNames
			m.searchIdx = 0
			return m, nil
		case "tab", "shift+tab":
			// Consume so tab bar handles these instead.
			return m, nil
		}
	}

	// Delegate everything else to editline.
	_, cmd := m.editor.Update(msg)
	return m, cmd
}

func (m InputModel) updateTabSearch(msg tea.KeyPressMsg) (InputModel, tea.Cmd) {
	switch msg.String() {
	case "enter":
		if len(m.searchMatches) > 0 && m.searchIdx < len(m.searchMatches) {
			selected := m.searchMatches[m.searchIdx]
			m.mode = InputNormal
			m.searchQuery = ""
			m.searchMatches = nil
			return m, func() tea.Msg { return TabSearchMsg{Selected: selected} }
		}
		m.mode = InputNormal
		m.searchQuery = ""
		m.searchMatches = nil
		return m, nil
	case "escape", "ctrl+c", "ctrl+t":
		m.mode = InputNormal
		m.searchQuery = ""
		m.searchMatches = nil
		return m, nil
	case "up", "ctrl+p":
		if m.searchIdx < len(m.searchMatches)-1 {
			m.searchIdx++
		}
		return m, nil
	case "down", "ctrl+n":
		if m.searchIdx > 0 {
			m.searchIdx--
		}
		return m, nil
	case "backspace":
		if len(m.searchQuery) > 0 {
			m.searchQuery = m.searchQuery[:len(m.searchQuery)-1]
			m.filterTabNames()
			m.searchIdx = 0
		}
		return m, nil
	default:
		key := msg.String()
		if len(key) == 1 && key[0] >= 32 && key[0] < 127 {
			m.searchQuery += key
			m.filterTabNames()
			m.searchIdx = 0
		}
		return m, nil
	}
}

func (m *InputModel) filterTabNames() {
	if m.searchQuery == "" {
		m.searchMatches = m.tabNames
		return
	}
	query := strings.ToLower(m.searchQuery)
	var matches []string
	for _, name := range m.tabNames {
		if strings.Contains(strings.ToLower(name), query) {
			matches = append(matches, name)
		}
	}
	m.searchMatches = matches
}

// View renders the input area with a border box.
func (m InputModel) View() string {
	borderColor := tui.ColorPrimary
	if m.executing {
		borderColor = tui.ColorDim
	}
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor)
	if m.totalWidth > 2 {
		boxStyle = boxStyle.Width(m.totalWidth - 2)
	}

	var content string
	if m.mode == InputTabSearch {
		content = m.renderSearchOverlay("tab-search", m.searchQuery, m.searchMatches, m.searchIdx)
	} else {
		content = m.editor.View()
		// Strip bubbline's built-in help/shortcut line from the bottom.
		// We show shortcuts in the sidebar instead. Keep the search
		// prompt (Ctrl+R) which replaces the help line during search.
		// The help line uses "•" bullet separators; the search prompt does not.
		if idx := strings.LastIndex(content, "\n"); idx >= 0 {
			lastLine := content[idx+1:]
			if strings.Contains(lastLine, "•") {
				content = content[:idx]
			}
		}
	}
	return boxStyle.Render(content)
}

func (m InputModel) renderSearchOverlay(title, query string, matches []string, selectedIdx int) string {
	promptStyle := lipgloss.NewStyle().Foreground(tui.ColorPrimary).Bold(true)
	queryStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF"))
	matchStyle := lipgloss.NewStyle().Foreground(tui.ColorDim)
	selectedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Background(tui.ColorPrimary)

	prompt := promptStyle.Render(fmt.Sprintf("(%s)> ", title)) + queryStyle.Render(query)

	maxShow := 5
	if len(matches) < maxShow {
		maxShow = len(matches)
	}

	var lines []string
	lines = append(lines, prompt)

	if len(matches) == 0 && query != "" {
		lines = append(lines, matchStyle.Render("  (no matches)"))
	} else {
		for i := 0; i < maxShow; i++ {
			if i == selectedIdx {
				lines = append(lines, selectedStyle.Render("> "+matches[i]))
			} else {
				lines = append(lines, matchStyle.Render("  "+matches[i]))
			}
		}
		if len(matches) > maxShow {
			lines = append(lines, matchStyle.Render(fmt.Sprintf("  ... +%d more", len(matches)-maxShow)))
		}
	}

	return strings.Join(lines, "\n")
}
