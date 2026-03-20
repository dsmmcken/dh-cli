#!/usr/bin/env node
/**
 * Persistent render daemon for Firecracker VM.
 *
 * Pre-loads all Node.js modules and Deephaven JSAPI at startup, then listens
 * on a Unix socket for render requests. This process is captured in the VM
 * snapshot, eliminating the ~14s cold start (5s module loading + 9s JSAPI
 * download) on every `dh render --vm` invocation.
 *
 * Protocol (one request per connection, JSON lines):
 *   Request:  {"widget":"name", "actions":["snapshot"], "timeout":15000, "rows":10, "json":false}
 *   Response: {"exit_code":0, "stdout":"", "stderr":"...", "render_output":"..."}
 */

// ── Suppress noisy warnings (same filters as oneshot.mjs) ──
const SUPPRESSED = [
    'was not wrapped in act',
    'visible label',
    'aria-label',
    'Dashboard widget has changed',
    'Retrying url',
];
const _origWarn = console.warn;
const _origError = console.error;
const _origLog = console.log;
function isSuppressed(args) {
    return SUPPRESSED.some(s => String(args[0] ?? '').includes(s));
}
console.log = (...a) => { if (!isSuppressed(a)) _origLog.apply(console, a); };
console.warn = (...a) => { if (!isSuppressed(a)) _origWarn.apply(console, a); };
console.error = (...a) => { if (!isSuppressed(a)) _origError.apply(console, a); };

import net from 'node:net';
import fs from 'node:fs';
import { createTestClient } from '../src/index.mjs';
import { DaemonSession } from '../src/cli/session.mjs';

const SOCKET_PATH = '/tmp/render-daemon.sock';
const READY_PATH = '/tmp/render_daemon_ready';
const DH_URL = 'http://127.0.0.1:10000';

// ── Phase 1: Pre-load all modules and JSAPI ──
// The expensive part of createTestClient is module loading (~5s) and JSAPI
// download (~9s). By calling it once at boot, all npm modules are cached in
// Node.js's module system and JSAPI files are cached on disk. Subsequent
// createTestClient calls reuse these caches and only pay ~200ms for a fresh
// WebSocket connection.
const _tBoot = Date.now();
try {
    const warmClient = await createTestClient(DH_URL);
    warmClient.close(); // don't keep the boot-time connection
} catch (e) {
    process.stderr.write(`[render-daemon] FATAL: ${e.message}\n`);
    process.exit(1);
}
process.stderr.write(`[render-daemon] Pre-loaded in ${Date.now() - _tBoot}ms\n`);

// ── Action parser (mirrors oneshot.mjs argv parsing) ──
function parseActions(argv) {
    if (!argv || argv.length === 0) return [{ type: 'snapshot' }];

    const actions = [];
    let i = 0;
    while (i < argv.length) {
        const cmd = argv[i];
        switch (cmd) {
            case 'click':
                actions.push({ type: 'click', target: argv[++i] });
                break;
            case 'fill':
                actions.push({ type: 'fill', target: argv[++i], value: argv[++i] });
                break;
            case 'select':
                actions.push({ type: 'select', target: argv[++i], value: argv[++i] });
                break;
            case 'table':
                actions.push({ type: 'table', id: argv[++i] });
                break;
            case 'wait':
                actions.push({ type: 'wait', ms: parseInt(argv[++i], 10) });
                break;
            case 'snapshot':
            case 'tables':
            case 'html':
                actions.push({ type: cmd });
                break;
            default:
                // Unknown action — pass through as-is, will error at execution
                actions.push({ type: cmd });
        }
        i++;
    }
    return actions;
}

// ── Output formatter (mirrors oneshot.mjs output logic) ──
function formatOutput(data, jsonMode) {
    if (jsonMode) return JSON.stringify(data);
    if (data.snapshot) return data.snapshot;
    if (data.html) return data.html;
    if (data.tables) {
        const lines = [];
        for (const t of data.tables) {
            lines.push(`\n${t.id}: ${t.columns.map(c => c.name).join(', ')} (${t.rowCount} rows)`);
            if (t.sampleRows?.length) {
                for (const row of t.sampleRows) {
                    lines.push('  ' + JSON.stringify(row));
                }
            }
        }
        return lines.join('\n');
    }
    if (data.columns) {
        const lines = [`Columns: ${data.columns.map(c => c.name).join(', ')} (${data.rowCount} rows)`];
        for (const row of data.rows) {
            lines.push('  ' + JSON.stringify(row));
        }
        return lines.join('\n');
    }
    if (data.message) return data.message;
    return '';
}

// ── Request handler ──
async function handleRender(request) {
    const {
        widget, widget_type: widgetType,
        actions: actionArgv = [],
        timeout = 15000, rows = 10, json: jsonMode = false,
    } = request;

    const stderrParts = [];
    const outputParts = [];
    const _t0 = Date.now();

    // Use DaemonSession.open() — identical to the oneshot.mjs flow.
    // All npm modules and JSAPI files are cached from the boot-time load,
    // so this only pays ~200ms for a fresh WebSocket (not 14s cold start).
    const _tConnect = Date.now();
    const session = new DaemonSession();
    const openResult = await session.open(DH_URL);
    if (!openResult.ok) {
        return {
            exit_code: 1, stdout: '', stderr: '',
            error: `Failed to connect: ${openResult.error}`,
            render_output: '',
        };
    }
    stderrParts.push(`[timing] connect: ${Date.now() - _tConnect}ms`);

    try {
        const renderResult = await session.render(widget, widgetType || 'deephaven.ui.Element', timeout);
        if (!renderResult.ok) {
            return {
                exit_code: 1, stdout: '', stderr: '',
                error: `Render failed: ${renderResult.error}`,
                render_output: '',
            };
        }
        const _t1 = Date.now();
        stderrParts.push(`[timing] session.render: ${_t1 - _t0}ms`);

        // Parse and execute the action pipeline
        const actions = parseActions(actionArgv);
        for (const action of actions) {
            let result;
            switch (action.type) {
                case 'snapshot':
                    result = session.snapshot();
                    break;
                case 'click':
                    result = await session.click(action.target);
                    if (result.ok) {
                        outputParts.push(formatOutput({ message: result.message }, jsonMode));
                        result = session.snapshot();
                    }
                    break;
                case 'fill':
                    result = await session.fill(action.target, action.value);
                    if (result.ok) {
                        outputParts.push(formatOutput({ message: result.message }, jsonMode));
                        result = session.snapshot();
                    }
                    break;
                case 'select':
                    result = await session.select(action.target, action.value);
                    if (result.ok) {
                        outputParts.push(formatOutput({ message: result.message }, jsonMode));
                        result = session.snapshot();
                    }
                    break;
                case 'tables':
                    result = await session.tables(rows);
                    break;
                case 'table':
                    result = await session.table(action.id, rows);
                    break;
                case 'html':
                    result = session.html();
                    break;
                case 'wait':
                    result = await session.wait(action.ms || 5000);
                    break;
                default:
                    result = { ok: false, error: `Unknown action: ${action.type}` };
            }

            if (!result.ok) {
                return {
                    exit_code: 1, stdout: '',
                    stderr: stderrParts.join('\n') + '\n',
                    error: `Error (${action.type}): ${result.error}`,
                    render_output: outputParts.join('\n'),
                };
            }

            const out = formatOutput(result, jsonMode);
            if (out) outputParts.push(out);
        }

        const _t2 = Date.now();
        stderrParts.push(`[timing] total daemon render: ${_t2 - _t0}ms`);

        return {
            exit_code: 0, stdout: '',
            stderr: stderrParts.join('\n') + '\n',
            error: null,
            render_output: outputParts.join('\n') + (outputParts.length ? '\n' : ''),
        };
    } catch (e) {
        return {
            exit_code: 1, stdout: '',
            stderr: stderrParts.join('\n') + '\n',
            error: `Daemon error: ${e.message || e}`,
            render_output: '',
        };
    } finally {
        session.close();
    }
}

// ── Unix socket server ──
// One request per connection, serial processing. This matches the VM's
// one-render-at-a-time model (vsock serves one request per connection too).
try { fs.unlinkSync(SOCKET_PATH); } catch (_) {}

const server = net.createServer((conn) => {
    let buf = '';
    conn.on('data', (chunk) => {
        buf += chunk.toString();
        const nlIdx = buf.indexOf('\n');
        if (nlIdx >= 0) {
            const line = buf.slice(0, nlIdx);
            buf = '';
            processRequest(conn, line);
        }
    });
    conn.on('error', () => {});
});

async function processRequest(conn, line) {
    let request;
    try {
        request = JSON.parse(line);
    } catch (e) {
        conn.end(JSON.stringify({ exit_code: 1, error: `Bad JSON: ${e.message}` }) + '\n');
        return;
    }

    try {
        const response = await handleRender(request);
        conn.end(JSON.stringify(response) + '\n');
    } catch (e) {
        conn.end(JSON.stringify({
            exit_code: 1, error: `Unhandled: ${e.message || e}`,
        }) + '\n');
    }
}

server.listen(SOCKET_PATH, () => {
    fs.writeFileSync(READY_PATH, '');
    process.stderr.write(`[render-daemon] Listening on ${SOCKET_PATH}\n`);
});

// Keep alive — the event loop stays open via the server.listen().
// Handle signals gracefully for clean shutdown.
process.on('SIGTERM', () => process.exit(0));
process.on('SIGINT', () => process.exit(0));
// Prevent unhandled rejections from crashing the daemon
process.on('unhandledRejection', (err) => {
    process.stderr.write(`[render-daemon] unhandled rejection: ${err?.message || err}\n`);
});
