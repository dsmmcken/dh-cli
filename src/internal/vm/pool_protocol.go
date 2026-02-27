package vm

// PoolRequest is sent from the client (dh exec --vm / dh vm pool) to the pool daemon.
type PoolRequest struct {
	Type          string `json:"type"`                      // "exec", "scale", "status", "stop"
	Code          string `json:"code,omitempty"`            // for exec
	CWD           string `json:"cwd,omitempty"`             // for exec
	ShowTables    bool   `json:"show_tables,omitempty"`     // for exec
	ShowTableMeta bool   `json:"show_table_meta,omitempty"` // for exec
	TargetSize    int    `json:"target_size,omitempty"`     // for scale
}

// PoolResponse is sent from the pool daemon to the client.
type PoolResponse struct {
	Type    string          `json:"type"`              // "exec_result", "status", "error", "ok"
	Exec    *VsockResponse  `json:"exec,omitempty"`    // for exec_result
	Status  *PoolStatus     `json:"status,omitempty"`  // for status
	Error   string          `json:"error,omitempty"`   // for error
	Version string          `json:"version,omitempty"` // pool's version
}

// PoolStatus describes the current state of the pool daemon.
type PoolStatus struct {
	Running     bool   `json:"running"`
	PID         int    `json:"pid"`
	Version     string `json:"version"`
	Ready       int    `json:"ready"`
	TargetSize  int    `json:"target_size"`
	IdleSeconds int    `json:"idle_seconds"`
	IdleTimeout int    `json:"idle_timeout_seconds"`
}
