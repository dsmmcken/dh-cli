//go:build linux

package vm

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// File server operation codes (guest → host).
const (
	opStat    = 1
	opRead    = 2
	opReaddir = 3
)

// File server response status codes (host → guest).
const (
	statusOK     = 0
	statusNoent  = 1
	statusIO     = 2
)

// fileServer serves host files over a Firecracker vsock connection.
// It listens on {vsockPath}_{FileServerPort} for guest connections.
type fileServer struct {
	rootDir  string
	listener net.Listener
	done     chan struct{}
	wg       sync.WaitGroup
}

// StartFileServer starts a goroutine-based file server that serves files from
// rootDir over the Firecracker guest→host vsock mechanism. The listener socket
// is at vsockPath_10001 (Firecracker convention: guest CID=2:port → host UDS).
func StartFileServer(vsockPath string, rootDir string) (io.Closer, error) {
	listenPath := fmt.Sprintf("%s_%d", vsockPath, FileServerPort)

	// Remove stale socket from previous runs.
	os.Remove(listenPath)

	listener, err := net.Listen("unix", listenPath)
	if err != nil {
		return nil, fmt.Errorf("listening on %s: %w", listenPath, err)
	}

	fs := &fileServer{
		rootDir:  rootDir,
		listener: listener,
		done:     make(chan struct{}),
	}

	fs.wg.Add(1)
	go fs.acceptLoop()

	return fs, nil
}

func (fs *fileServer) Close() error {
	close(fs.done)
	err := fs.listener.Close()
	fs.wg.Wait()
	return err
}

func (fs *fileServer) acceptLoop() {
	defer fs.wg.Done()
	for {
		conn, err := fs.listener.Accept()
		if err != nil {
			select {
			case <-fs.done:
				return
			default:
				continue
			}
		}
		fs.wg.Add(1)
		go func() {
			defer fs.wg.Done()
			fs.handleConn(conn)
		}()
	}
}

// handleConn processes requests on a single vsock connection.
// Firecracker prepends a "CONNECT <port>\n" / "OK <local>\n" handshake
// before forwarding raw bytes. We handle that first, then process
// length-prefixed binary messages in a loop.
func (fs *fileServer) handleConn(conn net.Conn) {
	defer conn.Close()

	// Read and discard the Firecracker vsock handshake line.
	// The guest kernel sends "CONNECT <port>\n"; Firecracker translates
	// and forwards the connection to us, but the first bytes on the UDS
	// are just the raw stream — no handshake on the host-side listener.
	// So we go straight to the binary protocol.

	for {
		// Read length-prefixed message: [4-byte big-endian length][payload]
		var msgLen uint32
		if err := binary.Read(conn, binary.BigEndian, &msgLen); err != nil {
			return // connection closed or error
		}
		if msgLen == 0 || msgLen > 16*1024*1024 { // sanity: max 16 MiB
			return
		}

		payload := make([]byte, msgLen)
		if _, err := io.ReadFull(conn, payload); err != nil {
			return
		}

		fs.handleMessage(conn, payload)
	}
}

func (fs *fileServer) handleMessage(conn net.Conn, payload []byte) {
	if len(payload) < 1 {
		writeError(conn, statusIO)
		return
	}

	op := payload[0]
	rest := payload[1:]

	switch op {
	case opStat:
		fs.handleStat(conn, rest)
	case opRead:
		fs.handleRead(conn, rest)
	case opReaddir:
		fs.handleReaddir(conn, rest)
	default:
		writeError(conn, statusIO)
	}
}

// handleStat: [2-byte path_len][path_bytes]
// Response:   [status=0][4-byte mode][8-byte size][8-byte mtime_sec][1-byte is_dir]
func (fs *fileServer) handleStat(conn net.Conn, data []byte) {
	relPath, ok := readPath(data)
	if !ok {
		writeError(conn, statusIO)
		return
	}

	absPath, err := fs.safePath(relPath)
	if err != nil {
		writeError(conn, statusNoent)
		return
	}

	fi, err := os.Stat(absPath)
	if err != nil {
		writeError(conn, statusNoent)
		return
	}

	var isDir uint8
	if fi.IsDir() {
		isDir = 1
	}

	// [4-byte length][status=0][4-byte mode][8-byte size][8-byte mtime][1-byte is_dir]
	resp := make([]byte, 4+1+4+8+8+1)
	binary.BigEndian.PutUint32(resp[0:4], uint32(1+4+8+8+1))
	resp[4] = statusOK
	binary.BigEndian.PutUint32(resp[5:9], uint32(fi.Mode()))
	binary.BigEndian.PutUint64(resp[9:17], uint64(fi.Size()))
	binary.BigEndian.PutUint64(resp[17:25], uint64(fi.ModTime().Unix()))
	resp[25] = isDir
	conn.Write(resp)
}

// handleRead: [2-byte path_len][path_bytes][8-byte offset][4-byte length]
// Response:   [status=0][4-byte bytes_read][raw file bytes]
func (fs *fileServer) handleRead(conn net.Conn, data []byte) {
	if len(data) < 2 {
		writeError(conn, statusIO)
		return
	}

	pathLen := binary.BigEndian.Uint16(data[0:2])
	if int(2+pathLen+8+4) > len(data) {
		writeError(conn, statusIO)
		return
	}

	relPath := string(data[2 : 2+pathLen])
	offset := binary.BigEndian.Uint64(data[2+pathLen : 2+pathLen+8])
	readLen := binary.BigEndian.Uint32(data[2+pathLen+8 : 2+pathLen+12])

	// Cap read size at 1 MiB per request.
	if readLen > 1024*1024 {
		readLen = 1024 * 1024
	}

	absPath, err := fs.safePath(relPath)
	if err != nil {
		writeError(conn, statusNoent)
		return
	}

	f, err := os.Open(absPath)
	if err != nil {
		writeError(conn, statusNoent)
		return
	}
	defer f.Close()

	buf := make([]byte, readLen)
	n, err := f.ReadAt(buf, int64(offset))
	if err != nil && err != io.EOF {
		if n == 0 {
			writeError(conn, statusIO)
			return
		}
	}

	// [4-byte length][status=0][4-byte bytes_read][raw bytes]
	hdr := make([]byte, 4+1+4)
	binary.BigEndian.PutUint32(hdr[0:4], uint32(1+4+n))
	hdr[4] = statusOK
	binary.BigEndian.PutUint32(hdr[5:9], uint32(n))
	conn.Write(hdr)
	conn.Write(buf[:n])
}

// handleReaddir: [2-byte path_len][path_bytes]
// Response:      [status=0][2-byte count][{2-byte name_len, name, 1-byte is_dir}...]
func (fs *fileServer) handleReaddir(conn net.Conn, data []byte) {
	relPath, ok := readPath(data)
	if !ok {
		writeError(conn, statusIO)
		return
	}

	absPath, err := fs.safePath(relPath)
	if err != nil {
		writeError(conn, statusNoent)
		return
	}

	entries, err := os.ReadDir(absPath)
	if err != nil {
		writeError(conn, statusNoent)
		return
	}

	// Build entry list
	var entryBuf []byte
	count := 0
	for _, e := range entries {
		name := e.Name()
		if len(name) > 65535 {
			continue
		}
		var isDir uint8
		if e.IsDir() {
			isDir = 1
		}
		nameLenBytes := make([]byte, 2)
		binary.BigEndian.PutUint16(nameLenBytes, uint16(len(name)))
		entryBuf = append(entryBuf, nameLenBytes...)
		entryBuf = append(entryBuf, []byte(name)...)
		entryBuf = append(entryBuf, isDir)
		count++
		if count >= 65535 {
			break
		}
	}

	// [4-byte length][status=0][2-byte count][entries...]
	hdr := make([]byte, 4+1+2)
	binary.BigEndian.PutUint32(hdr[0:4], uint32(1+2+len(entryBuf)))
	hdr[4] = statusOK
	binary.BigEndian.PutUint16(hdr[5:7], uint16(count))
	conn.Write(hdr)
	conn.Write(entryBuf)
}

// safePath validates and resolves a relative path against rootDir.
// Returns error if the path escapes rootDir via directory traversal.
func (fs *fileServer) safePath(relPath string) (string, error) {
	cleaned := filepath.Clean(relPath)
	if filepath.IsAbs(cleaned) {
		// Strip leading slash to make it relative.
		cleaned = cleaned[1:]
	}
	absPath := filepath.Join(fs.rootDir, cleaned)

	// Verify the resolved path is still under rootDir.
	resolved, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		// File may not exist yet for stat — try parent dir.
		resolved = absPath
	}
	if !isSubPath(fs.rootDir, resolved) {
		return "", fmt.Errorf("path escapes root: %s", relPath)
	}
	return absPath, nil
}

// isSubPath checks whether child is under (or equal to) parent.
func isSubPath(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	// "." means equal, anything starting with ".." escapes
	return rel == "." || (!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != "..")
}

// readPath extracts a path from [2-byte path_len][path_bytes].
func readPath(data []byte) (string, bool) {
	if len(data) < 2 {
		return "", false
	}
	pathLen := binary.BigEndian.Uint16(data[0:2])
	if int(2+pathLen) > len(data) {
		return "", false
	}
	return string(data[2 : 2+pathLen]), true
}

// writeError sends a length-prefixed error response.
func writeError(conn net.Conn, status byte) {
	resp := make([]byte, 5)
	binary.BigEndian.PutUint32(resp[0:4], 1)
	resp[4] = status
	conn.Write(resp)
}
