package screens

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dsmmcken/dh-cli/go_src/internal/config"
	"github.com/dsmmcken/dh-cli/go_src/internal/java"
)

type javaResultMsg struct {
	info *java.JavaInfo
	err  error
}

type javaCheckKeyMap struct {
	Up    key.Binding
	Down  key.Binding
	Enter key.Binding
	Quit  key.Binding
	Back  key.Binding
}

type JavaCheckScreen struct {
	keys     javaCheckKeyMap
	spinner  spinner.Model
	checking bool
	result   *java.JavaInfo
	err      error
	cursor   int
	options  []string
	wizard   bool
	dhgHome  string
	width    int
	height   int
}

func NewJavaCheckScreen(dhgHome string, wizard bool) JavaCheckScreen {
	if dhgHome == "" {
		dhgHome = config.DHGHome()
	}
	s := spinner.New()
	s.Spinner = spinner.Dot
	return JavaCheckScreen{
		keys: javaCheckKeyMap{
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
			Quit: key.NewBinding(
				key.WithKeys("q", "ctrl+c"),
				key.WithHelp("q", "quit"),
			),
			Back: key.NewBinding(
				key.WithKeys("esc"),
				key.WithHelp("esc", "back"),
			),
		},
		spinner:  s,
		checking: true,
		wizard:   wizard,
		dhgHome:  dhgHome,
	}
}

func (m JavaCheckScreen) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.detectJava())
}

func (m JavaCheckScreen) detectJava() tea.Cmd {
	dhgHome := m.dhgHome
	return func() tea.Msg {
		info, err := java.Detect(dhgHome)
		return javaResultMsg{info: info, err: err}
	}
}

func (m JavaCheckScreen) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case javaResultMsg:
		m.checking = false
		m.result = msg.info
		m.err = msg.err
		if msg.info != nil && msg.info.Found {
			if m.wizard {
				m.options = []string{"Next"}
			}
			// No buttons when accessed from main menu — just status display
		} else {
			m.options = []string{"Install Java", "Skip for now"}
		}
		return m, nil

	case spinner.TickMsg:
		if m.checking {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
		return m, nil

	case tea.KeyMsg:
		if m.checking {
			if key.Matches(msg, m.keys.Quit) {
				return m, tea.Quit
			}
			return m, nil
		}

		switch {
		case key.Matches(msg, m.keys.Up):
			if m.cursor > 0 {
				m.cursor--
			}
		case key.Matches(msg, m.keys.Down):
			if m.cursor < len(m.options)-1 {
				m.cursor++
			}
		case key.Matches(msg, m.keys.Enter):
			return m, m.handleSelect()
		case key.Matches(msg, m.keys.Quit):
			return m, tea.Quit
		case key.Matches(msg, m.keys.Back):
			if !m.wizard {
				return m, popScreen()
			}
		}
	}
	return m, nil
}

func (m JavaCheckScreen) handleSelect() tea.Cmd {
	if m.result != nil && m.result.Found {
		// "Next" selected
		if m.wizard {
			return pushScreen(NewVersionPickerScreen(m.dhgHome))
		}
		return popScreen()
	}

	switch m.cursor {
	case 0: // "Install Java"
		// For now just go to next step
		if m.wizard {
			return pushScreen(NewVersionPickerScreen(m.dhgHome))
		}
		return popScreen()
	case 1: // "Skip for now"
		if m.wizard {
			return pushScreen(NewVersionPickerScreen(m.dhgHome))
		}
		return popScreen()
	}
	return nil
}

func (m JavaCheckScreen) View() string {
	var b strings.Builder

	if m.wizard {
		b.WriteString("  Step 1 of 3 — Java\n\n")
	} else {
		b.WriteString("  Java Status\n\n")
	}

	if m.checking {
		b.WriteString(fmt.Sprintf("  Checking for Java 17+...  %s\n", m.spinner.View()))
		return b.String()
	}

	if m.err != nil {
		b.WriteString(fmt.Sprintf("  Error: %s\n", m.err))
		return b.String()
	}

	if m.result != nil && m.result.Found {
		b.WriteString(fmt.Sprintf("  ✓ Java %s found\n", m.result.Version))
		b.WriteString(fmt.Sprintf("    %s (%s)\n\n", m.result.Path, m.result.Source))
	} else {
		b.WriteString("  ✗ No compatible Java found\n\n")
		b.WriteString("  Deephaven requires Java 17+.\n")
		b.WriteString("  We can install Eclipse Temurin 21 to ~/.dhg/java/\n")
		b.WriteString("  (no sudo required).\n\n")
	}

	for i, opt := range m.options {
		if i == m.cursor {
			b.WriteString(lipgloss.NewStyle().Foreground(colorPrimary).Bold(true).Render("  > " + opt))
		} else {
			b.WriteString("    " + opt)
		}
		b.WriteString("\n")
	}

	b.WriteString("\n")
	var helpParts []string
	if len(m.options) > 0 {
		helpParts = append(helpParts, "enter continue")
	}
	if !m.wizard {
		helpParts = append(helpParts, "esc back")
	}
	helpParts = append(helpParts, "q quit")
	b.WriteString(lipgloss.NewStyle().Foreground(colorDim).Render("  " + strings.Join(helpParts, " • ")))

	return b.String()
}
