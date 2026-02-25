package repl

import "github.com/charmbracelet/lipgloss"

var (
	stylePrompt    = lipgloss.NewStyle().Foreground(lipgloss.Color("#4A90FF"))
	styleExecuting = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFA500")).Italic(true)
)
