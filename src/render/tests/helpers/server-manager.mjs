/**
 * Server lifecycle helper for snapshot tests.
 *
 * Starts individual DH servers per Python test file using `--port 0`
 * for auto-assigned ports. Parses the assigned port from stdout/stderr.
 */
import { spawn } from 'node:child_process';
import { readdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';

/**
 * Extract the first widget name from a Python test file.
 * Looks for a top-level `xxx_widget = ` assignment.
 */
export function extractWidgetName(pyFilePath) {
    const content = readFileSync(pyFilePath, 'utf8');
    const match = content.match(/^(\w+_widget)\s*=/m);
    return match ? match[1] : null;
}

/**
 * Discover Python test files in a directory.
 *
 * @param {string} repoRoot - Absolute path to the repository root
 * @param {string} dir - Relative directory path (e.g. 'tests/components')
 * @param {string} prefix - File prefix filter (e.g. 'test_' or 'err_')
 * @returns {{ name: string, script: string, widget: string }[]}
 */
export function discoverPyFiles(repoRoot, dir, prefix) {
    const fullDir = join(repoRoot, dir);
    let files;
    try {
        files = readdirSync(fullDir);
    } catch {
        return [];
    }
    return files
        .filter(f => f.startsWith(prefix) && f.endsWith('.py'))
        .sort()
        .map(f => {
            const name = f.replace('.py', '');
            const widget = extractWidgetName(join(fullDir, f));
            return { name, script: join(dir, f), widget };
        })
        .filter(e => e.widget);
}

/**
 * Wait for a DH server to become ready by polling the JSAPI endpoint.
 */
export async function waitForServer(port, timeoutMs = 60000) {
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
 * Start a DH server for a single Python script using `--port 0`.
 *
 * Parses the auto-assigned port from stdout/stderr, then polls
 * the JSAPI endpoint until the server is ready.
 *
 * @param {string} pyScript - Relative path to the Python script (from cwd)
 * @param {object} [options]
 * @param {string} [options.cwd] - Working directory for the server process
 * @param {number} [options.timeoutMs=90000] - Timeout for server startup
 * @returns {Promise<{ port: number, url: string, proc: ChildProcess, kill: () => Promise<void> }>}
 */
export async function startServer(pyScript, { cwd, timeoutMs = 90000 } = {}) {
    const proc = spawn('dh', ['serve', pyScript, '--port', '0', '--no-browser'], {
        cwd,
        stdio: ['ignore', 'pipe', 'pipe'],
        detached: false,
    });

    let stdout = '';
    let stderr = '';
    proc.stdout.on('data', (chunk) => { stdout += chunk.toString(); });
    proc.stderr.on('data', (chunk) => { stderr += chunk.toString(); });

    // Wait for port to appear in output
    const start = Date.now();
    let port = null;

    const portPromise = new Promise((resolve, reject) => {
        const check = () => {
            const match = (stdout + stderr).match(/http:\/\/localhost:(\d+)/);
            if (match) {
                resolve(parseInt(match[1], 10));
            } else if (Date.now() - start > timeoutMs) {
                reject(new Error(
                    `No port found for ${pyScript} within ${timeoutMs}ms\n` +
                    `stdout: ${stdout.slice(-300)}\nstderr: ${stderr.slice(-300)}`
                ));
            } else {
                setTimeout(check, 500);
            }
        };
        check();

        proc.on('error', (err) => {
            reject(new Error(`Failed to spawn server for ${pyScript}: ${err.message}`));
        });

        proc.on('exit', (code) => {
            if (port === null) {
                reject(new Error(
                    `Server for ${pyScript} exited with code ${code} before port was assigned\n` +
                    `stderr: ${stderr.slice(-500)}`
                ));
            }
        });
    });

    port = await portPromise;
    const url = `http://localhost:${port}`;

    // Wait for JSAPI to be ready
    const remaining = timeoutMs - (Date.now() - start);
    await waitForServer(port, Math.max(remaining, 10000));

    return {
        port,
        url,
        proc,
        async kill() {
            try { proc.kill('SIGTERM'); } catch {}
            await new Promise(resolve => {
                const timer = setTimeout(() => {
                    try { proc.kill('SIGKILL'); } catch {}
                    resolve();
                }, 3000);
                proc.on('exit', () => { clearTimeout(timer); resolve(); });
            });
        },
    };
}
