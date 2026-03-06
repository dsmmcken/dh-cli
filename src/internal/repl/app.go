package repl

import (
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/dsmmcken/dh-cli/src/internal/tui"
	"github.com/dsmmcken/dh-cli/src/internal/repl/editline"
)

// SessionStartedMsg is sent when the Python session is ready.
type SessionStartedMsg struct {
	Session *Session
	Err     error
}

// ExecuteResultMsg is sent when code execution completes.
type ExecuteResultMsg struct {
	Code     string
	Response *Response
	Err      error
}

// TableUpdateMsg is sent when a live table pushes an update.
type TableUpdateMsg struct {
	Name     string
	Response *Response
}

// REPLModel is the top-level bubbletea model for the REPL.
type REPLModel struct {
	input      InputModel
	tabbar     TabBarModel
	logview    LogViewModel
	tableviews map[string]*TableViewModel
	sidebar    SidebarModel

	session         *Session
	cfg             SessionConfig
	executing       bool
	err             error
	activeView      string
	subscribedTable string // name of the currently subscribed table, or ""

	width  int
	height int
}

// NewREPLModel creates a new REPL model with the given session config.
func NewREPLModel(cfg SessionConfig) REPLModel {
	historyPath := filepath.Join(cfg.DHHome, "repl_history")
	return REPLModel{
		input:      NewInput(historyPath),
		tabbar:     NewTabBar(),
		logview:    NewLogView(),
		tableviews: make(map[string]*TableViewModel),
		sidebar:    NewSidebar(),
		cfg:        cfg,
		activeView: "log",
	}
}

// Init starts cursor blinking and kicks off session creation.
func (m REPLModel) Init() tea.Cmd {
	m.logview.AppendEntry(LogEntry{
		Type: LogInfo,
		Text: "Starting Deephaven...",
	})
	return tea.Batch(
		m.input.editor.Focus(),
		m.startSession(),
	)
}

func (m REPLModel) startSession() tea.Cmd {
	cfg := m.cfg
	return func() tea.Msg {
		session, err := NewSession(cfg)
		return SessionStartedMsg{Session: session, Err: err}
	}
}

// Update handles all messages.
func (m REPLModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.layout()
		return m, nil

	case tea.KeyPressMsg:
		// Let tab search handle ctrl+c itself (to cancel search, not quit).
		if m.input.IsTabSearching() && msg.String() == "ctrl+c" {
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return m, cmd
		}

		// While executing, allow scrolling in the active content area.
		if m.executing {
			switch msg.String() {
			case "ctrl+c", "ctrl+d":
				if m.session != nil {
					m.session.Close()
				}
				return m, tea.Quit
			}
			if m.activeView == "log" {
				var cmd tea.Cmd
				m.logview, cmd = m.logview.Update(msg)
				return m, cmd
			} else if tv, ok := m.tableviews[m.activeView]; ok {
				updated, cmd := tv.Update(msg)
				*tv = updated
				return m, cmd
			}
			return m, nil
		}

	case editline.InputCompleteMsg:
		// Bubbline signals input is complete.
		editorErr := m.input.editor.Err
		if editorErr != nil {
			// Ctrl+D (io.EOF) or Ctrl+C on empty (ErrInterrupted) → quit.
			if editorErr == io.EOF || editorErr == editline.ErrInterrupted {
				if m.session != nil {
					m.session.Close()
				}
				return m, tea.Quit
			}
		}

		code := strings.TrimSpace(m.input.Value())
		if code == "" {
			m.input.Reset()
			return m, nil
		}

		if m.session == nil || m.executing {
			return m, nil
		}

		m.executing = true
		m.input.SetExecuting(true)
		m.input.editor.AddHistoryEntry(code)
		m.input.SaveHistory()
		m.input.Reset()

		m.logview.AppendEntry(LogEntry{Type: LogCommand, Text: code})

		return m, m.executeCode(code)

	case SessionStartedMsg:
		if msg.Err != nil {
			m.err = msg.Err
			m.logview.AppendEntry(LogEntry{
				Type: LogError,
				Text: fmt.Sprintf("Failed to start session: %v", msg.Err),
			})
			return m, nil
		}
		m.session = msg.Session
		ready := m.session.Ready()
		m.logview.AppendEntry(LogEntry{
			Type: LogInfo,
			Text: fmt.Sprintf("Connected to Deephaven %s (%s mode, port %d)",
				ready.Version, ready.Mode, ready.Port),
		})
		host := "localhost"
		if m.cfg.Host != "" {
			host = m.cfg.Host
		}
		m.sidebar.SetServerInfo(ServerInfoData{
			Host:       host,
			Port:       ready.Port,
			Version:    ready.Version,
			Mode:       ready.Mode,
			TableCount: 0,
		})
		return m, m.listenForPush()

	case ExecuteResultMsg:
		m.executing = false
		m.input.SetExecuting(false)
		if msg.Err != nil {
			m.logview.AppendEntry(LogEntry{
				Type: LogError,
				Text: fmt.Sprintf("Execution error: %v", msg.Err),
			})
			return m, nil
		}

		resp := msg.Response
		var entries []LogEntry

		if resp.Stdout != "" {
			entries = append(entries, LogEntry{Type: LogStdout, Text: resp.Stdout})
		}
		if resp.Stderr != "" {
			entries = append(entries, LogEntry{Type: LogStderr, Text: resp.Stderr})
		}
		if resp.HasError() {
			entries = append(entries, LogEntry{Type: LogError, Text: *resp.Error})
		}
		if resp.ResultRepr != nil && *resp.ResultRepr != "" && *resp.ResultRepr != "None" {
			entries = append(entries, LogEntry{Type: LogResult, Text: *resp.ResultRepr})
		}

		if len(entries) > 0 {
			m.logview.AppendEntries(entries)
		}

		// Update sidebar table count
		if m.sidebar.serverInfo != nil {
			m.sidebar.serverInfo.TableCount = len(m.tableviews)
			if len(resp.AllTables) > 0 {
				m.sidebar.serverInfo.TableCount = len(resp.AllTables)
			}
		}

		// Handle newly assigned tables
		if len(resp.AssignedTables) > 0 {
			var fetchCmds []tea.Cmd
			for _, tableName := range resp.AssignedTables {
				m.tabbar.AddTableTab(tableName, -1, false)
				name := tableName
				fetchCmds = append(fetchCmds, m.fetchTableData(name, 0))
			}

			lastTable := resp.AssignedTables[len(resp.AssignedTables)-1]
			switchCmd := m.switchToView(lastTable)
			if switchCmd != nil {
				fetchCmds = append(fetchCmds, switchCmd)
			}

			m.updateTabNames()
			return m, tea.Batch(fetchCmds...)
		}

		m.updateTabNames()
		return m, nil

	case TableDataMsg:
		if msg.Err != nil {
			m.logview.AppendEntry(LogEntry{
				Type: LogError,
				Text: fmt.Sprintf("Failed to fetch table %s: %v", msg.Name, msg.Err),
			})
			return m, nil
		}

		resp := msg.Response
		tv, exists := m.tableviews[msg.Name]
		if !exists {
			meta := TableMeta{
				Name:     msg.Name,
				RowCount: resp.TotalRows,
				Columns:  make([]ColumnMeta, len(resp.Columns)),
			}
			for i, colName := range resp.Columns {
				typ := ""
				if i < len(resp.Types) {
					typ = resp.Types[i]
				}
				meta.Columns[i] = ColumnMeta{Name: colName, Type: typ}
			}
			newTV := NewTableView(msg.Name, meta)
			m.tableviews[msg.Name] = &newTV
			tv = &newTV
		}

		tv.SetData(resp)
		if resp.IsRefreshing {
			tv.isRefreshing = true
		}
		m.tabbar.UpdateTableTab(msg.Name, resp.TotalRows, tv.isRefreshing)

		if m.width > 0 && m.height > 0 {
			contentHeight := m.contentHeight()
			tv.SetSize(m.mainWidth()-2, contentHeight)
		}

		if m.activeView == msg.Name {
			tv.Focus()
		} else {
			tv.Blur()
		}

		m.updateTabNames()

		// Auto-subscribe if this table is refreshing and currently active
		if m.activeView == msg.Name && tv.isRefreshing && m.subscribedTable != msg.Name && m.session != nil {
			m.subscribedTable = msg.Name
			tv.SetSubscribed(true)
			session := m.session
			name := msg.Name
			return m, func() tea.Msg {
				session.Subscribe(name, 0, 200)
				return nil
			}
		}

		return m, nil

	case TableUpdateMsg:
		if msg.Response == nil {
			return m, m.listenForPush()
		}
		resp := msg.Response
		tv, exists := m.tableviews[msg.Name]
		if exists {
			tv.SetData(resp)
			m.tabbar.UpdateTableTab(msg.Name, resp.TotalRows, true)
		}
		return m, m.listenForPush()

	case TabSelectedMsg:
		cmd := m.switchToView(msg.Tab.Name)
		return m, cmd

	case TabSearchMsg:
		cmd := m.switchToView(msg.Selected)
		return m, cmd
	}

	// Forward to sub-components
	if !m.executing {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		cmds = append(cmds, cmd)
		// Recalculate layout when input height changes (multi-line)
		if m.width > 0 && m.height > 0 {
			m.layout()
		}
	}

	{
		var cmd tea.Cmd
		m.tabbar, cmd = m.tabbar.Update(msg)
		cmds = append(cmds, cmd)
	}

	// Forward to the active content area
	if m.activeView == "log" {
		var cmd tea.Cmd
		m.logview, cmd = m.logview.Update(msg)
		cmds = append(cmds, cmd)
	} else if tv, ok := m.tableviews[m.activeView]; ok {
		updated, cmd := tv.Update(msg)
		*tv = updated
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m REPLModel) executeCode(code string) tea.Cmd {
	session := m.session
	return func() tea.Msg {
		resp, err := session.Execute(code)
		return ExecuteResultMsg{Code: code, Response: resp, Err: err}
	}
}

// listenForPush returns a tea.Cmd that waits for the next push message.
func (m REPLModel) listenForPush() tea.Cmd {
	session := m.session
	if session == nil {
		return nil
	}
	return func() tea.Msg {
		select {
		case resp, ok := <-session.PushChannel():
			if !ok {
				return nil
			}
			return TableUpdateMsg{Name: resp.Name, Response: resp}
		case <-session.Done():
			return nil
		}
	}
}

func (m *REPLModel) switchToView(name string) tea.Cmd {
	// Blur the old table view
	if old, ok := m.tableviews[m.activeView]; ok {
		old.Blur()
	}

	// Unsubscribe from old table if it was subscribed
	var cmds []tea.Cmd
	if m.subscribedTable != "" && m.subscribedTable != name {
		oldName := m.subscribedTable
		if oldTV, ok := m.tableviews[oldName]; ok {
			oldTV.SetSubscribed(false)
		}
		session := m.session
		if session != nil {
			cmds = append(cmds, func() tea.Msg {
				session.Unsubscribe(oldName)
				return nil
			})
		}
		m.subscribedTable = ""
	}

	m.activeView = name
	m.tabbar.SetActiveByName(name)

	if tv, ok := m.tableviews[name]; ok {
		tv.Focus()

		// Subscribe if the table is refreshing and not already subscribed
		if tv.isRefreshing && m.subscribedTable != name && m.session != nil {
			m.subscribedTable = name
			tv.SetSubscribed(true)
			session := m.session
			cmds = append(cmds, func() tea.Msg {
				session.Subscribe(name, 0, 200)
				return nil
			})
		}
	}

	if len(cmds) > 0 {
		return tea.Batch(cmds...)
	}
	return nil
}

func (m REPLModel) fetchTableData(name string, offset int) tea.Cmd {
	session := m.session
	return func() tea.Msg {
		resp, err := session.FetchTable(name, offset, 200)
		return TableDataMsg{Name: name, Response: resp, Err: err}
	}
}

func (m *REPLModel) updateTabNames() {
	var names []string
	for name := range m.tableviews {
		names = append(names, name)
	}
	sort.Strings(names)
	m.input.SetTabNames(names)
}

// mainWidth returns the width available for the main content area.
func (m REPLModel) mainWidth() int {
	if m.width >= 60 {
		return m.width - m.sidebar.Width()
	}
	return m.width
}

func (m REPLModel) contentHeight() int {
	inputRenderedHeight := m.input.Height()
	tabBarHeight := 3  // 1 line + 2 for tab box borders
	contentBorder := 2 // top + bottom border on content area

	h := m.height - inputRenderedHeight - tabBarHeight - contentBorder
	if h < 1 {
		h = 1
	}
	return h
}

func (m *REPLModel) layout() {
	mainWidth := m.mainWidth()
	contentHeight := m.contentHeight()
	contentInnerWidth := mainWidth - 2 // -2 for content area border

	m.input.SetWidth(mainWidth)
	m.tabbar.SetWidth(mainWidth)
	m.logview.SetSize(contentInnerWidth, contentHeight)
	m.sidebar.SetHeight(m.height)

	for _, tv := range m.tableviews {
		tv.SetSize(contentInnerWidth, contentHeight)
	}
}

// View renders the REPL layout with sidebar.
func (m REPLModel) View() tea.View {
	var content string
	if m.err != nil && m.session == nil {
		content = fmt.Sprintf("\n  %s\n\n  Press Ctrl+C to exit.\n",
			tui.StyleError.Render(fmt.Sprintf("Error: %v", m.err)))
	} else {
		var contentView string
		if m.activeView == "log" {
			contentView = m.logview.View()
		} else if tv, ok := m.tableviews[m.activeView]; ok {
			contentView = tv.View()
		} else {
			contentView = tui.StyleDim.Render("  Loading table...")
		}

		contentBoxStyle := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(tui.ColorDim).
			Width(m.mainWidth() - 2).
			Height(m.contentHeight())

		mainSections := []string{
			m.input.View(),
			m.tabbar.View(),
			contentBoxStyle.Render(contentView),
		}

		mainArea := lipgloss.JoinVertical(lipgloss.Left, mainSections...)

		if m.width >= 60 {
			content = lipgloss.JoinHorizontal(lipgloss.Top, mainArea, m.sidebar.View())
		} else {
			content = mainArea
		}
	}

	v := tea.NewView(content)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}
