package repl

import (
	"fmt"
	"strconv"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dsmmcken/dh-cli/go_src/internal/tui"
)

// TableDataMsg is sent when table data has been fetched from the server.
type TableDataMsg struct {
	Name     string
	Response *Response
	Err      error
}

// TableViewModel displays paginated column data from a Deephaven table.
type TableViewModel struct {
	name         string
	table        table.Model
	totalRows    int
	isRefreshing bool
	isSubscribed bool
	columns      []string
	types        []string
	dataOffset   int
	dataLimit    int
	loading      bool
	width        int
	height       int
	ready        bool
}

// NewTableView creates a table view from table metadata.
func NewTableView(name string, meta TableMeta) TableViewModel {
	cols := calculateColumns(meta.Columns, 80)

	t := table.New(
		table.WithColumns(cols),
		table.WithRows([]table.Row{}),
		table.WithFocused(true),
		table.WithHeight(20),
	)

	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(tui.ColorDim).
		BorderBottom(true).
		Bold(true)
	s.Selected = s.Selected.
		Foreground(lipgloss.Color("#FFFFFF")).
		Background(tui.ColorPrimary).
		Bold(false)
	t.SetStyles(s)

	colNames := make([]string, len(meta.Columns))
	colTypes := make([]string, len(meta.Columns))
	for i, c := range meta.Columns {
		colNames[i] = c.Name
		colTypes[i] = c.Type
	}

	return TableViewModel{
		name:         name,
		table:        t,
		totalRows:    meta.RowCount,
		isRefreshing: meta.IsRefreshing,
		columns:      colNames,
		types:        colTypes,
		dataOffset:   0,
		dataLimit:    200,
		loading:      false,
	}
}

func calculateColumns(cols []ColumnMeta, availableWidth int) []table.Column {
	if len(cols) == 0 {
		return nil
	}

	paddingPerCol := 2
	totalPadding := paddingPerCol * len(cols)
	usableWidth := availableWidth - totalPadding
	if usableWidth < len(cols)*8 {
		usableWidth = len(cols) * 8
	}

	perCol := usableWidth / len(cols)
	if perCol < 8 {
		perCol = 8
	}
	if perCol > 40 {
		perCol = 40
	}

	result := make([]table.Column, len(cols))
	for i, c := range cols {
		width := perCol
		nameLen := len(c.Name)
		if nameLen+2 > width && nameLen+2 <= 40 {
			width = nameLen + 2
		}
		result[i] = table.Column{Title: c.Name, Width: width}
	}
	return result
}

// SetSize updates the table view dimensions.
func (m *TableViewModel) SetSize(width, height int) {
	m.width = width
	m.height = height
	m.ready = true

	colMeta := make([]ColumnMeta, len(m.columns))
	for i, name := range m.columns {
		typ := ""
		if i < len(m.types) {
			typ = m.types[i]
		}
		colMeta[i] = ColumnMeta{Name: name, Type: typ}
	}
	m.table.SetColumns(calculateColumns(colMeta, width))

	tableHeight := height - 1
	if tableHeight < 3 {
		tableHeight = 3
	}
	m.table.SetHeight(tableHeight)
}

// SetData populates the table with fetched data from a Response.
func (m *TableViewModel) SetData(resp *Response) {
	if resp == nil {
		return
	}

	// Remember current cursor position for live updates
	prevCursor := m.table.Cursor()

	rows := make([]table.Row, len(resp.Rows))
	for i, row := range resp.Rows {
		strRow := make(table.Row, len(row))
		for j, val := range row {
			strRow[j] = formatCellValue(val)
		}
		rows[i] = strRow
	}

	if len(resp.Columns) > 0 {
		m.columns = resp.Columns
		m.types = resp.Types
		colMeta := make([]ColumnMeta, len(resp.Columns))
		for i, name := range resp.Columns {
			typ := ""
			if i < len(resp.Types) {
				typ = resp.Types[i]
			}
			colMeta[i] = ColumnMeta{Name: name, Type: typ}
		}
		m.table.SetColumns(calculateColumns(colMeta, m.width))
	}

	m.table.SetRows(rows)
	m.totalRows = resp.TotalRows
	m.dataOffset = resp.Offset
	m.loading = false

	// Restore cursor position, clamped to new row count
	if prevCursor > 0 && len(rows) > 0 {
		if prevCursor >= len(rows) {
			prevCursor = len(rows) - 1
		}
		m.table.SetCursor(prevCursor)
	}
}

func formatCellValue(val any) string {
	if val == nil {
		return "null"
	}
	switch v := val.(type) {
	case float64:
		if v == float64(int64(v)) {
			return strconv.FormatInt(int64(v), 10)
		}
		return strconv.FormatFloat(v, 'f', -1, 64)
	case string:
		return v
	case bool:
		if v {
			return "true"
		}
		return "false"
	default:
		return fmt.Sprintf("%v", v)
	}
}

// Update handles table-specific key events (scrolling, selection).
func (m TableViewModel) Update(msg tea.Msg) (TableViewModel, tea.Cmd) {
	if !m.ready {
		return m, nil
	}

	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

// View renders the table and status bar.
func (m TableViewModel) View() string {
	if !m.ready {
		return "Loading..."
	}

	return m.table.View() + "\n" + m.statusBar()
}

// SetSubscribed updates the subscription status for the LIVE indicator.
func (m *TableViewModel) SetSubscribed(subscribed bool) {
	m.isSubscribed = subscribed
}

func (m TableViewModel) statusBar() string {
	rowInfo := fmt.Sprintf("%d rows", m.totalRows)
	if m.totalRows == 0 {
		rowInfo = "empty"
	}

	var refreshInfo string
	if m.isRefreshing && m.isSubscribed {
		refreshInfo = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00FF00")).
			Bold(true).
			Render("LIVE")
	} else if m.isRefreshing {
		refreshInfo = "refreshing"
	} else {
		refreshInfo = "static"
	}

	if m.loading {
		return tui.StyleDim.Render(fmt.Sprintf("  %s | %s | loading...", m.name, rowInfo))
	}

	return tui.StyleDim.Render(fmt.Sprintf("  %s | %s | ", m.name, rowInfo)) + refreshInfo
}

// Focus enables keyboard navigation in the table.
func (m *TableViewModel) Focus() {
	m.table.Focus()
}

// Blur disables keyboard navigation in the table.
func (m *TableViewModel) Blur() {
	m.table.Blur()
}
