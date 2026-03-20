#!/usr/bin/env node
/**
 * dh-render - Interactive CLI for testing Deephaven UI components.
 *
 * Usage:
 *   dh-render open http://localhost:10000
 *   dh-render render counter_widget
 *   dh-render snapshot
 *   dh-render click "Increment"
 *   dh-render fill "Message" "Hello"
 *   dh-render select "Species" "setosa"
 *   dh-render tables
 *   dh-render close
 */
import { fork } from 'node:child_process';
import { existsSync, readFileSync, writeFileSync, unlinkSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { socketPath, sendCommand, isDaemonRunning } from '../src/cli/client.mjs';

const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);
const DAEMON_SCRIPT = join(__dirname, '..', 'src', 'cli', 'daemon.mjs');
const STATE_FILE = '/tmp/dh-render-state.json';

// ── Parse args ──
const args = process.argv.slice(2);
const command = args[0];

if (!command || command === '--help' || command === '-h') {
    printHelp();
    process.exit(0);
}

// ── Main ──
try {
    await run(command, args.slice(1));
} catch (e) {
    console.error(`Error: ${e.message}`);
    process.exit(1);
}

async function run(command, args) {
    switch (command) {
        case 'open': return handleOpen(args);
        case 'render': return handleRender(args);
        case 'snapshot': return handleSnapshot(args);
        case 'click': return handleClick(args);
        case 'fill': return handleFill(args);
        case 'select': return handleSelect(args);
        case 'options': return handleOptions(args);
        case 'tables': return handleTables(args);
        case 'table': return handleTable(args);
        case 'html': return handleSimple({ cmd: 'html' }, r => r.html);
        case 'document': return handleSimple({ cmd: 'document' }, r => r.tree);
        case 'callables': return handleSimple({ cmd: 'callables' }, r => formatCallables(r.callables));
        case 'call': return handleCall(args);
        case 'wait': return handleWait(args);
        case 'status': return handleSimple({ cmd: 'status' }, formatStatus);
        case 'close': return handleClose();
        default:
            console.error(`Unknown command: ${command}`);
            printHelp();
            process.exit(1);
    }
}

// ── Command handlers ──

async function handleOpen(args) {
    const url = args[0];
    if (!url) {
        console.error('Usage: dh-render open <url>');
        process.exit(1);
    }

    const sockPath = socketPath(url);

    // Check if daemon already running
    if (await isDaemonRunning(sockPath)) {
        // Verify the session is actually connected
        try {
            const status = await sendCommand(sockPath, { cmd: 'status' }, 5000);
            if (status.ok && status.connected) {
                saveState({ url, sockPath });
                console.log(`Already connected to ${url}`);
                return;
            }
        } catch (e) {
            // Daemon is stale — kill it and start fresh
        }
        // Daemon exists but session isn't connected — close and restart
        try { await sendCommand(sockPath, { cmd: 'close' }, 3000); } catch (e) {}
        await new Promise(r => setTimeout(r, 500));
    }

    // Start daemon
    console.log(`Starting daemon...`);
    await startDaemon(sockPath);

    // Send open command
    const result = await sendCommand(sockPath, { cmd: 'open', url });

    if (result.ok) {
        saveState({ url, sockPath });
        console.log(result.message);
    } else {
        console.error(`Failed: ${result.error}`);
        // Don't save state on failure — session isn't connected
        process.exit(1);
    }
}

async function handleRender(args) {
    const widgetName = args[0];
    if (!widgetName) {
        console.error('Usage: dh-render render <widget> [--type <type>]');
        process.exit(1);
    }

    const typeIdx = args.indexOf('--type');
    const widgetType = typeIdx >= 0 ? args[typeIdx + 1] : undefined;
    const timeoutIdx = args.indexOf('--timeout');
    const timeout = timeoutIdx >= 0 ? parseInt(args[timeoutIdx + 1], 10) : 15000;

    const sockPath = getActiveSock();
    const result = await sendCommand(sockPath, {
        cmd: 'render',
        widget: widgetName,
        type: widgetType,
        timeout,
    });

    if (result.ok) {
        console.log(`Rendered "${widgetName}" (${result.elementCount} elements, ${result.exportedObjectCount} objects, ${result.callableCount} callables)\n`);
        console.log(result.snapshot);
    } else {
        console.error(`Render failed: ${result.error}`);
        process.exit(1);
    }
}

async function handleSnapshot() {
    const sockPath = getActiveSock();
    const result = await sendCommand(sockPath, { cmd: 'snapshot' });

    if (result.ok) {
        console.log(result.snapshot);
    } else {
        console.error(result.error);
        process.exit(1);
    }
}

async function handleClick(args) {
    const target = args.join(' ');
    if (!target) {
        console.error('Usage: dh-render click <text-or-@ref>');
        process.exit(1);
    }

    const sockPath = getActiveSock();
    const result = await sendCommand(sockPath, { cmd: 'click', target });

    if (result.ok) {
        console.log(result.message);
    } else {
        console.error(result.error);
        process.exit(1);
    }
}

async function handleFill(args) {
    if (args.length < 2) {
        console.error('Usage: dh-render fill <label-or-@ref> <value>');
        process.exit(1);
    }

    const target = args[0];
    const value = args.slice(1).join(' ');
    const sockPath = getActiveSock();
    const result = await sendCommand(sockPath, { cmd: 'fill', target, value });

    if (result.ok) {
        console.log(result.message);
    } else {
        console.error(result.error);
        process.exit(1);
    }
}

async function handleSelect(args) {
    if (args.length < 2) {
        console.error('Usage: dh-render select <label-or-@ref> <value>');
        process.exit(1);
    }

    const target = args[0];
    const value = args.slice(1).join(' ');
    const sockPath = getActiveSock();
    const result = await sendCommand(sockPath, { cmd: 'select', target, value });

    if (result.ok) {
        console.log(result.message);
    } else {
        console.error(result.error);
        process.exit(1);
    }
}

async function handleOptions(args) {
    if (args.length < 1) {
        console.error('Usage: dh-render options <label-or-@ref> [--offset N] [--limit N]');
        process.exit(1);
    }

    // Parse target (everything before --flags)
    const flagIdx = args.findIndex(a => a.startsWith('--'));
    const target = flagIdx >= 0 ? args.slice(0, flagIdx).join(' ') : args.join(' ');

    const offsetIdx = args.indexOf('--offset');
    const offset = offsetIdx >= 0 ? parseInt(args[offsetIdx + 1], 10) : 0;
    const limitIdx = args.indexOf('--limit');
    const limit = limitIdx >= 0 ? parseInt(args[limitIdx + 1], 10) : 20;

    const sockPath = getActiveSock();
    const result = await sendCommand(sockPath, { cmd: 'options', target, offset, limit });

    if (result.ok) {
        const end = Math.min(offset + result.options.length, result.total);
        if (result.total <= result.limit) {
            console.log(`Options for "${result.label}" (${result.total} items):`);
        } else {
            console.log(`Options for "${result.label}" (showing ${offset + 1}-${end} of ${result.total}):`);
        }
        for (const opt of result.options) {
            console.log(`  ${opt}`);
        }
        if (end < result.total) {
            console.log(`  (use --offset ${end} to see more)`);
        }
    } else {
        console.error(result.error);
        process.exit(1);
    }
}

async function handleTables(args) {
    const rowsIdx = args.indexOf('--rows');
    const rows = rowsIdx >= 0 ? parseInt(args[rowsIdx + 1], 10) : 3;

    const sockPath = getActiveSock();
    const result = await sendCommand(sockPath, { cmd: 'tables', rows });

    if (result.ok) {
        if (result.tables.length === 0) {
            console.log('No exported tables.');
            return;
        }
        for (const t of result.tables) {
            console.log(`\n#${t.id} [Table] ${t.columns.length} columns, ${t.rowCount ?? '?'} rows`);
            console.log(`  Columns: ${t.columns.map(c => `${c.name} (${c.type.split('.').pop()})`).join(', ')}`);
            if (t.error) {
                console.log(`  Error: ${t.error}`);
            } else if (t.sampleRows.length > 0) {
                console.log(`  Sample data:`);
                for (const row of t.sampleRows.slice(0, 3)) {
                    const cols = Object.entries(row)
                        .filter(([_, v]) => typeof v !== 'object')
                        .map(([k, v]) => `${k}=${typeof v === 'number' ? v.toFixed(3) : v}`)
                        .join(', ');
                    console.log(`    { ${cols} }`);
                }
            }
        }
    } else {
        console.error(result.error);
        process.exit(1);
    }
}

async function handleTable(args) {
    const id = parseInt(args[0], 10);
    if (isNaN(id)) {
        console.error('Usage: dh-render table <objectId> [--rows N]');
        process.exit(1);
    }

    const rowsIdx = args.indexOf('--rows');
    const rows = rowsIdx >= 0 ? parseInt(args[rowsIdx + 1], 10) : 20;

    const sockPath = getActiveSock();
    const result = await sendCommand(sockPath, { cmd: 'table', id, rows });

    if (result.ok) {
        console.log(`Table #${id}: ${result.columns.length} columns, ${result.rowCount} rows`);
        console.log(`Columns: ${result.columns.map(c => `${c.name} (${c.type.split('.').pop()})`).join(', ')}`);
        if (result.rows.length > 0) {
            console.log(`\nData (${result.rows.length} rows):`);
            // Simple table output
            const colNames = result.columns.map(c => c.name).filter(n =>
                result.rows.some(r => typeof r[n] !== 'object')
            );
            console.log('  ' + colNames.join('\t'));
            for (const row of result.rows) {
                const vals = colNames.map(n => {
                    const v = row[n];
                    if (typeof v === 'number') return v.toFixed(3);
                    return String(v ?? '');
                });
                console.log('  ' + vals.join('\t'));
            }
        }
    } else {
        console.error(result.error);
        process.exit(1);
    }
}

async function handleCall(args) {
    const callableId = args[0];
    if (!callableId) {
        console.error('Usage: dh-render call <callableId> [args...]');
        process.exit(1);
    }

    let callArgs;
    try {
        callArgs = args.slice(1).map(a => JSON.parse(a));
    } catch {
        callArgs = args.slice(1);
    }

    const sockPath = getActiveSock();
    const result = await sendCommand(sockPath, { cmd: 'call', callableId, args: callArgs });

    if (result.ok) {
        console.log(result.message);
    } else {
        console.error(result.error);
        process.exit(1);
    }
}

async function handleWait(args) {
    const timeoutIdx = args.indexOf('--timeout');
    const timeout = timeoutIdx >= 0 ? parseInt(args[timeoutIdx + 1], 10) : 5000;

    const sockPath = getActiveSock();
    const result = await sendCommand(sockPath, { cmd: 'wait', timeout });

    if (result.ok) {
        console.log(result.message);
    } else {
        console.error(result.error);
        process.exit(1);
    }
}

async function handleClose() {
    const sockPath = getActiveSock();
    try {
        const result = await sendCommand(sockPath, { cmd: 'close' }, 5000);
        console.log(result.message || 'Session closed');
    } catch (e) {
        // Daemon may have exited before responding
        console.log('Session closed');
    }
    try { unlinkSync(STATE_FILE); } catch (e) {}
}

async function handleSimple(cmd, formatter) {
    const sockPath = getActiveSock();
    const result = await sendCommand(sockPath, cmd);

    if (result.ok) {
        const output = typeof formatter === 'function' ? formatter(result) : JSON.stringify(result, null, 2);
        console.log(output);
    } else {
        console.error(result.error);
        process.exit(1);
    }
}

// ── Helpers ──

function getActiveSock() {
    // Try state file first
    if (existsSync(STATE_FILE)) {
        const state = JSON.parse(readFileSync(STATE_FILE, 'utf8'));
        if (state.sockPath && existsSync(state.sockPath)) {
            return state.sockPath;
        }
    }

    // Try active symlink
    const activePath = '/tmp/dh-render-active.sock';
    if (existsSync(activePath)) return activePath;

    console.error('No active session. Use "dh-render open <url>" first.');
    process.exit(1);
}

function saveState(state) {
    writeFileSync(STATE_FILE, JSON.stringify(state), 'utf8');
}

function startDaemon(sockPath) {
    return new Promise((resolve, reject) => {
        const cssLoader = join(__dirname, '..', 'src', 'css-loader.mjs');
        const child = fork(DAEMON_SCRIPT, [sockPath], {
            detached: true,
            stdio: ['ignore', 'ignore', 'ignore', 'ipc'],
            execArgv: ['--import', cssLoader],
        });

        const timer = setTimeout(() => {
            reject(new Error('Daemon failed to start within 15s'));
        }, 15000);

        child.on('message', (msg) => {
            if (msg.ready) {
                clearTimeout(timer);
                child.unref();
                child.disconnect();
                resolve();
            }
        });

        child.on('error', (e) => {
            clearTimeout(timer);
            reject(new Error(`Failed to start daemon: ${e.message}`));
        });

        child.on('exit', (code) => {
            clearTimeout(timer);
            if (code !== 0) {
                reject(new Error(`Daemon exited with code ${code}`));
            }
        });
    });
}

function formatCallables(callables) {
    if (!callables || callables.length === 0) return 'No callables.';
    return callables.map(cb => `${cb.id}  ${cb.path}  (parent: ${cb.parentElement || 'root'})`).join('\n');
}

function formatStatus(result) {
    const lines = [];
    lines.push(`Connected: ${result.connected}`);
    if (result.serverUrl) lines.push(`Server: ${result.serverUrl}`);
    if (result.widget) lines.push(`Widget: ${result.widget} (${result.widgetType})`);
    lines.push(`Rendered: ${result.hasRender}`);
    lines.push(`Exported objects: ${result.exportedObjects}`);
    return lines.join('\n');
}

function printHelp() {
    console.log(`dh-render - Test Deephaven UI components interactively

Usage: dh-render <command> [args]

Connection:
  open <url>                    Connect to Deephaven server
  close                         Disconnect and stop daemon
  status                        Show connection status

Rendering:
  render <widget> [--type <t>]  Render a widget (type auto-detected)
  snapshot                      Show accessibility tree with @refs
  html                          Show rendered HTML
  document                      Show DH document tree

Interaction:
  click <text-or-@ref>          Click a button
  fill <label-or-@ref> <value>  Fill a text field
  select <label-or-@ref> <val>  Select a picker value
  options <label-or-@ref>       List picker options (paginated)
    [--offset N] [--limit N]    Pagination (default: 20 per page)

Data:
  tables [--rows N]             List exported tables with sample data
  table <id> [--rows N]         Fetch a specific table

Advanced:
  callables                     List all server-side callables
  call <id> [args...]           Call a callable directly
  wait [--timeout ms]           Wait for next document update

Examples:
  dh-render open http://localhost:10000
  dh-render render counter_widget
  dh-render snapshot
  dh-render click "Increment"
  dh-render fill "Message" "Hello"
  dh-render select "Current Species" "setosa"
  dh-render options "Current Species"
  dh-render snapshot
  dh-render close`);
}
