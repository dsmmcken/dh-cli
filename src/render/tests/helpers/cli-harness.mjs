/**
 * CLI harness for integration tests.
 * Extracted from test_all_components.mjs.
 */
import { execSync } from 'node:child_process';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';

const __filename = fileURLToPath(import.meta.url);
const __dirname = join(__filename, '..');
const DH_RENDER = join(__dirname, '..', '..', 'bin', 'dh-render.mjs');
const DH_RENDER_DIR = join(__dirname, '..', '..');
const SERVER_URL = process.env.DH_URL || 'http://localhost:10000';
const ERROR_SERVER_URL = process.env.DH_ERROR_URL || 'http://localhost:10001';
const COMPLEX_SERVER_URL = process.env.DH_COMPLEX_URL || 'http://localhost:10002';

// Track which server the daemon is connected to
let _connectedTo = null;

/**
 * Execute a dh-render CLI command.
 */
export function dhr(cmd, timeout = 45000) {
    return execSync(`node ${DH_RENDER} ${cmd}`, {
        encoding: 'utf8',
        timeout,
        cwd: DH_RENDER_DIR,
    }).trim();
}

export function render(widgetName) {
    return dhr(`render ${widgetName}`, 60000);
}

export function snapshot() {
    return dhr('snapshot');
}

export function click(target) {
    return dhr(`click ${target}`);
}

export function fill(target, value) {
    return dhr(`fill "${target}" "${value}"`);
}

export function html() {
    return dhr('html');
}

export function openSession(serverUrl) {
    try { dhr('close', 5000); } catch {}
    dhr(`open ${serverUrl || SERVER_URL}`, 60000);
    _connectedTo = 'component';
}

export function closeSession() {
    try { dhr('close', 5000); } catch {}
    _connectedTo = null;
}

export function openErrorSession(serverUrl) {
    try { dhr('close', 5000); } catch {}
    dhr(`open ${serverUrl || ERROR_SERVER_URL}`, 60000);
    _connectedTo = 'error';
}

/**
 * Ensure the component session is active.
 * If the daemon is connected to a different server or has no session,
 * re-open a session to the component server. No-op if already connected
 * to the component server.
 */
export function ensureComponentSession() {
    if (_connectedTo === 'component') return;
    openSession();
}

/**
 * Check whether the daemon is alive and responsive.
 * Returns true if the daemon responds to a status ping.
 */
export function isSessionAlive() {
    try {
        dhr('status', 5000);
        return true;
    } catch {
        _connectedTo = null;
        return false;
    }
}

/**
 * Ensure the component session is alive (verifies daemon health).
 * Unlike ensureComponentSession(), this pings the daemon and reconnects
 * if it has crashed.
 */
export function ensureComponentSessionAlive() {
    if (_connectedTo === 'component' && isSessionAlive()) return;
    openSession();
}

/**
 * Ensure the error session is alive (verifies daemon health).
 */
export function ensureErrorSessionAlive() {
    if (_connectedTo === 'error' && isSessionAlive()) return;
    openErrorSession();
}

export function openComplexSession(serverUrl) {
    try { dhr('close', 5000); } catch {}
    dhr(`open ${serverUrl || COMPLEX_SERVER_URL}`, 60000);
    _connectedTo = 'complex';
}

export function ensureComplexSessionAlive() {
    if (_connectedTo === 'complex' && isSessionAlive()) return;
    openComplexSession();
}
