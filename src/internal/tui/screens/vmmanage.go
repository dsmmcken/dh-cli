package screens

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/dsmmcken/dh-cli/src/internal/config"
	"github.com/dsmmcken/dh-cli/src/internal/vm"
)

const vmPollInterval = 5 * time.Second

// VMStatusLoadedMsg carries the results of an async VM status check.
// Exported for testing.
type VMStatusLoadedMsg struct {
	Prereqs   []*vm.PrereqError
	Snapshots []SnapshotEntry
	PoolInfo  *vm.PoolStatus
	PoolErr   error
}

// VMPollTickMsg is the periodic poll tick message. Exported for testing.
type VMPollTickMsg struct{}

// SnapshotEntry represents a VM snapshot in the list. Exported for testing.
type SnapshotEntry struct {
	Version string
	Ready   bool // true if CheckSnapshot passes
}

type vmKeyMap struct {
	Up      key.Binding
	Down    key.Binding
	Prepare key.Binding
	Delete  key.Binding
	Start   key.Binding
	Stop    key.Binding
	ScaleUp key.Binding
	ScaleDn key.Binding
	Help    key.Binding
	Back    key.Binding
	Quit    key.Binding
}

func (k vmKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Prepare, k.Delete, k.Start, k.Stop, k.Help, k.Back}
}

func (k vmKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down},
		{k.Prepare, k.Delete},
		{k.Start, k.Stop, k.ScaleUp, k.ScaleDn},
		{k.Help, k.Back, k.Quit},
	}
}

// VMManageScreen shows VM prerequisites, snapshots, and pool status.
type VMManageScreen struct {
	keys      vmKeyMap
	help      help.Model
	dhHome    string
	prereqs   []*vm.PrereqError
	snapshots []SnapshotEntry
	poolInfo  *vm.PoolStatus
	poolErr   error
	cursor    int
	loading   bool
	status    string // transient status message
	width     int
	height    int
}

// NewVMManageScreen creates a new VM management screen.
func NewVMManageScreen(dhHome string) VMManageScreen {
	return VMManageScreen{
		keys: vmKeyMap{
			Up:      key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
			Down:    key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
			Prepare: key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "prepare")),
			Delete:  key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "delete")),
			Start:   key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "start pool")),
			Stop:    key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "stop pool")),
			ScaleUp: key.NewBinding(key.WithKeys("+"), key.WithHelp("+", "scale +1")),
			ScaleDn: key.NewBinding(key.WithKeys("-"), key.WithHelp("-", "scale -1")),
			Help:    key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "more")),
			Back:    key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
			Quit:    key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
		},
		help:    help.New(),
		dhHome:  dhHome,
		loading: true,
	}
}

func (m VMManageScreen) Init() tea.Cmd {
	return tea.Batch(loadVMStatus(m.dhHome), pollVMTick())
}

// Snapshots returns the current snapshot list (for testing).
func (m VMManageScreen) Snapshots() []SnapshotEntry {
	return m.snapshots
}

// Cursor returns the current cursor position (for testing).
func (m VMManageScreen) Cursor() int {
	return m.cursor
}

// Status returns the current status message (for testing).
func (m VMManageScreen) Status() string {
	return m.status
}

// Loading returns whether the screen is still loading (for testing).
func (m VMManageScreen) Loading() bool {
	return m.loading
}

func loadVMStatus(dhHome string) tea.Cmd {
	return func() tea.Msg {
		paths := vm.NewVMPaths(dhHome)
		prereqs := vm.CheckPrerequisites(paths)

		// Scan snapshot directory
		var snapshots []SnapshotEntry
		entries, err := os.ReadDir(paths.SnapshotDir)
		if err == nil {
			for _, e := range entries {
				if !e.IsDir() {
					continue
				}
				ver := e.Name()
				ready := vm.CheckSnapshot(paths, ver) == nil
				snapshots = append(snapshots, SnapshotEntry{Version: ver, Ready: ready})
			}
		}

		// Check pool status
		var poolInfo *vm.PoolStatus
		var poolErr error
		if vm.PoolProbe() {
			resp, err := vm.PoolCommand(&vm.PoolRequest{Type: "status"})
			if err != nil {
				poolErr = err
			} else if resp.Status != nil {
				poolInfo = resp.Status
			}
		}

		return VMStatusLoadedMsg{
			Prereqs:   prereqs,
			Snapshots: snapshots,
			PoolInfo:  poolInfo,
			PoolErr:   poolErr,
		}
	}
}

func pollVMTick() tea.Cmd {
	return tea.Tick(vmPollInterval, func(_ time.Time) tea.Msg {
		return VMPollTickMsg{}
	})
}

func (m VMManageScreen) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.help.SetWidth(msg.Width)
		return m, nil

	case VMStatusLoadedMsg:
		m.loading = false
		m.prereqs = msg.Prereqs
		m.snapshots = msg.Snapshots
		m.poolInfo = msg.PoolInfo
		m.poolErr = msg.PoolErr
		if m.cursor >= len(m.snapshots) {
			m.cursor = max(0, len(m.snapshots)-1)
		}
		return m, nil

	case VMPollTickMsg:
		return m, tea.Batch(loadVMStatus(m.dhHome), pollVMTick())

	case tea.KeyPressMsg:
		if m.loading {
			if key.Matches(msg, m.keys.Quit) {
				return m, tea.Quit
			}
			return m, nil
		}

		// Check if platform supports VM
		supported := len(m.prereqs) == 0 || m.prereqs[0].Check != "platform"

		switch {
		case key.Matches(msg, m.keys.Up):
			if m.cursor > 0 {
				m.cursor--
			}
		case key.Matches(msg, m.keys.Down):
			if m.cursor < len(m.snapshots)-1 {
				m.cursor++
			}
		case key.Matches(msg, m.keys.Prepare):
			if !supported {
				m.status = "VM mode requires Linux with KVM support"
				return m, nil
			}
			version, err := config.ResolveVersion("", os.Getenv("DH_VERSION"))
			if err != nil {
				m.status = fmt.Sprintf("Error: %s", err)
				return m, nil
			}
			return m, pushScreen(NewVMPrepareScreen(m.dhHome, version))
		case key.Matches(msg, m.keys.Delete):
			if len(m.snapshots) > 0 {
				snap := m.snapshots[m.cursor]
				paths := vm.NewVMPaths(m.dhHome)
				snapDir := paths.SnapshotDirForVersion(snap.Version)
				rootfs := paths.RootfsForVersion(snap.Version)
				os.RemoveAll(snapDir)
				os.Remove(rootfs)
				m.status = fmt.Sprintf("Deleted snapshot %s", snap.Version)
				return m, loadVMStatus(m.dhHome)
			}
		case key.Matches(msg, m.keys.Start):
			if !supported {
				m.status = "VM mode requires Linux with KVM support"
				return m, nil
			}
			return m, m.startPool()
		case key.Matches(msg, m.keys.Stop):
			if vm.PoolProbe() {
				_, err := vm.PoolCommand(&vm.PoolRequest{Type: "stop"})
				if err != nil {
					m.status = fmt.Sprintf("Error stopping pool: %s", err)
				} else {
					m.status = "Pool daemon stopped"
				}
				return m, loadVMStatus(m.dhHome)
			}
		case key.Matches(msg, m.keys.ScaleUp):
			if m.poolInfo != nil {
				_, err := vm.PoolCommand(&vm.PoolRequest{
					Type:       "scale",
					TargetSize: m.poolInfo.TargetSize + 1,
				})
				if err != nil {
					m.status = fmt.Sprintf("Error scaling pool: %s", err)
				} else {
					m.status = fmt.Sprintf("Pool scaled to %d", m.poolInfo.TargetSize+1)
				}
				return m, loadVMStatus(m.dhHome)
			}
		case key.Matches(msg, m.keys.ScaleDn):
			if m.poolInfo != nil {
				target := m.poolInfo.TargetSize - 1
				if target < 1 {
					target = 1
				}
				_, err := vm.PoolCommand(&vm.PoolRequest{
					Type:       "scale",
					TargetSize: target,
				})
				if err != nil {
					m.status = fmt.Sprintf("Error scaling pool: %s", err)
				} else {
					m.status = fmt.Sprintf("Pool scaled to %d", target)
				}
				return m, loadVMStatus(m.dhHome)
			}
		case key.Matches(msg, m.keys.Help):
			m.help.ShowAll = !m.help.ShowAll
		case key.Matches(msg, m.keys.Back):
			return m, popScreen()
		case key.Matches(msg, m.keys.Quit):
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m VMManageScreen) startPool() tea.Cmd {
	dhHome := m.dhHome
	return func() tea.Msg {
		exePath, err := os.Executable()
		if err != nil {
			return VMStatusLoadedMsg{} // will refresh anyway
		}
		cmd := exec.Command(exePath, "vm", "pool", "start", "--background")
		if dhHome != "" {
			cmd.Env = append(os.Environ(), "DH_HOME="+dhHome)
		}
		cmd.Start()
		// Give the daemon a moment to start
		time.Sleep(500 * time.Millisecond)
		// Return a fresh status load
		return loadVMStatus(dhHome)()
	}
}

func (m VMManageScreen) View() tea.View {
	var b strings.Builder

	b.WriteString("  VM Management\n\n")

	if m.loading {
		b.WriteString("  Loading...\n")
		return tea.NewView(b.String())
	}

	// Prerequisites section
	b.WriteString(lipgloss.NewStyle().Foreground(colorPrimary).Bold(true).Render("  Prerequisites"))
	b.WriteString("\n")
	if len(m.prereqs) == 0 {
		b.WriteString("  " + lipgloss.NewStyle().Foreground(colorSuccess).Render("✓") + " All prerequisites met\n")
	} else {
		for _, p := range m.prereqs {
			b.WriteString("  " + lipgloss.NewStyle().Foreground(colorError).Render("✗") + " ")
			b.WriteString(p.Check + ": " + p.Message)
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")

	// Snapshots section
	b.WriteString(lipgloss.NewStyle().Foreground(colorPrimary).Bold(true).Render("  Snapshots"))
	b.WriteString("\n")
	if len(m.snapshots) == 0 {
		b.WriteString(lipgloss.NewStyle().Foreground(colorDim).Render("  No snapshots found."))
		b.WriteString("\n")
	} else {
		for i, snap := range m.snapshots {
			statusStr := lipgloss.NewStyle().Foreground(colorSuccess).Render("ready")
			if !snap.Ready {
				statusStr = lipgloss.NewStyle().Foreground(colorWarning).Render("incomplete")
			}
			label := fmt.Sprintf("%s: %s", snap.Version, statusStr)

			if i == m.cursor {
				b.WriteString(lipgloss.NewStyle().Foreground(colorPrimary).Bold(true).Render("  > " + label))
			} else {
				b.WriteString("    " + label)
			}
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")

	// Pool section
	b.WriteString(lipgloss.NewStyle().Foreground(colorPrimary).Bold(true).Render("  Pool daemon"))
	b.WriteString("\n")
	if m.poolInfo != nil {
		b.WriteString(fmt.Sprintf("  "+lipgloss.NewStyle().Foreground(colorSuccess).Render("running")+" (pid=%d)\n", m.poolInfo.PID))
		b.WriteString(fmt.Sprintf("    Version: %s  Ready: %d/%d  Idle: %ds\n",
			m.poolInfo.Version, m.poolInfo.Ready, m.poolInfo.TargetSize, m.poolInfo.IdleSeconds))
	} else if m.poolErr != nil {
		b.WriteString("  " + lipgloss.NewStyle().Foreground(colorError).Render(fmt.Sprintf("error: %s", m.poolErr)) + "\n")
	} else {
		b.WriteString("  " + lipgloss.NewStyle().Foreground(colorDim).Render("not running") + "\n")
	}

	// Status message
	if m.status != "" {
		b.WriteString("\n")
		b.WriteString("  " + lipgloss.NewStyle().Foreground(colorSuccess).Render(m.status))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(m.help.View(m.keys))

	return tea.NewView(b.String())
}
