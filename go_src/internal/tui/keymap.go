package tui

import "github.com/charmbracelet/bubbles/key"

type NavigationKeyMap struct {
	Up    key.Binding
	Down  key.Binding
	Enter key.Binding
	Help  key.Binding
	Quit  key.Binding
	Back  key.Binding
}

func DefaultNavKeyMap() NavigationKeyMap {
	return NavigationKeyMap{
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "down"),
		),
		Enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "select"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "more"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
		Back: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "back"),
		),
	}
}
