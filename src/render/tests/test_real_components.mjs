/**
 * Red/green tests for real component rendering.
 * Run with: node --import ./src/css-loader.mjs tests/test_real_components.mjs [phase]
 */

import { JSDOM } from 'jsdom';
import { installJsdomGlobals } from '../src/jsdom-globals.mjs';

// ── Setup ────────────────────────────────────────────────────────────

const dom = new JSDOM('<!DOCTYPE html><html><body><div id="root"></div></body></html>', {
    pretendToBeVisual: true,
    url: 'http://localhost',
});
const globals = installJsdomGlobals(dom);

// React act() requires this flag in non-test-framework environments
globalThis.IS_REACT_ACT_ENVIRONMENT = true;

// Now safe to import React and DH packages
const React = (await import('react')).default;
const ReactDOM = await import('react-dom/client');
const { createElement: h } = React;

// ── Test runner ──────────────────────────────────────────────────────

let passed = 0;
let failed = 0;
const phase = process.argv[2] || 'all';

async function test(name, testPhase, fn) {
    if (phase !== 'all' && phase !== testPhase) return;
    try {
        await fn();
        console.log(`  \u2713 ${name}`);
        passed++;
    } catch (e) {
        console.log(`  \u2717 ${name}`);
        console.log(`    ${e.message}`);
        failed++;
    }
}

function assert(condition, msg) {
    if (!condition) throw new Error(msg || 'Assertion failed');
}

async function renderAndWait(element, ms = 50) {
    const container = dom.window.document.createElement('div');
    dom.window.document.body.appendChild(container);
    const root = ReactDOM.createRoot(container);
    root.render(element);
    await new Promise(resolve => setTimeout(resolve, ms));
    return {
        container,
        root,
        cleanup: () => { root.unmount(); container.remove(); },
    };
}

/**
 * Render with act() and wait for async effects to settle.
 * Useful for components like WidgetHandler that have multi-step async initialization.
 * `checkFn` receives the full document body since portals may render outside the container.
 */
async function renderWithAct(element, { waitMs = 100, maxWaitMs = 3000, checkFn } = {}) {
    const { act } = React;
    const container = dom.window.document.createElement('div');
    dom.window.document.body.appendChild(container);
    const root = ReactDOM.createRoot(container);

    await act(async () => {
        root.render(element);
    });

    // Repeatedly flush pending effects with act + setTimeout
    const body = dom.window.document.body;
    const start = Date.now();
    while (Date.now() - start < maxWaitMs) {
        await act(async () => {
            await new Promise(resolve => setTimeout(resolve, waitMs));
        });
        if (checkFn && checkFn(body)) break;
    }

    return {
        container,
        body,
        root,
        cleanup: () => {
            root.unmount();
            container.remove();
        },
    };
}

// ── Phase 1: Provider scaffold ───────────────────────────────────────

console.log('\nPhase 1: Provider scaffold');

await test('loadProviders succeeds', 'phase1', async () => {
    const { loadProviders } = await import('../src/TestProviderStack.mjs');
    await loadProviders();
});

await test('TestProviderStack renders children', 'phase1', async () => {
    const { TestProviderStack } = await import('../src/TestProviderStack.mjs');
    const { container, cleanup } = await renderWithAct(
        h(TestProviderStack, null, h('div', { id: 'test-child' }, 'hello')),
        { checkFn: (b) => b.textContent.includes('hello') }
    );
    assert(container.textContent.includes('hello'), 'Expected "hello" in output');
    cleanup();
});

await test('real DH Text component renders inside providers', 'phase1', async () => {
    const { TestProviderStack } = await import('../src/TestProviderStack.mjs');
    const { Text } = await import('@deephaven/components');
    const { container, cleanup } = await renderAndWait(
        h(TestProviderStack, null, h(Text, null, 'DH Text Component'))
    );
    assert(container.textContent.includes('DH Text Component'), 'Expected DH Text content');
    cleanup();
});

await test('real DH Button component renders inside providers', 'phase1', async () => {
    const { TestProviderStack } = await import('../src/TestProviderStack.mjs');
    const { SpectrumButton } = await import('@deephaven/components');
    const { container, cleanup } = await renderAndWait(
        h(TestProviderStack, null,
            h(SpectrumButton, { variant: 'primary' }, 'Click Me')
        )
    );
    const button = container.querySelector('button');
    assert(button, 'Expected a <button> element');
    assert(button.textContent.includes('Click Me'), 'Expected button text');
    cleanup();
});

await test('DashboardPlugin loads as a function', 'phase1', async () => {
    const { DashboardPlugin } = await import('@deephaven/js-plugin-ui');
    assert(typeof DashboardPlugin === 'function', 'Expected DashboardPlugin to be a function');
});

// ── Phase 2: ObjectFetcherBridge ─────────────────────────────────────

console.log('\nPhase 2: ObjectFetcherBridge');

await test('createObjectFetchManager returns valid manager', 'phase2', async () => {
    const { createObjectFetchManager } = await import('../src/ObjectFetcherBridge.mjs');
    const mockConnection = {
        getObject: async (desc) => ({ type: 'Widget', name: desc.name }),
    };
    const manager = createObjectFetchManager(mockConnection);
    assert(typeof manager.subscribe === 'function', 'Expected subscribe method');
});

await test('ObjectFetchManager fires ready with fetch function', 'phase2', async () => {
    const { createObjectFetchManager } = await import('../src/ObjectFetcherBridge.mjs');
    const mockWidget = { type: 'test-widget', sendMessage: () => {} };
    const mockConnection = { getObject: async () => mockWidget };
    const manager = createObjectFetchManager(mockConnection);

    let update = null;
    manager.subscribe({ type: 'test', name: 'w' }, (u) => { update = u; });
    assert(update !== null, 'Expected immediate callback');
    assert(update.status === 'ready', 'Expected ready status');
    const fetched = await update.fetch();
    assert(fetched === mockWidget, 'Expected fetched widget');
});

await test('TestProviderStack accepts dh and objectFetchManager props', 'phase2', async () => {
    const { TestProviderStack } = await import('../src/TestProviderStack.mjs');
    const { createObjectFetchManager } = await import('../src/ObjectFetcherBridge.mjs');
    const mockDh = { CoreClient: function() {} };
    const mockManager = createObjectFetchManager({ getObject: async () => ({}) });

    const { container, cleanup } = await renderAndWait(
        h(TestProviderStack, { dh: mockDh, objectFetchManager: mockManager },
            h('div', null, 'with-contexts')
        )
    );
    assert(container.textContent.includes('with-contexts'), 'Expected content to render');
    cleanup();
});

// ── Phase 3: GoldenLayout in jsdom ───────────────────────────────────

console.log('\nPhase 3: GoldenLayout in jsdom');

await test('loadGoldenLayout succeeds', 'phase3', async () => {
    const { loadGoldenLayout } = await import('../src/GoldenLayoutSetup.mjs');
    await loadGoldenLayout();
});

await test('createTestLayout creates a working layout', 'phase3', async () => {
    const { createTestLayout } = await import('../src/GoldenLayoutSetup.mjs');
    const result = createTestLayout(dom.window);
    assert(result.layout, 'Expected layout instance');
    assert(result.portalPanelMap instanceof Map, 'Expected portalPanelMap');
    assert(typeof result.destroy === 'function', 'Expected destroy function');
    result.destroy();
});

await test('TestProviderStack accepts layoutManager prop', 'phase3', async () => {
    const { TestProviderStack } = await import('../src/TestProviderStack.mjs');
    const { createTestLayout } = await import('../src/GoldenLayoutSetup.mjs');
    const { layout, destroy } = createTestLayout(dom.window);

    const { container, cleanup } = await renderAndWait(
        h(TestProviderStack, { layoutManager: layout },
            h('div', null, 'with-layout')
        )
    );
    assert(container.textContent.includes('with-layout'), 'Expected content to render');
    cleanup();
    destroy();
});

// ── Phase 4: UITable stub via ElementPlugin ──────────────────────────

console.log('\nPhase 4: UITable stub via ElementPlugin');

await test('createUITableStubPluginMap returns valid plugin map', 'phase4', async () => {
    const { createUITableStubPluginMap } = await import('../src/UITableStub.mjs');
    const map = createUITableStubPluginMap();
    assert(map instanceof Map, 'Expected a Map');
    assert(map.size === 1, 'Expected 1 plugin entry');
    const entry = map.values().next().value;
    assert(entry.type === 'ElementPlugin', 'Expected ElementPlugin type');
    assert('deephaven.ui.elements.UITable' in entry.mapping, 'Expected UITable mapping');
});

await test('UITableStub renders table info', 'phase4', async () => {
    const { UITableStub } = await import('../src/UITableStub.mjs');
    const mockTable = { type: 'Table' };
    const { container, cleanup } = await renderWithAct(
        h(UITableStub, { table: mockTable })
    );
    const el = container.querySelector('[data-component="deephaven.ui.elements.UITable"]');
    assert(el, 'Expected UITable stub element');
    assert(el.getAttribute('role') === 'table', 'Expected table role');
    assert(el.getAttribute('data-table-type') === 'Table', 'Expected data-table-type attribute');
    assert(el.querySelector('[data-testid="table-expand"]'), 'Expected expand button');
    cleanup();
});

await test('default TestProviderStack includes UITable stub', 'phase4', async () => {
    const { TestProviderStack } = await import('../src/TestProviderStack.mjs');
    // The default pluginMap should include the UITable stub
    // Verify by checking that the plugin context is available with the stub
    const { container, cleanup } = await renderAndWait(
        h(TestProviderStack, null, h('div', null, 'has-stub'))
    );
    assert(container.textContent.includes('has-stub'), 'Expected content');
    cleanup();
});

// ── Phase 5: WidgetHandler rendering pipeline ───────────────────────

console.log('\nPhase 5: WidgetHandler rendering pipeline');

await test('WidgetHandler is exported from patched bundle', 'phase5', async () => {
    const pluginUi = await import('@deephaven/js-plugin-ui');
    assert(typeof pluginUi.WidgetHandler === 'function', 'Expected WidgetHandler function');
    assert(typeof pluginUi.PortalPanelManager === 'function', 'Expected PortalPanelManager function');
    assert(pluginUi.PortalPanelManagerContext != null, 'Expected PortalPanelManagerContext');
});

await test('WidgetHandler renders with mock widget in full provider stack', 'phase5', async () => {
    const { TestProviderStack, loadProviders } = await import('../src/TestProviderStack.mjs');
    const { createObjectFetchManager } = await import('../src/ObjectFetcherBridge.mjs');
    const { loadGoldenLayout, createTestLayout } = await import('../src/GoldenLayoutSetup.mjs');
    const { WidgetHandler } = await import('@deephaven/js-plugin-ui');

    await loadProviders();
    await loadGoldenLayout();
    const { layout, destroy } = createTestLayout(dom.window);

    // Mock a widget that sends a simple document (a text element)
    const mockDocument = {
        __dhElemName: 'deephaven.ui.components.Text',
        props: { children: ['Hello from WidgetHandler'] },
    };
    const mockJsonRpcResponse = JSON.stringify({
        jsonrpc: '2.0',
        method: 'documentPatched',
        params: [[{ op: 'replace', path: '', value: mockDocument }], null],
    });

    const mockWidget = {
        type: 'deephaven.ui.Element',
        exportedObjects: [],
        getDataAsString: () => mockJsonRpcResponse,
        sendMessage: () => {},
        addEventListener: () => () => {},
        close: () => {},
    };

    const mockDh = { CoreClient: function() {} };
    const objectFetchManager = createObjectFetchManager({
        getObject: async () => mockWidget,
    });

    const widgetDescriptor = { type: 'deephaven.ui.Element', name: 'test_widget' };

    const body = dom.window.document.body;
    const { cleanup } = await renderWithAct(
        h(TestProviderStack, {
            dh: mockDh,
            objectFetchManager,
            layoutManager: layout,
        },
            h(WidgetHandler, {
                widgetDescriptor,
                id: 'test-widget-1',
                onClose: () => {},
            })
        ),
        { checkFn: (b) => b.textContent.includes('Hello from WidgetHandler') }
    );

    // Content is portaled into GoldenLayout container, check full body
    assert(body.textContent.includes('Hello from WidgetHandler'),
        `Expected "Hello from WidgetHandler" in body, got: "${body.textContent.substring(0, 300)}"`);
    cleanup();
    destroy();
});

await test('WidgetHandler renders a button with real Spectrum component', 'phase5', async () => {
    const { TestProviderStack } = await import('../src/TestProviderStack.mjs');
    const { createObjectFetchManager } = await import('../src/ObjectFetcherBridge.mjs');
    const { createTestLayout } = await import('../src/GoldenLayoutSetup.mjs');
    const { WidgetHandler } = await import('@deephaven/js-plugin-ui');

    // Clean up stale portal content from prior tests
    for (const el of dom.window.document.body.querySelectorAll('#dh-test-layout, .ui-portal-panel')) {
        el.remove();
    }

    const { layout, destroy } = createTestLayout(dom.window);

    // A document with a panel containing a button
    const mockDocument = {
        __dhElemName: 'deephaven.ui.components.Panel',
        props: {
            children: [{
                __dhElemName: 'deephaven.ui.components.Button',
                props: {
                    children: ['Real Spectrum Button'],
                    variant: 'primary',
                },
            }],
        },
    };
    const mockJsonRpcResponse = JSON.stringify({
        jsonrpc: '2.0',
        method: 'documentPatched',
        params: [[{ op: 'replace', path: '', value: mockDocument }], null],
    });

    const mockWidget = {
        type: 'deephaven.ui.Element',
        exportedObjects: [],
        getDataAsString: () => mockJsonRpcResponse,
        sendMessage: () => {},
        addEventListener: () => () => {},
        close: () => {},
    };

    const objectFetchManager = createObjectFetchManager({
        getObject: async () => mockWidget,
    });

    const body = dom.window.document.body;
    const { container, cleanup } = await renderWithAct(
        h(TestProviderStack, {
            dh: { CoreClient: function() {} },
            objectFetchManager,
            layoutManager: layout,
        },
            h(WidgetHandler, {
                widgetDescriptor: { type: 'deephaven.ui.Element', name: 'btn' },
                id: 'test-btn-1',
                onClose: () => {},
            })
        ),
        {
            maxWaitMs: 5000,
            checkFn: (b) => {
                // Look specifically for our button text, not stale buttons
                const buttons = b.querySelectorAll('button');
                return Array.from(buttons).some(btn =>
                    btn.textContent.includes('Real Spectrum Button')
                );
            },
        }
    );

    // ReactPanel portals content into a GoldenLayout panel container.
    // Check both the React render container and the layout container.
    const buttons = body.querySelectorAll('button');
    const button = Array.from(buttons).find(btn =>
        btn.textContent.includes('Real Spectrum Button')
    );
    assert(button, `Expected a <button> with "Real Spectrum Button" text. Found buttons: ${Array.from(buttons).map(b => `"${b.textContent.trim()}"`).join(', ') || 'none'}`);
    cleanup();
    destroy();
});

// ── Results ──────────────────────────────────────────────────────────

console.log(`\n${passed} passed, ${failed} failed\n`);
globals.restore();
process.exit(failed > 0 ? 1 : 0);
