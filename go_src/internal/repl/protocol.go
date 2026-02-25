package repl

import (
	"strconv"
	"sync/atomic"
)

var cmdCounter atomic.Int64

func nextID() string {
	return strconv.FormatInt(cmdCounter.Add(1), 10)
}

// Command represents a message sent from Go to the Python subprocess.
type Command struct {
	Type   string `json:"type"`
	ID     string `json:"id"`
	Code   string `json:"code,omitempty"`
	Name   string `json:"name,omitempty"`
	Offset *int `json:"offset,omitempty"`
	Limit  *int `json:"limit,omitempty"`
}

// NewExecuteCmd creates an execute command for the given code.
func NewExecuteCmd(code string) Command {
	return Command{Type: "execute", ID: nextID(), Code: code}
}

// NewListTablesCmd creates a list_tables command.
func NewListTablesCmd() Command {
	return Command{Type: "list_tables", ID: nextID()}
}

// NewFetchTableCmd creates a fetch_table command.
func NewFetchTableCmd(name string, offset, limit int) Command {
	return Command{Type: "fetch_table", ID: nextID(), Name: name, Offset: &offset, Limit: &limit}
}

// NewServerInfoCmd creates a server_info command.
func NewServerInfoCmd() Command {
	return Command{Type: "server_info", ID: nextID()}
}

// NewShutdownCmd creates a shutdown command.
func NewShutdownCmd() Command {
	return Command{Type: "shutdown", ID: nextID()}
}

// NewSubscribeCmd creates a subscribe command for live table updates.
func NewSubscribeCmd(name string, offset, limit int) Command {
	return Command{Type: "subscribe", ID: nextID(), Name: name, Offset: &offset, Limit: &limit}
}

// NewUnsubscribeCmd creates an unsubscribe command.
func NewUnsubscribeCmd(name string) Command {
	return Command{Type: "unsubscribe", ID: nextID(), Name: name}
}

// Response is a union type covering all Python -> Go messages.
// Only the fields relevant to the Type are populated.
type Response struct {
	Type string `json:"type"`
	ID   string `json:"id,omitempty"`

	// "ready" fields
	Port    int    `json:"port,omitempty"`
	Version string `json:"version,omitempty"`
	Mode    string `json:"mode,omitempty"`

	// "result" fields
	Stdout         string   `json:"stdout,omitempty"`
	Stderr         string   `json:"stderr,omitempty"`
	Error          *string  `json:"error"`
	ResultRepr     *string  `json:"result_repr"`
	AssignedTables []string `json:"assigned_tables,omitempty"`
	AllTables      []string `json:"all_tables,omitempty"`
	ElapsedMs      int      `json:"elapsed_ms,omitempty"`

	// "tables" fields
	Tables []TableMeta `json:"tables,omitempty"`

	// "table_data" / "table_update" fields
	Name         string   `json:"name,omitempty"`
	Columns      []string `json:"columns,omitempty"`
	Types        []string `json:"types,omitempty"`
	Rows         [][]any  `json:"rows,omitempty"`
	TotalRows    int      `json:"total_rows,omitempty"`
	Offset       int      `json:"offset,omitempty"`
	IsRefreshing bool     `json:"is_refreshing,omitempty"`

	// "server_info" fields
	Host       string `json:"host,omitempty"`
	TableCount int    `json:"table_count,omitempty"`

	// "error" fields
	Message string `json:"message,omitempty"`
}

// TableMeta holds metadata for a single table.
type TableMeta struct {
	Name         string       `json:"name"`
	RowCount     int          `json:"row_count"`
	IsRefreshing bool         `json:"is_refreshing"`
	Columns      []ColumnMeta `json:"columns"`
}

// ColumnMeta holds metadata for a single column.
type ColumnMeta struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

func (r *Response) IsReady() bool          { return r.Type == "ready" }
func (r *Response) IsResult() bool         { return r.Type == "result" }
func (r *Response) IsError() bool          { return r.Type == "error" }
func (r *Response) IsShutdownAck() bool    { return r.Type == "shutdown_ack" }
func (r *Response) HasError() bool         { return r.Error != nil && *r.Error != "" }
func (r *Response) IsTableUpdate() bool    { return r.Type == "table_update" }
func (r *Response) IsSubscribeAck() bool   { return r.Type == "subscribe_ack" }
func (r *Response) IsUnsubscribeAck() bool { return r.Type == "unsubscribe_ack" }
