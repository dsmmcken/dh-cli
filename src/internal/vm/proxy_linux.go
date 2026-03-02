//go:build linux

package vm

import (
	"fmt"
	"io"
	"net"
	"sync"
)

// HTTPProxy forwards TCP connections from the host to the Deephaven web UI
// inside a VM via vsock. Each accepted TCP connection is bridged to a new
// vsock stream on HTTPProxyPort, where the in-VM Python proxy forwards
// bytes to the Deephaven server on localhost:10000.
type HTTPProxy struct {
	listener  net.Listener
	vsockPath string
	done      chan struct{}
	wg        sync.WaitGroup
	stderr    io.Writer // optional, for verbose logging

	mu    sync.Mutex
	conns map[net.Conn]struct{} // active connections, closed on shutdown
}

// StartHTTPProxy starts a TCP listener on listenAddr and proxies connections
// to the VM's HTTP proxy vsock port. Returns the proxy (which is also an
// io.Closer) or an error if the listener cannot bind.
// If stderr is non-nil, connection errors are logged there.
func StartHTTPProxy(listenAddr, vsockPath string, stderr io.Writer) (*HTTPProxy, error) {
	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return nil, err
	}

	p := &HTTPProxy{
		listener:  ln,
		vsockPath: vsockPath,
		done:      make(chan struct{}),
		stderr:    stderr,
		conns:     make(map[net.Conn]struct{}),
	}

	p.wg.Add(1)
	go p.acceptLoop()

	return p, nil
}

// Addr returns the listener's address (useful when port 0 is used).
func (p *HTTPProxy) Addr() net.Addr {
	return p.listener.Addr()
}

func (p *HTTPProxy) acceptLoop() {
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
			p.handleConn(conn)
		}()
	}
}

func (p *HTTPProxy) trackConn(c net.Conn) {
	p.mu.Lock()
	p.conns[c] = struct{}{}
	p.mu.Unlock()
}

func (p *HTTPProxy) untrackConn(c net.Conn) {
	p.mu.Lock()
	delete(p.conns, c)
	p.mu.Unlock()
}

func (p *HTTPProxy) handleConn(tcpConn net.Conn) {
	p.trackConn(tcpConn)
	defer func() {
		p.untrackConn(tcpConn)
		tcpConn.Close()
	}()

	vsockConn, err := connectVsock(p.vsockPath, HTTPProxyPort)
	if err != nil {
		if p.stderr != nil {
			fmt.Fprintf(p.stderr, "HTTP proxy: vsock connect failed: %v\n", err)
		}
		return
	}
	p.trackConn(vsockConn)
	defer func() {
		p.untrackConn(vsockConn)
		vsockConn.Close()
	}()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		io.Copy(vsockConn, tcpConn)
	}()
	go func() {
		defer wg.Done()
		io.Copy(tcpConn, vsockConn)
	}()
	wg.Wait()
}

// Close stops the proxy listener, force-closes all active connections, and
// waits for handler goroutines to exit.
func (p *HTTPProxy) Close() error {
	close(p.done)
	err := p.listener.Close()
	// Force-close all active connections so io.Copy goroutines unblock.
	p.mu.Lock()
	for c := range p.conns {
		c.Close()
	}
	p.mu.Unlock()
	p.wg.Wait()
	return err
}
