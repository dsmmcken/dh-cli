/**
 * Error snapshot tests — auto-discovers tests/scripts/errors/err_*.py,
 * starts a DH server per test, renders, and compares the accessibility-tree
 * snapshot against files in tests/snapshots/errors/.
 *
 * Error widgets are expected to produce errors — the snapshot captures
 * either the DH error boundary output or a [render error] message.
 *
 * Run:     npx vitest run --project snapshots -t 'error'
 * Update:  npx vitest run --project snapshots -t 'error' --update
 */
import { describe, it, expect } from 'vitest';
import { join, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import { startServer, discoverPyFiles } from '../helpers/server-manager.mjs';
import { DaemonSession } from '../../src/cli/session.mjs';

const __dirname = dirname(fileURLToPath(import.meta.url));
const REPO_ROOT = join(__dirname, '..', '..');
const SNAPSHOTS_DIR = join(REPO_ROOT, 'tests', 'snapshots');

const entries = discoverPyFiles(REPO_ROOT, 'tests/scripts/errors', 'err_');

describe.runIf(globalThis.__DH_SERVER_AVAILABLE__ && entries.length > 0)('error snapshots', () => {
    for (const { name, widget, script } of entries) {
        it(name, async () => {
            const server = await startServer(script, { cwd: REPO_ROOT });
            try {
                const session = new DaemonSession();
                let snap;
                try {
                    await session.open(server.url);
                    await session.render(widget, null, 30000);
                    try { await session.wait(5000); } catch {}
                    const result = session.snapshot();
                    snap = result.snapshot;
                } catch (e) {
                    snap = `[render error]\n${(e.stderr || e.message || String(e)).trim()}`;
                }
                session.close();

                await expect(snap).toMatchFileSnapshot(
                    join(SNAPSHOTS_DIR, 'errors', `${name}.snap`)
                );
            } finally {
                await server.kill();
            }
        }, 120000);
    }
});
