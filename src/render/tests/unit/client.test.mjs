import { describe, it, expect, vi, beforeEach, afterEach, afterAll } from 'vitest';
import { socketPath, sendCommand, isDaemonRunning } from '../../src/cli/client.mjs';
import { createServer } from 'node:net';
import { unlinkSync, existsSync } from 'node:fs';

// Track all mock servers/sockets for cleanup
const cleanupPaths = [];
const cleanupServers = [];

/**
 * Create a mock daemon that listens on a Unix socket and responds via a handler.
 */
function createMockDaemon(handler) {
    const path = `/tmp/dh-render-test-${Date.now()}-${Math.random().toString(36).slice(2, 8)}.sock`;
    const server = createServer((socket) => {
        let data = '';
        socket.on('data', (chunk) => {
            data += chunk.toString();
            if (data.includes('\n')) {
                const cmd = JSON.parse(data.trim());
                const response = handler(cmd);
                if (response !== undefined) {
                    socket.write(JSON.stringify(response) + '\n');
                }
                // If response is undefined, handler chose not to respond (for timeout tests)
            }
        });
    });
    server.listen(path);
    cleanupPaths.push(path);
    cleanupServers.push(server);
    return { path, server, close: () => new Promise(r => server.close(r)) };
}

afterEach(() => {
    // Clean up any servers created during the test
});

afterAll(async () => {
    // Close all servers
    for (const server of cleanupServers) {
        try {
            server.close();
        } catch (_) { /* ignore */ }
    }
    cleanupServers.length = 0;

    // Remove socket files
    for (const p of cleanupPaths) {
        try {
            if (existsSync(p)) unlinkSync(p);
        } catch (_) { /* ignore */ }
    }
    cleanupPaths.length = 0;
});

describe('socketPath', () => {
    it('returns null when no serverUrl and no active session file', () => {
        // Ensure the active session file does not exist for this test
        const activePath = '/tmp/dh-render-active.sock';
        const activeExists = existsSync(activePath);
        // If it does exist, we cannot guarantee the result, so skip assertion on that case.
        if (!activeExists) {
            const result = socketPath(undefined);
            expect(result).toBeNull();
        } else {
            // Active session file exists, so it should return that path
            const result = socketPath(undefined);
            expect(result).toBe(activePath);
        }
    });

    it('returns a path with hash for a URL', () => {
        const result = socketPath('http://localhost:10000');
        expect(result).toMatch(/^\/tmp\/dh-render-[0-9a-f]{8}\.sock$/);
    });

    it('different URLs produce different paths', () => {
        const path1 = socketPath('http://localhost:10000');
        const path2 = socketPath('http://localhost:10001');
        expect(path1).not.toBe(path2);
    });

    it('same URL produces the same path consistently', () => {
        const url = 'http://localhost:9999';
        const path1 = socketPath(url);
        const path2 = socketPath(url);
        expect(path1).toBe(path2);
    });

    it('path starts with /tmp/dh-render-', () => {
        const result = socketPath('http://example.com:5555');
        expect(result.startsWith('/tmp/dh-render-')).toBe(true);
    });
});

describe('sendCommand', () => {
    it('sends a command and receives a response', async () => {
        const mock = createMockDaemon((cmd) => {
            return { ok: true, echo: cmd.cmd };
        });

        const response = await sendCommand(mock.path, { cmd: 'ping' });
        expect(response).toEqual({ ok: true, echo: 'ping' });
        await mock.close();
    });

    it('handles different commands', async () => {
        const mock = createMockDaemon((cmd) => {
            if (cmd.cmd === 'status') return { ok: true, status: 'running' };
            if (cmd.cmd === 'render') return { ok: true, widget: cmd.name };
            return { ok: false, error: 'unknown' };
        });

        const statusResp = await sendCommand(mock.path, { cmd: 'status' });
        expect(statusResp).toEqual({ ok: true, status: 'running' });

        const renderResp = await sendCommand(mock.path, { cmd: 'render', name: 'my_widget' });
        expect(renderResp).toEqual({ ok: true, widget: 'my_widget' });

        await mock.close();
    });

    it('rejects on timeout when daemon does not respond', async () => {
        // Handler intentionally never responds
        const mock = createMockDaemon(() => undefined);

        await expect(sendCommand(mock.path, { cmd: 'slow' }, 100))
            .rejects.toThrow(/Timeout/);

        await mock.close();
    });

    it('rejects when socket does not exist', async () => {
        const fakePath = '/tmp/dh-render-nonexistent-99999.sock';
        await expect(sendCommand(fakePath, { cmd: 'ping' }))
            .rejects.toThrow();
    });
});

describe('isDaemonRunning', () => {
    it('returns false for a non-existent socket path', async () => {
        const result = await isDaemonRunning('/tmp/dh-render-does-not-exist-12345.sock');
        expect(result).toBe(false);
    });

    it('returns false for null socket path', async () => {
        const result = await isDaemonRunning(null);
        expect(result).toBe(false);
    });

    it('returns true for a running daemon that responds to ping', async () => {
        const mock = createMockDaemon((cmd) => {
            if (cmd.cmd === 'ping') return { ok: true, pong: true };
            return { ok: true };
        });

        const result = await isDaemonRunning(mock.path);
        expect(result).toBe(true);
        await mock.close();
    });
});
