package discovery

import (
	"fmt"
	"os"
	"strings"
)

// Server represents a discovered Deephaven server instance.
type Server struct {
	Port        int    `json:"port"`
	PID         int    `json:"pid,omitempty"`
	Source      string `json:"source"`
	Script      string `json:"script,omitempty"`
	CWD         string `json:"cwd,omitempty"`
	ContainerID string `json:"container_id,omitempty"`
}

// Discover finds all running Deephaven servers by combining platform-specific
// process discovery with Docker container discovery. Results are deduplicated by port.
func Discover() ([]Server, error) {
	procServers, err := discoverProcesses()
	if err != nil {
		return nil, fmt.Errorf("process discovery: %w", err)
	}

	dockerServers, err := discoverDocker()
	if err != nil {
		return nil, fmt.Errorf("docker discovery: %w", err)
	}

	return deduplicateByPort(procServers, dockerServers), nil
}

// deduplicateByPort merges two server lists, preferring process-based entries over docker
// when both exist on the same port.
func deduplicateByPort(procServers, dockerServers []Server) []Server {
	seen := make(map[int]bool)
	var result []Server

	for _, s := range procServers {
		if !seen[s.Port] {
			seen[s.Port] = true
			result = append(result, s)
		}
	}
	for _, s := range dockerServers {
		if !seen[s.Port] {
			seen[s.Port] = true
			result = append(result, s)
		}
	}
	return result
}

// ClassifyProcess determines the source type of a process based on its command line.
func ClassifyProcess(pid int) string {
	cmdline := readProcCmdline(pid)
	return classifyCmdline(cmdline)
}

// classifyCmdline classifies a process based on its command line string.
func classifyCmdline(cmdline string) string {
	lower := strings.ToLower(cmdline)

	if strings.Contains(lower, "dh serve") || strings.Contains(lower, "dh-serve") {
		return "dh serve"
	}
	if strings.Contains(lower, "dh repl") || strings.Contains(lower, "dh-repl") {
		return "dh repl"
	}
	if strings.Contains(lower, "deephaven") && strings.Contains(lower, "java") {
		return "java"
	}
	if strings.Contains(lower, "java") && strings.Contains(lower, "deephaven") {
		return "java"
	}
	if strings.Contains(lower, "java") {
		return "java"
	}
	return "unknown"
}

// ClassifyCmdlineForTest exposes classifyCmdline for unit testing.
func ClassifyCmdlineForTest(cmdline string) string {
	return classifyCmdline(cmdline)
}

// DeduplicateByPortForTest exposes deduplicateByPort for unit testing.
func DeduplicateByPortForTest(proc, docker []Server) []Server {
	return deduplicateByPort(proc, docker)
}

// readProcCmdline reads the command line for a given PID.
// Returns empty string if it cannot be read.
func readProcCmdline(pid int) string {
	path := fmt.Sprintf("/proc/%d/cmdline", pid)
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	// cmdline uses null bytes as separators
	return strings.ReplaceAll(string(data), "\x00", " ")
}
