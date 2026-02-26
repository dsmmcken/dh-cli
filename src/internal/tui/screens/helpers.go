package screens

import tea "github.com/charmbracelet/bubbletea"

// PushScreenMsg tells the app to push a new screen onto the stack.
type PushScreenMsg struct {
	Screen tea.Model
}

// PopScreenMsg tells the app to pop the current screen.
type PopScreenMsg struct{}

func pushScreen(s tea.Model) tea.Cmd {
	return func() tea.Msg {
		return PushScreenMsg{Screen: s}
	}
}

func popScreen() tea.Cmd {
	return func() tea.Msg {
		return PopScreenMsg{}
	}
}
