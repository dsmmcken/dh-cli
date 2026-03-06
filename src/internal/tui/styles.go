package tui

import (
	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/compat"
)

var (
	ColorPrimary = compat.AdaptiveColor{Light: lipgloss.Color("#2F71F2"), Dark: lipgloss.Color("#4A90FF")}
	ColorSuccess = compat.AdaptiveColor{Light: lipgloss.Color("#04B575"), Dark: lipgloss.Color("#04B575")}
	ColorWarning = compat.AdaptiveColor{Light: lipgloss.Color("#FFA500"), Dark: lipgloss.Color("#FFA500")}
	ColorError   = compat.AdaptiveColor{Light: lipgloss.Color("#FF4672"), Dark: lipgloss.Color("#FF4672")}
	ColorDim     = compat.AdaptiveColor{Light: lipgloss.Color("#999999"), Dark: lipgloss.Color("#666666")}

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
