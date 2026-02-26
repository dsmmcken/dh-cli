package repl

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dsmmcken/dh-cli/src/internal/tui"
)

// ServerInfoData holds display data for the sidebar.
type ServerInfoData struct {
	Host       string
	Port       int
	Version    string
	Mode       string
	TableCount int
}

// replKeyMap defines keybindings displayed via the help component.
type replKeyMap struct {
	Submit    key.Binding
	Newline   key.Binding
	History   key.Binding
	SearchHist key.Binding
	SearchTabs key.Binding
	NextTab   key.Binding
	PrevTab   key.Binding
	Quit      key.Binding
}

func (k replKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Submit, k.Newline, k.History, k.SearchHist, k.SearchTabs, k.NextTab, k.PrevTab, k.Quit}
}

func (k replKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{k.ShortHelp()}
}

var defaultREPLKeyMap = replKeyMap{
	Submit:     key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "submit")),
	Newline:    key.NewBinding(key.WithKeys("shift+enter"), key.WithHelp("shift+ret", "newline")),
	History:    key.NewBinding(key.WithKeys("up", "down"), key.WithHelp("↑/↓", "history")),
	SearchHist: key.NewBinding(key.WithKeys("ctrl+r"), key.WithHelp("ctrl+r", "search hist")),
	SearchTabs: key.NewBinding(key.WithKeys("ctrl+t"), key.WithHelp("ctrl+t", "search tabs")),
	NextTab:    key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next tab")),
	PrevTab:    key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "prev tab")),
	Quit:       key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "quit")),
}

// SidebarModel displays server info and keybinding help.
type SidebarModel struct {
	serverInfo *ServerInfoData
	help       help.Model
	keys       replKeyMap
	width      int
	height     int
}

// NewSidebar creates a new sidebar with a fixed width.
func NewSidebar() SidebarModel {
	h := help.New()
	h.ShowAll = true
	h.ShortSeparator = ""
	return SidebarModel{
		width: 28,
		help:  h,
		keys:  defaultREPLKeyMap,
	}
}

// SetServerInfo updates the server info display data.
func (m *SidebarModel) SetServerInfo(info ServerInfoData) {
	m.serverInfo = &info
}

// SetHeight updates the sidebar height.
func (m *SidebarModel) SetHeight(h int) {
	m.height = h
}

// Width returns the fixed sidebar width.
func (m SidebarModel) Width() int {
	return m.width
}

// Update is a no-op; the sidebar doesn't handle messages directly.
func (m SidebarModel) Update(msg tea.Msg) (SidebarModel, tea.Cmd) {
	return m, nil
}

// View renders the sidebar with server info and keybinding help.
func (m SidebarModel) View() string {
	var sections []string

	sections = append(sections, m.renderServerInfo())
	sections = append(sections, "")
	sections = append(sections, m.renderHelp())

	content := strings.Join(sections, "\n")

	style := lipgloss.NewStyle().
		Width(m.width - 2).
		Height(m.height).
		BorderStyle(lipgloss.NormalBorder()).
		BorderLeft(true).
		BorderForeground(tui.ColorDim).
		PaddingLeft(1).
		PaddingRight(1)

	return style.Render(content)
}

func (m SidebarModel) renderServerInfo() string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(tui.ColorPrimary)
	labelStyle := lipgloss.NewStyle().Foreground(tui.ColorDim)
	valueStyle := lipgloss.NewStyle()

	lines := []string{
		titleStyle.Render("Server"),
	}

	if m.serverInfo == nil {
		lines = append(lines, labelStyle.Render("Connecting..."))
	} else {
		info := m.serverInfo
		lines = append(lines,
			labelStyle.Render("Host: ")+valueStyle.Render(fmt.Sprintf("%s:%d", info.Host, info.Port)),
			labelStyle.Render("Ver:  ")+valueStyle.Render(info.Version),
			labelStyle.Render("Mode: ")+valueStyle.Render(info.Mode),
			labelStyle.Render("Tbls: ")+valueStyle.Render(fmt.Sprintf("%d", info.TableCount)),
		)
	}

	return strings.Join(lines, "\n")
}

func (m SidebarModel) renderHelp() string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(tui.ColorPrimary)
	m.help.Width = m.width - 4 // account for border + padding

	lines := []string{
		titleStyle.Render("Keys"),
		"",
		m.help.View(m.keys),
	}

	return strings.Join(lines, "\n")
}
