package screens

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/progress"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/dsmmcken/dh-cli/src/internal/vm"
)

type vmPrepareStepMsg struct {
	step    int
	message string
}

type vmPrepareDoneMsg struct {
	err error
}

// VMPrepareScreen shows step-by-step progress for VM snapshot preparation.
type VMPrepareScreen struct {
	dhHome   string
	version  string
	progress progress.Model
	step     int
	status   string
	done     bool
	err      error
	width    int
	height   int
}

var vmPrepareSteps = []string{
	"Ensuring Firecracker binary",
	"Ensuring kernel",
	"Building rootfs",
	"Creating snapshot",
}

// NewVMPrepareScreen creates a new VM prepare progress screen.
func NewVMPrepareScreen(dhHome, version string) VMPrepareScreen {
	p := progress.New()
	return VMPrepareScreen{
		dhHome:   dhHome,
		version:  version,
		progress: p,
		status:   "Starting preparation...",
	}
}

func (m VMPrepareScreen) Init() tea.Cmd {
	return m.doPrepare()
}

func (m VMPrepareScreen) doPrepare() tea.Cmd {
	dhHome := m.dhHome
	version := m.version
	return func() tea.Msg {
		paths := vm.NewVMPaths(dhHome)
		var logBuf bytes.Buffer

		// Step 1: Ensure Firecracker
		if err := vm.EnsureFirecracker(paths, &logBuf); err != nil {
			return vmPrepareDoneMsg{err: fmt.Errorf("firecracker: %w", err)}
		}

		// Step 2: Ensure kernel
		if err := vm.EnsureKernel(paths, &logBuf); err != nil {
			return vmPrepareDoneMsg{err: fmt.Errorf("kernel: %w", err)}
		}

		// Step 3: Build rootfs
		if err := vm.EnsureRootfs(paths, version, &logBuf); err != nil {
			return vmPrepareDoneMsg{err: fmt.Errorf("rootfs: %w", err)}
		}

		// Step 4: Create snapshot
		cfg := &vm.VMConfig{
			DHHome:  dhHome,
			Version: version,
		}
		if err := vm.BootAndSnapshot(context.Background(), cfg, paths, &logBuf); err != nil {
			return vmPrepareDoneMsg{err: fmt.Errorf("snapshot: %w", err)}
		}

		return vmPrepareDoneMsg{err: nil}
	}
}

func (m VMPrepareScreen) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		w := msg.Width - 10
		if w < 20 {
			w = 20
		}
		m.progress.SetWidth(w)
		return m, nil

	case vmPrepareStepMsg:
		m.step = msg.step
		m.status = msg.message
		return m, nil

	case vmPrepareDoneMsg:
		m.done = true
		m.err = msg.err
		return m, nil

	case progress.FrameMsg:
		var cmd tea.Cmd
		m.progress, cmd = m.progress.Update(msg)
		return m, cmd

	case tea.KeyPressMsg:
		if m.done {
			switch msg.String() {
			case "q", "ctrl+c", "enter", "esc":
				return m, popScreen()
			}
		}
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m VMPrepareScreen) View() tea.View {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("  Preparing VM snapshot — %s\n\n", m.version))

	if m.done {
		if m.err != nil {
			b.WriteString(fmt.Sprintf("  %s %s\n\n",
				lipgloss.NewStyle().Foreground(colorError).Render("Error:"),
				m.err))
			b.WriteString(lipgloss.NewStyle().Foreground(colorDim).Render("  Press any key to go back"))
		} else {
			b.WriteString(fmt.Sprintf("  %s Snapshot for %s is ready.\n\n",
				lipgloss.NewStyle().Foreground(colorSuccess).Render("✓"),
				m.version))
			b.WriteString(lipgloss.NewStyle().Foreground(colorDim).Render("  Press any key to go back"))
		}
		return tea.NewView(b.String())
	}

	// Show step progress
	pct := float64(m.step) / float64(len(vmPrepareSteps))
	b.WriteString("  " + m.progress.ViewAs(pct) + "\n\n")

	for i, stepName := range vmPrepareSteps {
		if i < m.step {
			b.WriteString(fmt.Sprintf("  %s %s\n",
				lipgloss.NewStyle().Foreground(colorSuccess).Render("✓"),
				stepName))
		} else if i == m.step {
			b.WriteString(fmt.Sprintf("  %s %s\n",
				lipgloss.NewStyle().Foreground(colorPrimary).Render("●"),
				lipgloss.NewStyle().Bold(true).Render(stepName)))
		} else {
			b.WriteString(fmt.Sprintf("  %s %s\n",
				lipgloss.NewStyle().Foreground(colorDim).Render("○"),
				lipgloss.NewStyle().Foreground(colorDim).Render(stepName)))
		}
	}

	return tea.NewView(b.String())
}
