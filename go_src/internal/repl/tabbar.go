package repl

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dsmmcken/dh-cli/go_src/internal/tui"
)

// TabType identifies the type of tab.
type TabType string

const (
	TabLog   TabType = "log"
	TabTable TabType = "table"
)

// TabInfo describes a single tab in the tab bar.
type TabInfo struct {
	Name         string
	Type         TabType
	RowCount     int
	IsRefreshing bool
}

// TabSelectedMsg is emitted when the active tab changes.
type TabSelectedMsg struct {
	Tab TabInfo
}

// TabBarModel is a horizontal tab bar component.
type TabBarModel struct {
	tabs      []TabInfo
	activeIdx int
	width     int
}

// NewTabBar creates a tab bar with the permanent "log" tab.
func NewTabBar() TabBarModel {
	return TabBarModel{
		tabs: []TabInfo{
			{Name: "log", Type: TabLog},
		},
		activeIdx: 0,
	}
}

// SetWidth updates available width for rendering.
func (m *TabBarModel) SetWidth(w int) {
	m.width = w
}

// ActiveTab returns the currently selected tab.
func (m TabBarModel) ActiveTab() TabInfo {
	if m.activeIdx < len(m.tabs) {
		return m.tabs[m.activeIdx]
	}
	return m.tabs[0]
}

// AddTableTab inserts a table tab after the "log" tab at position 1 (most recent first).
// If a tab with the same name already exists, update it instead.
func (m *TabBarModel) AddTableTab(name string, rowCount int, isRefreshing bool) {
	for i, t := range m.tabs {
		if t.Name == name && t.Type == TabTable {
			m.tabs[i].RowCount = rowCount
			m.tabs[i].IsRefreshing = isRefreshing
			return
		}
	}

	newTab := TabInfo{
		Name:         name,
		Type:         TabTable,
		RowCount:     rowCount,
		IsRefreshing: isRefreshing,
	}
	// Shift activeIdx to account for insertion at position 1
	if m.activeIdx >= 1 {
		m.activeIdx++
	}
	m.tabs = append(m.tabs[:1], append([]TabInfo{newTab}, m.tabs[1:]...)...)
}

// UpdateTableTab updates metadata for an existing table tab.
func (m *TabBarModel) UpdateTableTab(name string, rowCount int, isRefreshing bool) {
	for i, t := range m.tabs {
		if t.Name == name && t.Type == TabTable {
			m.tabs[i].RowCount = rowCount
			m.tabs[i].IsRefreshing = isRefreshing
			return
		}
	}
}

// SetActiveByName switches the active tab to the one with the given name.
func (m *TabBarModel) SetActiveByName(name string) bool {
	for i, t := range m.tabs {
		if t.Name == name {
			m.activeIdx = i
			return true
		}
	}
	return false
}

// TabCount returns the number of tabs.
func (m TabBarModel) TabCount() int {
	return len(m.tabs)
}

// Update handles tab navigation keys.
func (m TabBarModel) Update(msg tea.Msg) (TabBarModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "tab":
			m.activeIdx = (m.activeIdx + 1) % len(m.tabs)
			return m, func() tea.Msg { return TabSelectedMsg{Tab: m.tabs[m.activeIdx]} }
		case "shift+tab":
			m.activeIdx = (m.activeIdx - 1 + len(m.tabs)) % len(m.tabs)
			return m, func() tea.Msg { return TabSelectedMsg{Tab: m.tabs[m.activeIdx]} }
		}
	}
	return m, nil
}

// View renders the tab bar as a horizontal row of styled labels with overflow truncation.
func (m TabBarModel) View() string {
	if m.width == 0 || len(m.tabs) == 0 {
		return ""
	}

	var tabs []string
	usedWidth := 0

	for i, t := range m.tabs {
		label := t.Name
		if t.Type == TabTable && t.RowCount >= 0 {
			label = fmt.Sprintf("%s (%d)", t.Name, t.RowCount)
		}

		renderedWidth := lipgloss.Width(label) + 2
		if t.Type == TabTable && t.IsRefreshing {
			renderedWidth += 5 // " LIVE" badge
		}

		// Check overflow: reserve space for "+N" if not last tab
		if usedWidth+renderedWidth > m.width && i > 0 {
			remaining := len(m.tabs) - i
			truncStyle := lipgloss.NewStyle().Foreground(tui.ColorDim).Padding(0, 1)
			tabs = append(tabs, truncStyle.Render(fmt.Sprintf("+%d", remaining)))
			break
		}

		usedWidth += renderedWidth

		var tabRendered string
		if i == m.activeIdx {
			style := lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#FFFFFF")).
				Background(tui.ColorPrimary).
				Padding(0, 1)
			tabRendered = style.Render(label)
		} else {
			style := lipgloss.NewStyle().
				Foreground(tui.ColorDim).
				Padding(0, 1)
			tabRendered = style.Render(label)
		}

		if t.Type == TabTable && t.IsRefreshing {
			badge := lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00")).Bold(true).Render(" LIVE")
			tabRendered = tabRendered + badge
		}
		tabs = append(tabs, tabRendered)
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, tabs...)
}
