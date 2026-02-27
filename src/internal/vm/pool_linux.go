//go:build linux

package vm

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	firecracker "github.com/firecracker-microvm/firecracker-go-sdk"
)

// Pool manages a set of pre-warmed Firecracker VMs for fast execution.
type Pool struct {
	mu          sync.Mutex
	version     string
	paths       *VMPaths
	dhHome      string
	targetSize  int
	idleTimeout time.Duration
	verbose     bool
	useUffd     bool

	// Warm VM queue — buffered channel acts as a thread-safe FIFO.
	ready chan *poolVM

	// Lifecycle
	listener net.Listener
	lastReq  time.Time
	done     chan struct{}
	wg       sync.WaitGroup
	stderr   io.Writer
}

// poolVM is a pre-warmed VM instance waiting in the ready queue.
type poolVM struct {
	info       *InstanceInfo
	machine    *firecracker.Machine
	uffdCloser io.Closer
	vsockPath  string
	instanceID string
}

// PoolConfig configures a new Pool.
type PoolConfig struct {
	DHHome      string
	Version     string
	TargetSize  int
	IdleTimeout time.Duration
	Verbose     bool
	UseUffd     bool
}

// NewPool creates a new pool manager. Call Start to begin operation.
func NewPool(cfg PoolConfig) *Pool {
	paths := NewVMPaths(cfg.DHHome)
	return &Pool{
		version:     cfg.Version,
		paths:       paths,
		dhHome:      cfg.DHHome,
		targetSize:  cfg.TargetSize,
		idleTimeout: cfg.IdleTimeout,
		verbose:     cfg.Verbose,
		useUffd:     cfg.UseUffd,
		ready:       make(chan *poolVM, cfg.TargetSize+4), // small buffer headroom
		done:        make(chan struct{}),
		lastReq:     time.Now(),
	}
}

// Start fills the pool, starts the Unix socket listener, and runs the
// idle timer. Blocks until Shutdown is called or context is cancelled.
func (p *Pool) Start(ctx context.Context, stderr io.Writer) error {
	p.stderr = stderr

	// Verify snapshot exists
	if err := CheckSnapshot(p.paths, p.version); err != nil {
		return err
	}

	// Start page cache warming
	WarmSnapshotPageCacheAsync(p.paths, p.version)

	// Pre-fill pool
	for i := 0; i < p.targetSize; i++ {
		if err := p.fillOne(ctx); err != nil {
			p.log("Warning: failed to pre-fill VM %d/%d: %v", i+1, p.targetSize, err)
		} else {
			p.log("Pre-warmed VM %d/%d", i+1, p.targetSize)
		}
	}

	// Start Unix socket listener
	socketPath := PoolSocketPath()
	os.Remove(socketPath)
	var err error
	p.listener, err = net.Listen("unix", socketPath)
	if err != nil {
		p.drainAll()
		return fmt.Errorf("listening on %s: %w", socketPath, err)
	}

	p.log("Pool daemon listening on %s (version=%s, pool_size=%d, idle_timeout=%s)",
		socketPath, p.version, p.targetSize, p.idleTimeout)

	// Start backfill goroutine
	p.wg.Add(1)
	go p.backfillLoop(ctx)

	// Start idle watcher
	p.wg.Add(1)
	go p.idleWatcher()

	// Accept loop (blocks until listener closed)
	p.wg.Add(1)
	go p.acceptLoop(ctx)

	// Wait for shutdown signal
	select {
	case <-ctx.Done():
		p.Shutdown()
	case <-p.done:
	}

	p.wg.Wait()
	return nil
}

// fillOne restores one VM from snapshot and puts it in the ready channel.
func (p *Pool) fillOne(ctx context.Context) error {
	instanceID := fmt.Sprintf("pool-%d", time.Now().UnixNano())
	instanceDir := p.paths.InstanceDir(instanceID)
	if err := os.MkdirAll(instanceDir, 0o755); err != nil {
		return fmt.Errorf("creating instance dir: %w", err)
	}

	vsockPath := fmt.Sprintf("%s/vsock.sock", instanceDir)

	cfg := &VMConfig{
		DHHome:       p.dhHome,
		Version:      p.version,
		Verbose:      false, // suppress per-VM verbose output in pool
		UseUffd:      p.useUffd,
		VsockUDSPath: vsockPath,
		ReadOnlyDisk: true,
	}

	info, machine, uffdCloser, err := RestoreFromSnapshot(ctx, cfg, p.paths, io.Discard)
	if err != nil {
		os.RemoveAll(instanceDir)
		return fmt.Errorf("restoring snapshot: %w", err)
	}

	pvm := &poolVM{
		info:       info,
		machine:    machine,
		uffdCloser: uffdCloser,
		vsockPath:  vsockPath,
		instanceID: instanceID,
	}

	select {
	case p.ready <- pvm:
		return nil
	default:
		// Channel full — destroy this VM
		p.destroyPoolVM(pvm)
		return fmt.Errorf("ready channel full")
	}
}

// backfillLoop keeps the ready channel at target size.
func (p *Pool) backfillLoop(ctx context.Context) {
	defer p.wg.Done()
	for {
		select {
		case <-p.done:
			return
		default:
		}

		p.mu.Lock()
		target := p.targetSize
		p.mu.Unlock()

		current := len(p.ready)
		if current < target {
			if err := p.fillOne(ctx); err != nil {
				p.log("Backfill error: %v", err)
				// Back off briefly on error to avoid tight loops
				select {
				case <-time.After(500 * time.Millisecond):
				case <-p.done:
					return
				}
			}
		} else {
			// Pool is full, wait a bit before checking again
			select {
			case <-time.After(100 * time.Millisecond):
			case <-p.done:
				return
			}
		}
	}
}

// acceptLoop accepts connections on the Unix socket.
func (p *Pool) acceptLoop(ctx context.Context) {
	defer p.wg.Done()
	for {
		conn, err := p.listener.Accept()
		if err != nil {
			select {
			case <-p.done:
				return
			default:
				continue
			}
		}
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			p.handleConnection(ctx, conn)
		}()
	}
}

// handleConnection reads a single request and dispatches it.
func (p *Pool) handleConnection(ctx context.Context, conn net.Conn) {
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(5 * time.Minute))

	reader := bufio.NewReader(conn)
	line, err := reader.ReadBytes('\n')
	if err != nil {
		return
	}

	var req PoolRequest
	if err := json.Unmarshal(line, &req); err != nil {
		p.sendResponse(conn, &PoolResponse{Type: "error", Error: "invalid request JSON"})
		return
	}

	switch req.Type {
	case "exec":
		p.handleExec(ctx, conn, &req)
	case "status":
		p.handleStatus(conn)
	case "scale":
		p.handleScale(conn, req.TargetSize)
	case "stop":
		p.sendResponse(conn, &PoolResponse{Type: "ok"})
		go p.Shutdown()
	default:
		p.sendResponse(conn, &PoolResponse{Type: "error", Error: fmt.Sprintf("unknown request type: %s", req.Type)})
	}
}

// handleExec dequeues a warm VM, starts a file server, executes code, and
// destroys the VM. Triggers backfill to replace the consumed VM.
func (p *Pool) handleExec(ctx context.Context, conn net.Conn, req *PoolRequest) {
	p.mu.Lock()
	p.lastReq = time.Now()
	p.mu.Unlock()

	// Non-blocking dequeue — fail fast if no warm VM is available so the
	// client can fall through to cold restore immediately.
	var pvm *poolVM
	select {
	case pvm = <-p.ready:
	default:
		p.sendResponse(conn, &PoolResponse{Type: "error", Error: "no warm VMs available"})
		return
	}

	defer p.destroyPoolVM(pvm)

	// Start file server at the SNAPSHOT vsock path (not per-instance path).
	// Firecracker remembers the original vsock UDS path internally and uses
	// it to construct guest-to-host listener paths ({vsockPath}_{port}).
	// The renamed per-instance socket works for host-to-guest connections
	// (ExecuteViaVsock), but guest-to-host (file server) must use the
	// original snapshot path.
	snapVsockPath := filepath.Join(p.paths.SnapshotDirForVersion(p.version), "vsock.sock")
	cwd := req.CWD
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	fileServer, err := StartFileServer(snapVsockPath, cwd)
	if err != nil {
		p.log("Warning: file server for %s: %v", pvm.instanceID, err)
	}
	if fileServer != nil {
		defer fileServer.Close()
	}

	// Execute code via vsock — use the per-instance (renamed) path for
	// host-to-guest communication.
	vsockReq := &VsockRequest{
		Code:          req.Code,
		ShowTables:    req.ShowTables,
		ShowTableMeta: req.ShowTableMeta,
	}

	resp, err := ExecuteViaVsock(pvm.vsockPath, VsockPort, vsockReq)
	if err != nil {
		p.sendResponse(conn, &PoolResponse{
			Type:    "error",
			Error:   fmt.Sprintf("vsock exec: %v", err),
			Version: p.version,
		})
		return
	}

	p.sendResponse(conn, &PoolResponse{
		Type:    "exec_result",
		Exec:    resp,
		Version: p.version,
	})
}

// handleStatus returns the current pool state.
func (p *Pool) handleStatus(conn net.Conn) {
	p.mu.Lock()
	idleSecs := int(time.Since(p.lastReq).Seconds())
	target := p.targetSize
	p.mu.Unlock()

	status := &PoolStatus{
		Running:     true,
		PID:         os.Getpid(),
		Version:     p.version,
		Ready:       len(p.ready),
		TargetSize:  target,
		IdleSeconds: idleSecs,
		IdleTimeout: int(p.idleTimeout.Seconds()),
	}
	p.sendResponse(conn, &PoolResponse{Type: "status", Status: status, Version: p.version})
}

// handleScale adjusts the target pool size.
func (p *Pool) handleScale(conn net.Conn, newSize int) {
	if newSize < 0 {
		p.sendResponse(conn, &PoolResponse{Type: "error", Error: "target size must be >= 0"})
		return
	}

	p.mu.Lock()
	oldSize := p.targetSize
	p.targetSize = newSize
	p.mu.Unlock()

	// If shrinking, drain excess VMs
	for len(p.ready) > newSize {
		select {
		case pvm := <-p.ready:
			p.destroyPoolVM(pvm)
		default:
			break
		}
	}

	p.log("Pool scaled: %d → %d", oldSize, newSize)
	p.sendResponse(conn, &PoolResponse{Type: "ok", Version: p.version})
}

// idleWatcher shuts down the pool after idleTimeout of inactivity.
func (p *Pool) idleWatcher() {
	defer p.wg.Done()
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-p.done:
			return
		case <-ticker.C:
			p.mu.Lock()
			idle := time.Since(p.lastReq)
			timeout := p.idleTimeout
			p.mu.Unlock()

			if timeout > 0 && idle > timeout {
				p.log("Idle timeout reached (%.0fs > %s), shutting down", idle.Seconds(), timeout)
				go p.Shutdown()
				return
			}
		}
	}
}

// Shutdown gracefully stops the pool: closes the listener, drains all VMs,
// removes the socket file, and signals all goroutines to exit.
func (p *Pool) Shutdown() {
	p.mu.Lock()
	select {
	case <-p.done:
		p.mu.Unlock()
		return // already shutting down
	default:
		close(p.done)
	}
	p.mu.Unlock()

	p.log("Shutting down pool daemon...")

	if p.listener != nil {
		p.listener.Close()
	}

	// Remove socket file
	os.Remove(PoolSocketPath())

	// Drain and destroy all warm VMs
	p.drainAll()
}

// drainAll destroys all VMs in the ready channel.
func (p *Pool) drainAll() {
	for {
		select {
		case pvm := <-p.ready:
			p.destroyPoolVM(pvm)
		default:
			return
		}
	}
}

// destroyPoolVM tears down a single pool VM.
func (p *Pool) destroyPoolVM(pvm *poolVM) {
	if pvm == nil {
		return
	}
	DestroyInstance(pvm.machine, pvm.info, p.paths)
	if pvm.uffdCloser != nil {
		pvm.uffdCloser.Close()
	}
}

// sendResponse writes a JSON response to the connection.
func (p *Pool) sendResponse(conn net.Conn, resp *PoolResponse) {
	data, err := json.Marshal(resp)
	if err != nil {
		return
	}
	data = append(data, '\n')
	conn.Write(data)
}

// log writes a timestamped message to stderr if verbose, or always for
// important lifecycle events.
func (p *Pool) log(format string, args ...any) {
	if p.stderr != nil {
		msg := fmt.Sprintf(format, args...)
		fmt.Fprintf(p.stderr, "[pool] %s %s\n", time.Now().Format("15:04:05"), msg)
	}
}
