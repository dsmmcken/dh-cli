//go:build linux

package vm

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"syscall"
	"time"
)

// ErrPoolNotRunning is returned when the pool daemon socket cannot be reached.
var ErrPoolNotRunning = errors.New("pool daemon not running")

// PoolSocketPath returns the Unix socket path for the pool daemon.
// Uses the current user's UID to avoid conflicts between users.
func PoolSocketPath() string {
	return fmt.Sprintf("/tmp/dh-pool-%d.sock", os.Getuid())
}

// PoolProbe checks if a pool daemon is running by attempting to connect
// to its Unix socket. Returns true if the socket accepts connections.
func PoolProbe() bool {
	conn, err := net.DialTimeout("unix", PoolSocketPath(), 100*time.Millisecond)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// PoolExec sends an exec request to the pool daemon and returns the response.
func PoolExec(req *PoolRequest) (*PoolResponse, error) {
	return poolRPC(req)
}

// PoolCheckout requests a warm VM from the pool daemon and transfers ownership
// to the caller. The caller must clean up via DestroyCheckedOutVM when done.
func PoolCheckout() (*CheckoutInfo, error) {
	resp, err := poolRPC(&PoolRequest{Type: "checkout"})
	if err != nil {
		return nil, err
	}
	if resp.Type == "error" {
		return nil, fmt.Errorf("pool checkout: %s", resp.Error)
	}
	if resp.Checkout == nil {
		return nil, fmt.Errorf("pool checkout: no checkout info in response")
	}
	return resp.Checkout, nil
}

// DestroyCheckedOutVM cleans up a VM obtained via PoolCheckout. It kills the
// Firecracker process and removes the instance directory. Safe to call from
// any process — does not require a *firecracker.Machine handle.
func DestroyCheckedOutVM(info *CheckoutInfo) {
	if info == nil {
		return
	}
	if info.PID > 0 {
		proc, err := os.FindProcess(info.PID)
		if err == nil {
			proc.Signal(syscall.SIGKILL)
			// Poll for process death — can't Wait() since it's not our child.
			for i := 0; i < 50; i++ {
				if proc.Signal(syscall.Signal(0)) != nil {
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
		}
	}
	if info.InstanceDir != "" {
		os.RemoveAll(info.InstanceDir)
	}
}

// PoolCommand sends a control command (status/stop/scale) to the pool daemon.
func PoolCommand(req *PoolRequest) (*PoolResponse, error) {
	return poolRPC(req)
}

// poolRPC sends a request to the pool daemon over the Unix socket and reads
// the response. Uses newline-delimited JSON (same pattern as vsock protocol).
func poolRPC(req *PoolRequest) (*PoolResponse, error) {
	conn, err := net.DialTimeout("unix", PoolSocketPath(), 2*time.Second)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrPoolNotRunning, err)
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(5 * time.Minute))

	reqBytes, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}
	reqBytes = append(reqBytes, '\n')
	if _, err := conn.Write(reqBytes); err != nil {
		return nil, fmt.Errorf("sending request: %w", err)
	}

	reader := bufio.NewReader(conn)
	respLine, err := reader.ReadBytes('\n')
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	var resp PoolResponse
	if err := json.Unmarshal(respLine, &resp); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	return &resp, nil
}
