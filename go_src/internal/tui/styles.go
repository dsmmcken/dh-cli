package tui

import "github.com/charmbracelet/lipgloss"

var (
	ColorPrimary = lipgloss.AdaptiveColor{Light: "#2F71F2", Dark: "#4A90FF"}
	ColorSuccess = lipgloss.AdaptiveColor{Light: "#04B575", Dark: "#04B575"}
	ColorWarning = lipgloss.AdaptiveColor{Light: "#FFA500", Dark: "#FFA500"}
	ColorError   = lipgloss.AdaptiveColor{Light: "#FF4672", Dark: "#FF4672"}
	ColorDim     = lipgloss.AdaptiveColor{Light: "#999999", Dark: "#666666"}

	StyleLogo = lipgloss.NewStyle().Foreground(ColorPrimary)

	StyleSelected = lipgloss.NewStyle().
			Foreground(ColorPrimary).
			Bold(true)

	StyleDim = lipgloss.NewStyle().Foreground(ColorDim)

	StyleTitle = lipgloss.NewStyle().
			Foreground(ColorPrimary).
			Bold(true).
			MarginBottom(1)

	StyleSuccess = lipgloss.NewStyle().Foreground(ColorSuccess)
	StyleWarning = lipgloss.NewStyle().Foreground(ColorWarning)
	StyleError   = lipgloss.NewStyle().Foreground(ColorError)

	StyleHelpBar = lipgloss.NewStyle().Foreground(ColorDim)
)
