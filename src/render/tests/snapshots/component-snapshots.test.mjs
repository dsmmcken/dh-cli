/**
 * Component snapshot tests — auto-discovers tests/scripts/components/test_*.py,
 * starts a DH server per test, renders, and compares the accessibility-tree
 * snapshot against files in tests/snapshots/components/.
 *
 * Snapshots containing DH error boundaries or empty content fail the test.
 * Vitest controls concurrency across test files (parallel by default).
 *
 * Run:     npx vitest run --project snapshots -t 'component'
 * Update:  npx vitest run --project snapshots -t 'component' --update
 */
import { describe, it, expect } from 'vitest';
import { join, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import { startServer, discoverPyFiles } from '../helpers/server-manager.mjs';
import { DaemonSession } from '../../src/cli/session.mjs';

const __dirname = dirname(fileURLToPath(import.meta.url));
const REPO_ROOT = join(__dirname, '..', '..');
const SNAPSHOTS_DIR = join(REPO_ROOT, 'tests', 'snapshots');

const entries = discoverPyFiles(REPO_ROOT, 'tests/scripts/components', 'test_');

/**
 * Assert that a snapshot does not contain DH error boundary markers
 * or indicate a timeout/empty render.
 */
function assertNoErrors(snapshot, name) {
    if (!snapshot || !snapshot.trim()) {
        throw new Error(`Snapshot for "${name}" is empty (possible timeout or render failure)`);
    }
    if (/^\[render error\]/m.test(snapshot)) {
        throw new Error(
            `Snapshot for "${name}" contains a render error:\n${snapshot}`
        );
    }
    const hasReload = /\[button\] "Reload"/.test(snapshot);
    const hasInfo = /\[button\] "Information"/.test(snapshot);
    if (hasReload && hasInfo) {
        throw new Error(
            `Snapshot for "${name}" contains a DH error boundary:\n${snapshot}`
        );
    }
}

describe.runIf(globalThis.__DH_SERVER_AVAILABLE__ && entries.length > 0)('component snapshots', () => {
    for (const { name, widget, script } of entries) {
        it(name, async () => {
            const server = await startServer(script, { cwd: REPO_ROOT });
            try {
                const session = new DaemonSession();
                await session.open(server.url);
                await session.render(widget, null, 30000);
                try { await session.wait(5000); } catch {}

                const result = session.snapshot();
                const snap = result.snapshot;
                session.close();

                assertNoErrors(snap, name);

                await expect(snap).toMatchFileSnapshot(
                    join(SNAPSHOTS_DIR, 'components', `${name}.snap`)
                );
            } finally {
                await server.kill();
            }
        }, 120000);
    }
});
