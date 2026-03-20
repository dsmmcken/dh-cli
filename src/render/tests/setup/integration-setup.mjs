/**
 * Setup file for integration tests (runs in each test worker).
 *
 * Reads a marker file written by the globalSetup to determine if DH servers
 * are available. Sets globalThis.__DH_SERVER_AVAILABLE__ synchronously so
 * describe.runIf() can evaluate it at module parse time.
 *
 * With singleFork mode, all test files share one worker process. The session
 * is opened here once and shared by all test files.
 */
import { readFileSync } from 'node:fs';
import { join, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';

const __filename = fileURLToPath(import.meta.url);
const MARKER_FILE = join(dirname(__filename), '.dh-server-ready');

let serverInfo;
try {
    serverInfo = JSON.parse(readFileSync(MARKER_FILE, 'utf8'));
    globalThis.__DH_SERVER_AVAILABLE__ = true;
    if (serverInfo.componentPort) process.env.DH_URL = `http://localhost:${serverInfo.componentPort}`;
    if (serverInfo.errorPort) process.env.DH_ERROR_URL = `http://localhost:${serverInfo.errorPort}`;
} catch {
    globalThis.__DH_SERVER_AVAILABLE__ = false;
}

// Open session eagerly at module evaluation time (before any tests run).
// This runs once per worker. With singleFork, there's exactly one worker.
if (globalThis.__DH_SERVER_AVAILABLE__) {
    const { openSession, closeSession } = await import('../helpers/cli-harness.mjs');
    try {
        openSession();
        globalThis.__DH_CLOSE_SESSION__ = closeSession;
    } catch (e) {
        console.error(`Failed to open CLI session: ${e.message}`);
        globalThis.__DH_SERVER_AVAILABLE__ = false;
    }
}
