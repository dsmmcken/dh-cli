/**
 * Setup file for snapshot tests (runs in each test worker).
 *
 * - Checks whether the `dh` CLI is available so tests can skip gracefully
 * - Suppresses React act() warnings (expected with async WidgetHandler updates)
 */
import { execFileSync } from 'node:child_process';

// Fast PATH check instead of running `dh --version`
try {
    execFileSync('which', ['dh'], { timeout: 2000, stdio: 'ignore' });
    globalThis.__DH_SERVER_AVAILABLE__ = true;
} catch {
    globalThis.__DH_SERVER_AVAILABLE__ = false;
}

// Suppress noisy warnings that are expected in snapshot tests:
// - React act() warnings: WidgetHandler receives async server messages outside act()
// - Spectrum aria-label warnings: stubs don't provide visible labels
const _origError = console.error;
const _origWarn = console.warn;
const _suppress = (s) =>
    s.includes('not wrapped in act') ||
    s.includes('you must specify an aria-label') ||
    s.includes('If you do not provide a visible label');
console.error = (...args) => {
    if (typeof args[0] === 'string' && _suppress(args[0])) return;
    _origError.apply(console, args);
};
console.warn = (...args) => {
    if (typeof args[0] === 'string' && _suppress(args[0])) return;
    _origWarn.apply(console, args);
};
