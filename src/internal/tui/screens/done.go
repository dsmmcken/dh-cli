package screens

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type DoneScreen struct {
	version string
	width   int
	height  int
}

func NewDoneScreen(version string) DoneScreen {
	return DoneScreen{version: version}
}

func (m DoneScreen) Init() tea.Cmd {
	return nil
}

func (m DoneScreen) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("enter", "q", "ctrl+c"))):
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m DoneScreen) View() string {
	var b strings.Builder

	b.WriteString("  ✓ Setup Complete\n\n")
	b.WriteString(fmt.Sprintf("  Deephaven %s installed and set as default.\n\n", m.version))

	b.WriteString("  Quick start:\n")
	b.WriteString(lipgloss.NewStyle().Foreground(colorDim).Render("    dh versions       Manage versions") + "\n")
	b.WriteString(lipgloss.NewStyle().Foreground(colorDim).Render("    dh list           See running servers") + "\n")
	b.WriteString(lipgloss.NewStyle().Foreground(colorDim).Render("    dh doctor         Check environment") + "\n\n")

	b.WriteString(lipgloss.NewStyle().Foreground(colorPrimary).Bold(true).Render("  > Done"))
	b.WriteString("\n\n")
	b.WriteString(lipgloss.NewStyle().Foreground(colorDim).Render("  enter finish • q quit"))

	return b.String()
}
