/**
 * Vitest globalSetup for integration tests.
 *
 * Starts DH servers before integration tests and tears them down after.
 * - Port 10000: all_components.py (component tests)
 * - Port 10001: all_errors.py (error tests)
 *
 * Servers are started from /workspace (the repo root) because the Python
 * scripts use relative exec(open(...)) paths.
 *
 * Set DH_SKIP_SERVER=1 to skip automatic server management (e.g. if you
 * started servers manually).
 */
import { spawn } from 'node:child_process';
import { join, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import { writeFileSync, unlinkSync } from 'node:fs';

const __filename = fileURLToPath(import.meta.url);
const REPO_ROOT = join(dirname(__filename), '..', '..');
const MARKER_FILE = join(dirname(__filename), '.dh-server-ready');

const COMPONENT_PORT = Number(process.env.DH_PORT || 10000);
const ERROR_PORT = Number(process.env.DH_ERROR_PORT || 10001);
const COMPONENT_SCRIPT = 'tests/scripts/components/all_components.py';
const ERROR_SCRIPT = 'tests/scripts/errors/all_errors.py';

const servers = [];

/**
 * Wait for a DH server to become ready by polling the JSAPI endpoint.
 */
async function waitForServer(port, timeoutMs = 60000) {
    const url = `http://localhost:${port}/jsapi/dh-core.js`;
    const start = Date.now();
    while (Date.now() - start < timeoutMs) {
        try {
            const controller = new AbortController();
            const timer = setTimeout(() => controller.abort(), 2000);
            const resp = await fetch(url, { signal: controller.signal });
            clearTimeout(timer);
            if (resp.ok) return true;
        } catch {
            // not ready yet
        }
        await new Promise(r => setTimeout(r, 1000));
    }
    throw new Error(`Server on port ${port} did not start within ${timeoutMs}ms`);
}

/**
 * Start a DH server in the background.
 */
function startServer(script, port) {
    const proc = spawn('dh', ['serve', script, '--port', String(port), '--no-browser'], {
        cwd: REPO_ROOT,
        stdio: ['ignore', 'pipe', 'pipe'],
        detached: false,
    });

    let stderr = '';
    proc.stderr.on('data', (chunk) => { stderr += chunk.toString(); });

    proc.on('error', (err) => {
        console.error(`Failed to start server on port ${port}: ${err.message}`);
    });

    return { proc, port, getStderr: () => stderr };
}

/**
 * Check if a server is already running on a port.
 */
async function isServerRunning(port) {
    try {
        const controller = new AbortController();
        const timer = setTimeout(() => controller.abort(), 2000);
        const resp = await fetch(`http://localhost:${port}/jsapi/dh-core.js`, { signal: controller.signal });
        clearTimeout(timer);
        return resp.ok;
    } catch {
        return false;
    }
}

export async function setup() {
    // Clean any stale marker
    try { unlinkSync(MARKER_FILE); } catch {}

    if (process.env.DH_SKIP_SERVER === '1') {
        console.log('DH_SKIP_SERVER=1 — checking for manually started servers...');
        const componentUp = await isServerRunning(COMPONENT_PORT);
        const errorUp = await isServerRunning(ERROR_PORT);
        if (componentUp) {
            console.log(`  Component server found on port ${COMPONENT_PORT}`);
            // Write marker so workers know servers are available
            writeFileSync(MARKER_FILE, JSON.stringify({ componentPort: COMPONENT_PORT, errorPort: errorUp ? ERROR_PORT : null }));
        } else {
            console.log(`  No server on port ${COMPONENT_PORT} — integration tests will skip`);
        }
        return;
    }

    console.log('Starting DH servers for integration tests...');

    // Start both servers in parallel
    const componentServer = startServer(COMPONENT_SCRIPT, COMPONENT_PORT);
    const errorServer = startServer(ERROR_SCRIPT, ERROR_PORT);
    servers.push(componentServer, errorServer);

    // Wait for both to be ready
    try {
        await Promise.all([
            waitForServer(COMPONENT_PORT).then(() => {
                console.log(`  Component server ready on port ${COMPONENT_PORT}`);
            }),
            waitForServer(ERROR_PORT).then(() => {
                console.log(`  Error server ready on port ${ERROR_PORT}`);
            }),
        ]);
    } catch (err) {
        for (const s of servers) {
            const log = s.getStderr();
            if (log) console.error(`  Server port ${s.port} stderr:\n${log.slice(-500)}`);
        }
        for (const s of servers) {
            try { s.proc.kill('SIGTERM'); } catch {}
        }
        throw err;
    }

    // Write marker file so forked workers know servers are available
    writeFileSync(MARKER_FILE, JSON.stringify({
        componentPort: COMPONENT_PORT,
        errorPort: ERROR_PORT,
    }));
}

export async function teardown() {
    // Clean marker file
    try { unlinkSync(MARKER_FILE); } catch {}

    if (process.env.DH_SKIP_SERVER === '1') return;

    console.log('Stopping DH servers...');
    for (const s of servers) {
        try {
            s.proc.kill('SIGTERM');
            await new Promise(r => setTimeout(r, 500));
            if (!s.proc.killed) {
                s.proc.kill('SIGKILL');
            }
        } catch {
            // already dead
        }
    }
    servers.length = 0;
}
