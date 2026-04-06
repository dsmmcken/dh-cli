/**
 * Suppress noisy console messages from Deephaven bundles and React.
 *
 * Shared by oneshot.mjs and render-daemon.mjs — add new patterns here.
 */

const SUPPRESSED = [
    'was not wrapped in act',
    'visible label',
    'aria-label',
    'Dashboard widget has changed',
    'Retrying url',
    'loadWidgetInternal',
];

function isSuppressed(args) {
    const msg = String(args[0] ?? '');
    return SUPPRESSED.some(s => msg.includes(s));
}

export function installConsoleSuppression() {
    const origLog = console.log;
    const origWarn = console.warn;
    const origError = console.error;
    console.log = (...args) => { if (!isSuppressed(args)) origLog.apply(console, args); };
    console.warn = (...args) => { if (!isSuppressed(args)) origWarn.apply(console, args); };
    console.error = (...args) => { if (!isSuppressed(args)) origError.apply(console, args); };
}
