/**
 * dh-render daemon - Background process that holds session state.
 *
 * Listens on a Unix domain socket for JSON commands from the CLI client.
 * Persists JSAPI, connection, widget, and rendered DOM between invocations.
 *
 * Usage (spawned by CLI):
 *   node src/cli/daemon.mjs <socket-path>
 */
import { createServer } from 'node:net';
import { unlinkSync, existsSync, writeFileSync, symlinkSync } from 'node:fs';
import { DaemonSession } from './session.mjs';

const IDLE_TIMEOUT = 5 * 60 * 1000; // 5 minutes

const socketPath = process.argv[2];
if (!socketPath) {
    console.error('Usage: daemon.mjs <socket-path>');
    process.exit(1);
}

// Clean up stale socket
if (existsSync(socketPath)) {
    unlinkSync(socketPath);
}

const session = new DaemonSession();
let idleTimer = null;

function resetIdleTimer() {
    if (idleTimer) clearTimeout(idleTimer);
    idleTimer = setTimeout(() => {
        session.close();
        cleanup();
        process.exit(0);
    }, IDLE_TIMEOUT);
}

function cleanup() {
    try { unlinkSync(socketPath); } catch (e) { /* ignore */ }
    const activePath = '/tmp/dh-render-active.sock';
    try { unlinkSync(activePath); } catch (e) { /* ignore */ }
}

// Command dispatch
async function handleCommand(cmd) {
    resetIdleTimer();

    try {
        switch (cmd.cmd) {
            case 'ping':
                return { ok: true, message: 'pong' };

            case 'open':
                return await session.open(cmd.url);

            case 'render':
                return await session.render(cmd.widget, cmd.type, cmd.timeout);

            case 'snapshot':
                return session.snapshot();

            case 'click':
                return await session.click(cmd.target);

            case 'fill':
                return await session.fill(cmd.target, cmd.value);

            case 'select':
                return await session.select(cmd.target, cmd.value);

            case 'options':
                return await session.options(cmd.target, cmd.offset || 0, cmd.limit || 20);

            case 'tables':
                return await session.tables(cmd.rows || 3);

            case 'table':
                return await session.table(cmd.id, cmd.rows || 20);

            case 'html':
                return session.html();

            case 'document':
                return session.document();

            case 'callables':
                return session.callables();

            case 'call':
                return await session.call(cmd.callableId, cmd.args || []);

            case 'wait':
                return await session.wait(cmd.timeout || 5000);

            case 'status':
                return session.status();

            case 'close':
                const result = session.close();
                // Schedule shutdown after response is sent
                setTimeout(() => {
                    cleanup();
                    process.exit(0);
                }, 100);
                return result;

            default:
                return { ok: false, error: `Unknown command: ${cmd.cmd}` };
        }
    } catch (e) {
        let msg;
        if (typeof e === 'string') {
            msg = e;
        } else if (e?.name === 'AggregateError' && e?.errors?.length) {
            // AggregateError wraps multiple failures — show them all
            const subs = e.errors.map(sub => sub?.message || String(sub)).join('; ');
            msg = `${e.message || 'Connection failed'}: ${subs}`;
        } else {
            msg = e?.message || e?.detailMessage || String(e);
        }
        return { ok: false, error: msg || 'Unknown error' };
    }
}

// Async command queue to prevent concurrent React operations
let commandQueue = Promise.resolve();

const server = createServer((socket) => {
    let buffer = '';

    socket.on('data', (chunk) => {
        buffer += chunk.toString();
        const newlineIdx = buffer.indexOf('\n');
        if (newlineIdx >= 0) {
            const line = buffer.slice(0, newlineIdx);
            buffer = buffer.slice(newlineIdx + 1);

            let cmd;
            try {
                cmd = JSON.parse(line);
            } catch (e) {
                socket.write(JSON.stringify({ ok: false, error: 'Invalid JSON' }) + '\n');
                return;
            }

            // Queue commands sequentially
            commandQueue = commandQueue.then(async () => {
                const result = await handleCommand(cmd);
                try {
                    socket.write(JSON.stringify(result) + '\n');
                } catch (e) {
                    // Socket may have closed
                }
            }).catch((err) => {
                // Prevent broken promise chain — send error response if possible
                if (process.env.DH_RENDER_DEBUG) {
                    console.error('[daemon] queue error:', err);
                }
                try {
                    socket.write(JSON.stringify({ ok: false, error: err?.message || 'Internal error' }) + '\n');
                } catch (e) { /* socket may have closed */ }
            });
        }
    });

    socket.on('error', () => { /* ignore client disconnect */ });
});

server.listen(socketPath, () => {
    // Write active session symlink
    const activePath = '/tmp/dh-render-active.sock';
    try { unlinkSync(activePath); } catch (e) {}
    try { symlinkSync(socketPath, activePath); } catch (e) {}

    // Signal to parent that daemon is ready
    if (process.send) {
        process.send({ ready: true, socketPath });
    }

    resetIdleTimer();
});

// Cleanup on exit
process.on('SIGTERM', () => { session.close(); cleanup(); process.exit(0); });
process.on('SIGINT', () => { session.close(); cleanup(); process.exit(0); });
process.on('exit', cleanup);

// Suppress unhandled rejection crashes (jsdom/GWT noise)
process.on('unhandledRejection', (reason) => {
    // Log but don't crash
    if (process.env.DH_RENDER_DEBUG) {
        console.error('[daemon] unhandled rejection:', reason);
    }
});

// Suppress uncaught exceptions from async side-effects (React/Spectrum layout
// measurements, jsdom stubs, etc.) that fire after command handlers return.
process.on('uncaughtException', (err) => {
    if (process.env.DH_RENDER_DEBUG) {
        console.error('[daemon] uncaught exception:', err);
    }
});
