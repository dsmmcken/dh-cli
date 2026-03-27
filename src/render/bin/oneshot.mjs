#!/usr/bin/env node
/**
 * One-shot render + action pipeline for dh-cli integration.
 *
 * No daemon, no sockets. Connects, renders, performs actions, outputs, exits.
 *
 * Usage:
 *   node --import ./src/css-loader.mjs bin/oneshot.mjs --url <url> [--widget <name>] [actions...]
 *
 * Actions (left to right):
 *   snapshot                    Print accessibility tree
 *   click <target>              Click by text or @ref, then auto-snapshot
 *   fill <target> <value>       Fill text field, then auto-snapshot
 *   select <target> <value>     Select picker option, then auto-snapshot
 *   tables                      List exported tables
 *   table <id>                  Fetch table data
 *   html                        Dump rendered HTML
 *   wait <ms>                   Pause for async effects
 *   diagnose                    Full diagnostic JSON report
 *
 * If no actions given, defaults to "snapshot".
 *
 * Flags:
 *   --url <url>          Server URL (required)
 *   --widget <name>      Widget name (optional; auto-discovers if omitted)
 *   --type <type>        Widget type (default: auto-detect)
 *   --timeout <ms>       Render timeout (default: 15000)
 *   --rows <n>           Max table rows (default: 10)
 *   --json               Output JSON instead of text
 */
// ── Suppress noisy warnings that aren't actionable ──
// React act() warnings, Spectrum aria-label warnings, WidgetHandler lifecycle logs
const _origWarn = console.warn;
const _origError = console.error;
const SUPPRESSED = [
    'was not wrapped in act',
    'visible label',
    'aria-label',
    'Dashboard widget has changed',
    'Retrying url',
];
function isSuppressed(args) {
    const msg = String(args[0] ?? '');
    return SUPPRESSED.some(s => msg.includes(s));
}
const _origLog = console.log;
console.log = (...args) => { if (!isSuppressed(args)) _origLog.apply(console, args); };
console.warn = (...args) => { if (!isSuppressed(args)) _origWarn.apply(console, args); };
console.error = (...args) => { if (!isSuppressed(args)) _origError.apply(console, args); };

import { createTestClient, diagnoseWidget } from '../src/index.mjs';
import { DaemonSession } from '../src/cli/session.mjs';

// Silence DH's own logging framework (info-level lifecycle messages like
// "Dashboard widget has changed"). Only errors and warnings pass through.
try {
    const { default: Log } = await import('@deephaven/log');
    Log.setLogLevel(1); // WARN = 1
} catch (_) {}

// ── Arg parsing ──

const argv = process.argv.slice(2);

function getFlag(name, defaultValue) {
    const idx = argv.indexOf(`--${name}`);
    if (idx < 0) return defaultValue;
    return argv[idx + 1];
}

function hasFlag(name) {
    return argv.includes(`--${name}`);
}

const url = getFlag('url', null);
const widget = getFlag('widget', null);
const widgetType = getFlag('type', undefined);
const timeout = parseInt(getFlag('timeout', '15000'), 10);
const maxRows = parseInt(getFlag('rows', '10'), 10);
const jsonOutput = hasFlag('json');
const verbose = hasFlag('verbose');

if (!url) {
    console.error('Usage: oneshot.mjs --url <url> [--widget <name>] [actions...]');
    console.error('  --url is required. --widget is optional (auto-discovers if omitted).');
    process.exit(1);
}

// Parse actions from argv (skip flags)
const FLAG_NAMES = new Set(['url', 'widget', 'type', 'timeout', 'rows']);
const actions = [];
let i = 0;
while (i < argv.length) {
    if (argv[i].startsWith('--')) {
        const name = argv[i].slice(2);
        if (FLAG_NAMES.has(name)) {
            i += 2; // skip flag + value
        } else {
            i += 1; // boolean flag (--json)
        }
        continue;
    }

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
        case 'diagnose':
            actions.push({ type: cmd });
            break;
        default:
            console.error(`Unknown action: ${cmd}`);
            process.exit(1);
    }
    i++;
}

// Default to snapshot if no actions given
if (actions.length === 0) {
    actions.push({ type: 'snapshot' });
}

// ── Diagnose shortcut ──

if (actions.length === 1 && actions[0].type === 'diagnose') {
    try {
        const report = await diagnoseWidget(url, widget, { widgetType, timeout });
        console.log(JSON.stringify(report, null, 2));
        process.exit(report.status === 'ok' ? 0 : 1);
    } catch (e) {
        console.error(`Error: ${e.message || e}`);
        process.exit(1);
    }
}

// ── Main pipeline ──

const session = new DaemonSession();

function output(data) {
    if (jsonOutput) {
        console.log(JSON.stringify(data));
    } else if (data.snapshot) {
        console.log(data.snapshot);
    } else if (data.html) {
        console.log(data.html);
    } else if (data.tables) {
        for (const t of data.tables) {
            console.log(`\n${t.id}: ${t.columns.map(c => c.name).join(', ')} (${t.rowCount} rows)`);
            if (t.sampleRows?.length) {
                for (const row of t.sampleRows) {
                    console.log('  ' + JSON.stringify(row));
                }
            }
        }
    } else if (data.columns) {
        // Single table
        console.log(`Columns: ${data.columns.map(c => c.name).join(', ')} (${data.rowCount} rows)`);
        for (const row of data.rows) {
            console.log('  ' + JSON.stringify(row));
        }
    } else if (data.message) {
        console.log(data.message);
    }
}

// Detect illustrated_message errors in snapshot text (React error boundary UI).
// These render successfully (DOM exists) but indicate a widget error.
function snapshotHasError(text) {
    return /^\s*\[illustrated_message\] icon="warning"/m.test(text);
}

let hasIllustratedError = false;

try {
    const _t0 = Date.now();
    // Connect and render
    const openResult = await session.open(url);
    if (!openResult.ok) {
        console.error(`Connection failed: ${openResult.error}`);
        process.exit(1);
    }
    const _t1 = Date.now();
    if (verbose) console.error(`[timing] session.open (JSAPI load + connect): ${_t1 - _t0}ms`);

    const renderResult = await session.render(widget, widgetType, timeout);
    if (!renderResult.ok) {
        console.error(`Render failed: ${renderResult.error}`);
        process.exit(1);
    }
    const _t2 = Date.now();
    if (verbose) {
        console.error(`[timing] session.render (widget render): ${_t2 - _t1}ms`);
        console.error(`[timing] node.js total (open+render): ${_t2 - _t0}ms`);
    }

    // Execute action pipeline
    for (const action of actions) {
        let result;

        switch (action.type) {
            case 'snapshot':
                result = session.snapshot();
                break;
            case 'click':
                result = await session.click(action.target);
                if (result.ok) {
                    console.log(result.message);
                    result = session.snapshot();
                }
                break;
            case 'fill':
                result = await session.fill(action.target, action.value);
                if (result.ok) {
                    console.log(result.message);
                    result = session.snapshot();
                }
                break;
            case 'select':
                result = await session.select(action.target, action.value);
                if (result.ok) {
                    console.log(result.message);
                    result = session.snapshot();
                }
                break;
            case 'tables':
                result = await session.tables(maxRows);
                break;
            case 'table':
                result = await session.table(action.id, maxRows);
                break;
            case 'html':
                result = session.html();
                break;
            case 'wait':
                result = await session.wait(action.ms || 5000);
                break;
        }

        if (!result.ok) {
            console.error(`Error (${action.type}): ${result.error}`);
            process.exit(1);
        }

        output(result);

        if (result.snapshot && snapshotHasError(result.snapshot)) {
            hasIllustratedError = true;
        }
    }
} catch (e) {
    console.error(`Fatal: ${e.message || e}`);
    process.exit(1);
} finally {
    session.close();
    // Force exit — JSAPI event loops may keep process alive
    setTimeout(() => process.exit(hasIllustratedError ? 1 : 0), 100);
}
