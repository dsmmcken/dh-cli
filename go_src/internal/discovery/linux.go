//go:build linux

package discovery

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const tcpListen = 0x0A // TCP_LISTEN state in /proc/net/tcp

// discoverProcesses finds listening TCP sockets on Linux by parsing /proc/net/tcp and /proc/net/tcp6.
func discoverProcesses() ([]Server, error) {
	inodeToPID := buildInodePIDMap()

	var servers []Server
	for _, path := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		entries, err := parseProcNetTCP(path)
		if err != nil {
			continue // file may not exist
		}
		for _, entry := range entries {
			if entry.State != tcpListen {
				continue
			}
			pid := inodeToPID[entry.Inode]
			if pid == 0 {
				continue
			}
			source := ClassifyProcess(pid)
			cwd := readProcSymlink(pid, "cwd")
			servers = append(servers, Server{
				Port:   entry.Port,
				PID:    pid,
				Source: source,
				Script: readProcComm(pid),
				CWD:    cwd,
			})
		}
	}
	return servers, nil
}

// parseProcNetTCP reads and parses a /proc/net/tcp file.
func parseProcNetTCP(path string) ([]TCPEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseProcNetTCPContent(string(data)), nil
}

// buildInodePIDMap scans /proc/*/fd/* to build a mapping from socket inode to PID.
func buildInodePIDMap() map[uint64]int {
	result := make(map[uint64]int)

	entries, err := os.ReadDir("/proc")
	if err != nil {
		return result
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}

		fdDir := fmt.Sprintf("/proc/%d/fd", pid)
		fds, err := os.ReadDir(fdDir)
		if err != nil {
			continue
		}

		for _, fd := range fds {
			link, err := os.Readlink(filepath.Join(fdDir, fd.Name()))
			if err != nil {
				continue
			}
			// Links look like: socket:[12345]
			if !strings.HasPrefix(link, "socket:[") {
				continue
			}
			inodeStr := link[8 : len(link)-1]
			inode, err := strconv.ParseUint(inodeStr, 10, 64)
			if err != nil {
				continue
			}
			result[inode] = pid
		}
	}
	return result
}

// readProcComm reads /proc/<pid>/comm.
func readProcComm(pid int) string {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// readProcSymlink reads a symlink from /proc/<pid>/<name>.
func readProcSymlink(pid int, name string) string {
	link, err := os.Readlink(fmt.Sprintf("/proc/%d/%s", pid, name))
	if err != nil {
		return ""
	}
	return link
}
