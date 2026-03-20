package vm

// VsockRequest is the JSON request sent from the host to the VM runner daemon.
type VsockRequest struct {
	Code          string   `json:"code"`
	ShowTables    bool     `json:"show_tables"`
	ShowTableMeta bool     `json:"show_table_meta"`
	Render        bool     `json:"render,omitempty"`
	Widget        string   `json:"widget,omitempty"`
	Actions       []string `json:"actions,omitempty"`
	RenderTimeout int      `json:"render_timeout,omitempty"`
	MaxRows       int      `json:"max_rows,omitempty"`
	RenderJSON    bool     `json:"render_json,omitempty"`
	Verbose       bool     `json:"verbose,omitempty"`
}

// VsockResponse is the JSON response from the VM runner daemon.
type VsockResponse struct {
	ExitCode     int            `json:"exit_code"`
	Stdout       string         `json:"stdout"`
	Stderr       string         `json:"stderr"`
	ResultRepr   *string        `json:"result_repr"`
	Error        *string        `json:"error"`
	Tables       []any          `json:"tables"`
	Timing       map[string]any `json:"_timing,omitempty"`
	RenderOutput string         `json:"render_output,omitempty"`
}
