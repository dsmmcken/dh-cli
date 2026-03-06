package screens

import (
	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/compat"
)

var (
	colorPrimary = compat.AdaptiveColor{Light: lipgloss.Color("#2F71F2"), Dark: lipgloss.Color("#4A90FF")}
	colorDim     = compat.AdaptiveColor{Light: lipgloss.Color("#999999"), Dark: lipgloss.Color("#666666")}
	colorSuccess = compat.AdaptiveColor{Light: lipgloss.Color("#04B575"), Dark: lipgloss.Color("#04B575")}
	colorWarning = compat.AdaptiveColor{Light: lipgloss.Color("#FFA500"), Dark: lipgloss.Color("#FFA500")}
	colorError   = compat.AdaptiveColor{Light: lipgloss.Color("#FF4672"), Dark: lipgloss.Color("#FF4672")}
)
