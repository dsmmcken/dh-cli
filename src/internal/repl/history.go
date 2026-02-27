package repl

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// History manages REPL command history with file persistence.
type History struct {
	entries []string
	cursor  int
	draft   string
	path    string
	maxSize int
}

// NewHistory creates a history manager that loads from ~/.dh/repl_history.
func NewHistory(dhHome string) *History {
	path := filepath.Join(dhHome, "repl_history")
	h := &History{
		entries: []string{},
		cursor:  -1,
		path:    path,
		maxSize: 500,
	}
	h.load()
	return h
}

func (h *History) load() {
	data, err := os.ReadFile(h.path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var entry string
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			// Backwards compat: treat non-JSON lines as literal strings
			entry = line
		}
		h.entries = append(h.entries, entry)
	}
	if len(h.entries) > h.maxSize {
		h.entries = h.entries[len(h.entries)-h.maxSize:]
	}
}

// Add appends a command to history, deduplicating consecutive entries.
func (h *History) Add(cmd string) {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return
	}
	if len(h.entries) > 0 && h.entries[len(h.entries)-1] == cmd {
		return
	}
	h.entries = append(h.entries, cmd)
	if len(h.entries) > h.maxSize {
		h.entries = h.entries[len(h.entries)-h.maxSize:]
	}
	h.cursor = -1
	h.draft = ""
	h.save()
}

func (h *History) save() {
	dir := filepath.Dir(h.path)
	os.MkdirAll(dir, 0o755)

	var buf strings.Builder
	for _, entry := range h.entries {
		data, _ := json.Marshal(entry)
		buf.Write(data)
		buf.WriteByte('\n')
	}
	os.WriteFile(h.path, []byte(buf.String()), 0o644)
}

// Up moves to the previous (older) history entry.
func (h *History) Up(currentInput string) (string, bool) {
	if len(h.entries) == 0 {
		return "", false
	}
	if h.cursor == -1 {
		h.draft = currentInput
		h.cursor = len(h.entries) - 1
	} else if h.cursor > 0 {
		h.cursor--
	} else {
		return h.entries[0], false
	}
	return h.entries[h.cursor], true
}

// Down moves to the next (newer) history entry.
func (h *History) Down(currentInput string) (string, bool) {
	if h.cursor == -1 {
		return "", false
	}
	if h.cursor < len(h.entries)-1 {
		h.cursor++
		return h.entries[h.cursor], true
	}
	h.cursor = -1
	return h.draft, true
}

// ResetNavigation resets the cursor position.
func (h *History) ResetNavigation() {
	h.cursor = -1
	h.draft = ""
}

// Search returns entries matching the query, most recent first.
func (h *History) Search(query string) []string {
	if query == "" {
		return nil
	}
	query = strings.ToLower(query)
	var matches []string
	for i := len(h.entries) - 1; i >= 0; i-- {
		if strings.Contains(strings.ToLower(h.entries[i]), query) {
			matches = append(matches, h.entries[i])
		}
	}
	return matches
}
