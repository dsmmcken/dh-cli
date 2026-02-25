package discovery

import (
	"strconv"
	"strings"
)

// TCPEntry represents a parsed line from /proc/net/tcp.
type TCPEntry struct {
	Port  int
	Inode uint64
	State int
}

// ParseProcNetTCPContent parses the content of /proc/net/tcp or /proc/net/tcp6.
func ParseProcNetTCPContent(content string) []TCPEntry {
	var entries []TCPEntry
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if i == 0 {
			continue // skip header
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		entry, ok := parseTCPLine(line)
		if ok {
			entries = append(entries, entry)
		}
	}
	return entries
}

// parseTCPLine parses a single line from /proc/net/tcp.
// Format: sl local_address rem_address st tx_queue:rx_queue tr:tm->when retrnsmt uid timeout inode
func parseTCPLine(line string) (TCPEntry, bool) {
	fields := strings.Fields(line)
	if len(fields) < 10 {
		return TCPEntry{}, false
	}

	// local_address is fields[1], format: hex_ip:hex_port
	localParts := strings.SplitN(fields[1], ":", 2)
	if len(localParts) != 2 {
		return TCPEntry{}, false
	}
	port, err := strconv.ParseInt(localParts[1], 16, 32)
	if err != nil {
		return TCPEntry{}, false
	}

	// state is fields[3]
	state, err := strconv.ParseInt(fields[3], 16, 32)
	if err != nil {
		return TCPEntry{}, false
	}

	// inode is fields[9]
	inode, err := strconv.ParseUint(fields[9], 10, 64)
	if err != nil {
		return TCPEntry{}, false
	}

	return TCPEntry{
		Port:  int(port),
		Inode: inode,
		State: int(state),
	}, true
}

// ParseLsofOutput parses lsof -F pcn output into servers.
func ParseLsofOutput(content string) []Server {
	var servers []Server
	var currentPID int
	var currentComm string

	lines := strings.Split(content, "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		switch line[0] {
		case 'p':
			pid, err := strconv.Atoi(line[1:])
			if err != nil {
				continue
			}
			currentPID = pid
		case 'c':
			currentComm = line[1:]
		case 'n':
			// Format: n*:port or nhost:port
			addr := line[1:]
			idx := strings.LastIndex(addr, ":")
			if idx < 0 {
				continue
			}
			portStr := addr[idx+1:]
			port, err := strconv.Atoi(portStr)
			if err != nil {
				continue
			}
			source := classifyCmdline(currentComm)
			if source == "" {
				continue
			}
			servers = append(servers, Server{
				Port:   port,
				PID:    currentPID,
				Source: source,
				Script: currentComm,
			})
		}
	}
	return servers
}
