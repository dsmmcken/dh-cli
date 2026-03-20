/**
 * Patches @deephaven/js-plugin-ui CJS bundle for Node.js compatibility:
 *
 * 1. Stubs heavy external packages (console, iris-grid, chart, etc.) that have
 *    ESM/CJS interop issues or heavy dependencies (monaco-editor).
 * 2. Fixes ESM→CJS interop for packages whose default export matters
 *    (e.g., @deephaven/log where `require()` returns namespace, not default).
 *
 * Run once after npm install:
 *   node src/patch-bundle.mjs
 *
 * Add to package.json scripts as a postinstall hook:
 *   "postinstall": "node src/patch-bundle.mjs"
 */

import { readFileSync, writeFileSync, existsSync } from 'node:fs';
import { resolve, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = dirname(fileURLToPath(import.meta.url));
const bundlePath = resolve(__dirname, '../node_modules/@deephaven/js-plugin-ui/dist/index.js');
const backupPath = bundlePath + '.original';

if (!existsSync(bundlePath)) {
    console.error('Bundle not found:', bundlePath);
    process.exit(1);
}

// Always restore from original before patching (idempotent)
if (existsSync(backupPath)) {
    writeFileSync(bundlePath, readFileSync(backupPath));
} else {
    writeFileSync(backupPath, readFileSync(bundlePath));
    console.log('Backed up original bundle.');
}

let source = readFileSync(bundlePath, 'utf8');
let patchCount = 0;

// ── 1. Stub heavy packages ──────────────────────────────────────────
// These have ESM/CJS interop issues or heavy deps (monaco, etc.)
// Used only by UITable (iris-grid), charts, console, and core panel plugins.
const stubPackages = [
    '@deephaven/console',
    '@deephaven/iris-grid',
    '@deephaven/chart',
    '@deephaven/grid',
    '@deephaven/dashboard-core-plugins',
    // '@deephaven/jsapi-components' — NOT stubbed: needed for table-backed Picker/ComboBox
    '@deephaven/storage',
    '@deephaven/file-explorer',
];

const stubProxy = `(new Proxy({}, { get(t, p) {
    if (p === '__esModule') return true;
    if (p === 'default') return {};
    if (typeof p === 'string' && /^[A-Z]/.test(p)) return function(){return null};
    return function(){};
} }))`;

for (const pkg of stubPackages) {
    const pattern = `require("${pkg}")`;
    if (source.includes(pattern)) {
        source = source.replaceAll(pattern, stubProxy);
        patchCount++;
        console.log(`Stubbed: ${pattern}`);
    }
}

// ── 2. Fix ESM→CJS interop for default exports ──────────────────────
// When Node.js CJS `require()` loads an ESM module, it returns the namespace
// object { default, ... } rather than the default export. The bundle was built
// expecting CJS-style `require()` that returns the default.
//
// Pattern: `const X = require("pkg")` → `const X = require("pkg").default || require("pkg")`
const interopPackages = [
    '@deephaven/log',
    '@deephaven/icons',
];

for (const pkg of interopPackages) {
    const pattern = `require("${pkg}")`;
    const replacement = `(function(m){return m && m.__esModule ? (m.default || m) : m})(require("${pkg}"))`;
    if (source.includes(pattern)) {
        source = source.replaceAll(pattern, replacement);
        patchCount++;
        console.log(`Interop fix: ${pattern}`);
    }
}

// ── 3. Fix react-redux interop ──────────────────────────────────────
// react-redux may also have ESM default export issues
const reactReduxPattern = 'require("react-redux")';
if (source.includes(reactReduxPattern)) {
    source = source.replaceAll(
        reactReduxPattern,
        `(function(m){return m && m.__esModule ? (m.default || m) : m})(require("react-redux"))`
    );
    patchCount++;
    console.log('Interop fix: react-redux');
}

// ── 4. Fix applyJsonPatch for root-level operations ─────────────────
// The bundle's applyJsonPatch wrapper ignores the return value from
// applyOperation, so root-level replace (path: "") returns the original
// empty object instead of the new document. The Python server sends
// root-level replaces for the initial document patch.
// We need two patches: change `const` to `let` and handle the return value
const constDocLine = 'const shallowCopyDocument = { ...document2 };';
const letDocLine = 'let shallowCopyDocument = { ...document2 };';
const applyJsonPatchOld = 'applyOperation(shallowCopyDocument, operation, false, true);\n  });\n  return shallowCopyDocument;\n}';
const applyJsonPatchNew = 'var opResult = applyOperation(shallowCopyDocument, operation, false, true);\n    if (operation.path === "" && opResult && opResult.newDocument !== shallowCopyDocument) { shallowCopyDocument = opResult.newDocument; }\n  });\n  return shallowCopyDocument;\n}';
if (source.includes(constDocLine) && source.includes(applyJsonPatchOld)) {
    source = source.replace(constDocLine, letDocLine);
    source = source.replace(applyJsonPatchOld, applyJsonPatchNew);
    patchCount++;
    console.log('Fixed: applyJsonPatch root-level replace handling');
}

// ── 5. Export internal components needed for test rendering ──────────
// WidgetHandler, PortalPanelManager, and PortalPanelManagerContext are internal
// to the bundle but needed to mount the real rendering pipeline in tests.
const exportLine = 'exports.DashboardPlugin = DashboardPlugin;';
if (source.includes(exportLine)) {
    source = source.replace(
        exportLine,
        [
            exportLine,
            'exports.WidgetHandler = WidgetHandler;',
            'exports.PortalPanelManager = PortalPanelManager;',
            'exports.PortalPanelManagerContext = PortalPanelManagerContext;',
        ].join('\n')
    );
    patchCount++;
    console.log('Exported: WidgetHandler, PortalPanelManager, PortalPanelManagerContext');
}

if (patchCount > 0) {
    writeFileSync(bundlePath, source);
    console.log(`\nPatched ${patchCount} items in bundle.`);
} else {
    console.log('No patches needed.');
}
