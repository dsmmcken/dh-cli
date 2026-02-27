package repl

import (
	"bufio"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"
)

//go:embed repl_runner.py
var replRunnerScript string

// SessionConfig holds configuration for creating a REPL session.
type SessionConfig struct {
	Port    int
	JVMArgs string
	Version string
	Host    string

	// Remote auth
	AuthType      string
	AuthToken     string
	TLS           bool
	TLSCACert     string
	TLSClientCert string
	TLSClientKey  string

	// Resolved paths (populated by caller)
	PythonBin string
	JavaHome  string
	DHHome   string // config directory for history file
}

// Session manages the Python subprocess and JSON protocol communication.
type Session struct {
	cmd      *exec.Cmd
	stdin    io.WriteCloser
	stdout   io.ReadCloser
	mu       sync.Mutex
	pending  map[string]chan *Response
	readyCh  chan *Response
	readDone chan struct{}
	ready    *Response
	pushCh   chan *Response // channel for unsolicited server-push messages (table_update)
}

// NewSession starts the Python REPL subprocess and waits for the ready message.
func NewSession(cfg SessionConfig) (*Session, error) {
	// Build runner args
	var runnerArgs []string
	if cfg.Host != "" {
		runnerArgs = append(runnerArgs, "--mode", "remote")
		runnerArgs = append(runnerArgs, "--host", cfg.Host)
	} else {
		runnerArgs = append(runnerArgs, "--mode", "embedded")
	}
	runnerArgs = append(runnerArgs, "--port", fmt.Sprintf("%d", cfg.Port))
	if cfg.JVMArgs != "" {
		runnerArgs = append(runnerArgs, fmt.Sprintf("--jvm-args=%s", cfg.JVMArgs))
	}

	// Remote auth flags
	if cfg.Host != "" {
		if cfg.AuthType != "" {
			runnerArgs = append(runnerArgs, "--auth-type", cfg.AuthType)
		}
		if cfg.AuthToken != "" {
			runnerArgs = append(runnerArgs, "--auth-token", cfg.AuthToken)
		}
		if cfg.TLS {
			runnerArgs = append(runnerArgs, "--tls")
		}
		if cfg.TLSCACert != "" {
			runnerArgs = append(runnerArgs, "--tls-ca-cert", cfg.TLSCACert)
		}
		if cfg.TLSClientCert != "" {
			runnerArgs = append(runnerArgs, "--tls-client-cert", cfg.TLSClientCert)
		}
		if cfg.TLSClientKey != "" {
			runnerArgs = append(runnerArgs, "--tls-client-key", cfg.TLSClientKey)
		}
	}

	// Build command: python -c "<runner script>" <args...>
	cmdArgs := append([]string{"-c", replRunnerScript}, runnerArgs...)
	cmd := exec.Command(cfg.PythonBin, cmdArgs...)

	// Set environment
	cmd.Env = os.Environ()
	if cfg.Host == "" && cfg.JavaHome != "" {
		cmd.Env = append(cmd.Env, fmt.Sprintf("JAVA_HOME=%s", cfg.JavaHome))
	}

	// Process group for clean cleanup
	cmd.SysProcAttr = processGroupAttr()

	// Create pipes
	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("creating stdin pipe: %w", err)
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("creating stdout pipe: %w", err)
	}

	// Discard subprocess stderr to avoid corrupting the TUI alt screen
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting python process: %w", err)
	}

	s := &Session{
		cmd:      cmd,
		stdin:    stdinPipe,
		stdout:   stdoutPipe,
		pending:  make(map[string]chan *Response),
		readyCh:  make(chan *Response, 1),
		readDone: make(chan struct{}),
		pushCh:   make(chan *Response, 16),
	}

	// Start reader goroutine
	go s.readLoop()

	// Wait for ready message with 60-second timeout
	select {
	case resp := <-s.readyCh:
		s.ready = resp
	case <-s.readDone:
		return nil, fmt.Errorf("python process exited before sending ready message")
	case <-time.After(60 * time.Second):
		killProcessGroup(cmd.Process.Pid)
		return nil, fmt.Errorf("timed out waiting for python process to start (60s)")
	}

	return s, nil
}

func (s *Session) readLoop() {
	defer close(s.readDone)
	scanner := bufio.NewScanner(s.stdout)
	// Increase buffer for large table data responses (1MB max line)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var resp Response
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			continue
		}

		if resp.IsReady() {
			select {
			case s.readyCh <- &resp:
			default:
			}
			continue
		}

		if resp.IsShutdownAck() {
			continue
		}

		// Handle unsolicited server-push messages (no ID)
		if resp.IsTableUpdate() {
			select {
			case s.pushCh <- &resp:
			default:
				// Drop update if consumer is too slow — next poll will catch up
			}
			continue
		}

		// Dispatch to waiting caller by ID
		if resp.ID != "" {
			s.mu.Lock()
			ch, ok := s.pending[resp.ID]
			if ok {
				delete(s.pending, resp.ID)
			}
			s.mu.Unlock()
			if ok {
				ch <- &resp
			}
		}
	}
}

func (s *Session) sendAndWait(cmd Command) (*Response, error) {
	ch := make(chan *Response, 1)
	s.mu.Lock()
	s.pending[cmd.ID] = ch
	s.mu.Unlock()

	data, err := json.Marshal(cmd)
	if err != nil {
		s.mu.Lock()
		delete(s.pending, cmd.ID)
		s.mu.Unlock()
		return nil, fmt.Errorf("marshaling command: %w", err)
	}

	s.mu.Lock()
	_, err = fmt.Fprintf(s.stdin, "%s\n", data)
	s.mu.Unlock()
	if err != nil {
		s.mu.Lock()
		delete(s.pending, cmd.ID)
		s.mu.Unlock()
		return nil, fmt.Errorf("writing to stdin: %w", err)
	}

	select {
	case resp := <-ch:
		if resp.IsError() {
			return nil, fmt.Errorf("python error: %s", resp.Message)
		}
		return resp, nil
	case <-s.readDone:
		return nil, fmt.Errorf("python process exited unexpectedly")
	}
}

// Execute sends Python code and returns the result.
func (s *Session) Execute(code string) (*Response, error) {
	return s.sendAndWait(NewExecuteCmd(code))
}

// ListTables returns metadata for all tables on the server.
func (s *Session) ListTables() (*Response, error) {
	return s.sendAndWait(NewListTablesCmd())
}

// FetchTable returns paginated row data for the named table.
func (s *Session) FetchTable(name string, offset, limit int) (*Response, error) {
	return s.sendAndWait(NewFetchTableCmd(name, offset, limit))
}

// ServerInfo returns server connection details.
func (s *Session) ServerInfo() (*Response, error) {
	return s.sendAndWait(NewServerInfoCmd())
}

// Subscribe tells the Python runner to start polling a table and sending updates.
func (s *Session) Subscribe(name string, offset, limit int) (*Response, error) {
	return s.sendAndWait(NewSubscribeCmd(name, offset, limit))
}

// Unsubscribe tells the Python runner to stop polling a table.
func (s *Session) Unsubscribe(name string) (*Response, error) {
	return s.sendAndWait(NewUnsubscribeCmd(name))
}

// PushChannel returns the channel that receives unsolicited table_update messages.
func (s *Session) PushChannel() <-chan *Response {
	return s.pushCh
}

// Done returns a channel that is closed when the read loop exits.
func (s *Session) Done() <-chan struct{} {
	return s.readDone
}

// Ready returns the stored ready response (port, version, mode).
func (s *Session) Ready() *Response {
	return s.ready
}

// Close gracefully shuts down the Python subprocess.
func (s *Session) Close() {
	// Try graceful shutdown
	shutdownCmd := NewShutdownCmd()
	data, _ := json.Marshal(shutdownCmd)
	s.mu.Lock()
	fmt.Fprintf(s.stdin, "%s\n", data)
	s.mu.Unlock()
	s.stdin.Close()

	// Wait briefly for process to exit
	done := make(chan struct{})
	go func() {
		s.cmd.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Clean exit
	case <-time.After(3 * time.Second):
		// Force kill
		if s.cmd.Process != nil {
			killProcessGroup(s.cmd.Process.Pid)
		}
		<-done
	}
}
