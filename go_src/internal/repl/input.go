package repl

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dsmmcken/dh-cli/go_src/internal/tui"
)

// SubmitMsg is sent when the user presses Enter to submit code.
type SubmitMsg struct {
	Code string
}

// TabSearchMsg is emitted when the user selects a tab via Ctrl+T.
type TabSearchMsg struct {
	Selected string
}

// InputMode tracks the current input interaction mode.
type InputMode int

const (
	InputNormal        InputMode = iota
	InputHistorySearch           // Ctrl+R reverse-i-search
	InputTabSearch               // Ctrl+T tab search
)

// InputModel wraps a textarea with history, search, and submit behavior.
type InputModel struct {
	textarea      textarea.Model
	maxHeight     int
	totalWidth    int
	history       *History
	mode          InputMode
	searchQuery   string
	searchMatches []string
	searchIdx     int
	tabNames      []string
	executing     bool
}

// NewInput creates a new input component with history support.
func NewInput(history *History) InputModel {
	ta := textarea.New()
	ta.Placeholder = "Enter Python code..."
	ta.Prompt = "> "
	ta.SetHeight(1)
	ta.SetWidth(80)
	ta.ShowLineNumbers = false
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()
	ta.CharLimit = 0
	ta.Focus()

	return InputModel{
		textarea:  ta,
		maxHeight: 6,
		history:   history,
		mode:      InputNormal,
	}
}

// SetWidth updates the textarea width, accounting for the box border.
func (m *InputModel) SetWidth(w int) {
	m.totalWidth = w
	m.textarea.SetWidth(w - 2) // -2 for left+right box border
}

// Reset clears the textarea after submission.
func (m *InputModel) Reset() {
	m.textarea.Reset()
	m.textarea.SetHeight(1)
	if m.history != nil {
		m.history.ResetNavigation()
	}
}

// Value returns the current textarea content.
func (m InputModel) Value() string {
	return m.textarea.Value()
}

// SetTabNames updates the available tab names for Ctrl+T search.
func (m *InputModel) SetTabNames(names []string) {
	m.tabNames = names
}

// SetExecuting changes the visual state to indicate code is running.
func (m *InputModel) SetExecuting(v bool) {
	m.executing = v
}

// Height returns the rendered height including the border.
func (m InputModel) Height() int {
	lines := strings.Count(m.textarea.Value(), "\n") + 1
	if lines > m.maxHeight {
		lines = m.maxHeight
	}
	if lines < 1 {
		lines = 1
	}
	return lines + 2 // +2 for top and bottom border
}

// Update handles key input with history navigation and search modes.
func (m InputModel) Update(msg tea.Msg) (InputModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.mode == InputHistorySearch {
			return m.updateHistorySearch(msg)
		}
		if m.mode == InputTabSearch {
			return m.updateTabSearch(msg)
		}

		switch msg.String() {
		case "enter":
			code := strings.TrimSpace(m.textarea.Value())
			if code == "" {
				return m, nil
			}
			return m, func() tea.Msg { return SubmitMsg{Code: m.textarea.Value()} }
		case "shift+enter":
			// Forward as enter to the textarea so it uses splitLine internally
			var cmd tea.Cmd
			m.textarea, cmd = m.textarea.Update(tea.KeyMsg{Type: tea.KeyEnter})
			m.adjustHeight()
			return m, cmd
		case "tab", "shift+tab":
			return m, nil
		case "up":
			if m.canNavigateHistory() {
				if entry, ok := m.history.Up(m.textarea.Value()); ok {
					m.textarea.SetValue(entry)
					m.textarea.CursorEnd()
					m.adjustHeight()
				}
				return m, nil
			}
		case "down":
			if m.canNavigateHistory() {
				if entry, ok := m.history.Down(m.textarea.Value()); ok {
					m.textarea.SetValue(entry)
					m.textarea.CursorEnd()
					m.adjustHeight()
				}
				return m, nil
			}
		case "ctrl+r":
			m.mode = InputHistorySearch
			m.searchQuery = ""
			m.searchMatches = nil
			m.searchIdx = 0
			return m, nil
		case "ctrl+t":
			m.mode = InputTabSearch
			m.searchQuery = ""
			m.searchMatches = m.tabNames
			m.searchIdx = 0
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.textarea, cmd = m.textarea.Update(msg)
	m.adjustHeight()
	return m, cmd
}

func (m InputModel) canNavigateHistory() bool {
	return m.history != nil && strings.Count(m.textarea.Value(), "\n") == 0
}

func (m InputModel) updateHistorySearch(msg tea.KeyMsg) (InputModel, tea.Cmd) {
	switch msg.String() {
	case "enter":
		if len(m.searchMatches) > 0 && m.searchIdx < len(m.searchMatches) {
			selected := m.searchMatches[m.searchIdx]
			m.textarea.SetValue(selected)
			m.textarea.CursorEnd()
			m.adjustHeight()
		}
		m.mode = InputNormal
		m.searchQuery = ""
		m.searchMatches = nil
		return m, nil
	case "escape", "ctrl+c", "ctrl+r":
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
			m.searchMatches = m.history.Search(m.searchQuery)
			m.searchIdx = 0
		}
		return m, nil
	default:
		key := msg.String()
		if len(key) == 1 && key[0] >= 32 && key[0] < 127 {
			m.searchQuery += key
			m.searchMatches = m.history.Search(m.searchQuery)
			m.searchIdx = 0
		}
		return m, nil
	}
}

func (m InputModel) updateTabSearch(msg tea.KeyMsg) (InputModel, tea.Cmd) {
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

func (m *InputModel) adjustHeight() {
	lines := strings.Count(m.textarea.Value(), "\n") + 1
	height := lines
	if height > m.maxHeight {
		height = m.maxHeight
	}
	if height < 1 {
		height = 1
	}
	m.textarea.SetHeight(height)
}

// View renders the input area or search overlay.
func (m InputModel) View() string {
	borderColor := tui.ColorPrimary
	if m.executing {
		borderColor = tui.ColorDim
	}
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor)

	var content string
	switch m.mode {
	case InputHistorySearch:
		content = m.renderSearchOverlay("reverse-i-search", m.searchQuery, m.searchMatches, m.searchIdx)
	case InputTabSearch:
		content = m.renderSearchOverlay("tab-search", m.searchQuery, m.searchMatches, m.searchIdx)
	default:
		content = m.textarea.View()
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
