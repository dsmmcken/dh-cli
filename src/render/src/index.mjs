/**
 * dh-render-test - Test Deephaven UI components with real data in Node.js
 *
 * Renders widgets using the real WidgetHandler pipeline from @deephaven/js-plugin-ui,
 * with full GoldenLayout portal support and real Spectrum components.
 *
 * Usage:
 *   import { renderWidget, createTestClient } from 'dh-render-test';
 *
 *   // Quick one-shot render
 *   const result = await renderWidget('http://localhost:10000', 'my_widget');
 *   console.log(result.body.textContent);
 *   console.log(result.querySelector('button').textContent);
 *
 *   // Advanced usage with test client
 *   const client = await createTestClient('http://localhost:10000');
 *   const result = await client.render('my_widget');
 *   // Interact via DOM events, then flush React effects
 *   result.querySelector('button').click();
 *   await result.flush();
 *   client.close();
 */

export { JsApiLoader } from './JsApiLoader.mjs';
export { WidgetClient, CALLABLE_KEY, OBJECT_KEY, ELEMENT_KEY } from './WidgetClient.mjs';
export { DocumentRenderer } from './DocumentRenderer.mjs';
export { DEFAULT_COMPONENT_MAP, createComponentMap } from './ComponentMap.mjs';
export {
    findAllCallables,
    findAllObjects,
    findAllElements,
    findCallableByProp,
    findCallableByButtonText,
    findCallableByElement,
    getAtPath,
    prettyPrintDocument,
} from './helpers.mjs';
export {
    assertRowCount,
    assertMinRowCount,
    assertColumns,
    assertColumnEquals,
    assertColumnContains,
    assertColumnAll,
    assertTableHas,
    assertColumnSorted,
    assertColumnUnique,
    assertColumnInRange,
} from './tableAssertions.mjs';
export { TapReporter } from './TapReporter.mjs';
export { diagnoseWidget, listWidgets } from './diagnose.mjs';

import { JsApiLoader } from './JsApiLoader.mjs';
import { WidgetClient } from './WidgetClient.mjs';
import { installJsdomGlobals } from './jsdom-globals.mjs';
import { loadProviders, TestProviderStack } from './TestProviderStack.mjs';
import { loadGoldenLayout, createTestLayout } from './GoldenLayoutSetup.mjs';
import { createObjectFetchManager } from './ObjectFetcherBridge.mjs';
import { FigureWidgetPlugin, isFigureType } from './FigureStub.mjs';
import React from 'react';

const { createElement: h } = React;

/** Types that bypass WidgetHandler (they don't implement JSON-RPC widget protocol). */
const DIRECT_RENDER_TYPES = new Set(['Figure']);

/**
 * Create a reusable test client for a Deephaven server.
 * Loads JSAPI, installs jsdom globals, sets up GoldenLayout and providers.
 *
 * @param {string} serverUrl - The Deephaven server URL
 * @param {object} [options]
 * @returns {Promise<TestClient>}
 */
export async function createTestClient(serverUrl, options = {}) {
    const loader = new JsApiLoader(serverUrl);

    // Start JSAPI download and jsdom creation in parallel — they're independent.
    // JSAPI is the slow part (~1200ms); jsdom is fast but free to overlap.
    const [dh] = await Promise.all([
        loader.loadJSAPI(),
        Promise.resolve(loader.createDom()),
    ]);
    const dom = loader.dom;

    // Connect to server BEFORE installing jsdom globals.
    // Node.js v24's native WebSocket breaks if globalThis.Event is replaced
    // with jsdom's Event class (instanceof check fails in Node internals).
    const coreClient = new dh.CoreClient(serverUrl);
    await coreClient.login({ type: dh.CoreClient.LOGIN_TYPE_ANONYMOUS });
    const connection = await coreClient.getAsIdeConnection();

    // Suppress noisy reconnection logs during test teardown
    connection.addEventListener('disconnect', () => {});
    connection.addEventListener('requestfailed', () => {});

    // Complex dashboards open many table subscriptions that each add a close
    // listener to the shared WebSocket.  Raise the limit to avoid a spurious
    // MaxListenersExceededWarning (not a real leak).
    try {
        const { setMaxListeners } = await import('node:events');
        const ws = coreClient.getWebSocket?.() ?? connection.getWebSocket?.();
        if (ws) {
            setMaxListeners(50, ws);
        }
    } catch (_) { /* older Node or no getWebSocket — ignore */ }

    // Now install jsdom globals for React-DOM and DH components
    const globals = installJsdomGlobals(dom);
    globalThis.IS_REACT_ACT_ENVIRONMENT = true;

    // Load providers and GoldenLayout in parallel (both need jsdom globals).
    // ReactDOM can also load in parallel — it's independent.
    const [, , ReactDOM] = await Promise.all([
        loadProviders(),
        loadGoldenLayout(),
        import('react-dom/client'),
    ]);

    // WidgetHandler must load after providers (shares @deephaven/dashboard dep graph)
    const pluginUi = await import('@deephaven/js-plugin-ui');
    const WidgetHandler = pluginUi.WidgetHandler;

    // Create ObjectFetchManager wrapping the connection
    const objectFetchManager = createObjectFetchManager(connection);

    // Also create a WidgetClient for backward-compatible low-level operations
    const widgetClient = new WidgetClient(dh, serverUrl);
    widgetClient.client = coreClient;
    widgetClient.connection = connection;

    return new TestClient({
        loader,
        dh,
        connection,
        coreClient,
        objectFetchManager,
        widgetClient,
        dom,
        globals,
        WidgetHandler,
        ReactDOM,
    });
}

/**
 * Quick one-shot render of a widget.
 *
 * @param {string} serverUrl - The Deephaven server URL
 * @param {string} widgetName - Name of the widget to render
 * @param {object} [options]
 * @param {string} [options.widgetType] - Widget type (auto-detected if not specified)
 * @param {number} [options.timeout=10000] - Timeout in ms
 * @returns {Promise<RenderResult>}
 */
export async function renderWidget(serverUrl, widgetName, options = {}) {
    const client = await createTestClient(serverUrl, options);
    try {
        return await client.render(widgetName, options);
    } catch (e) {
        client.close();
        throw e;
    }
}

class TestClient {
    constructor({ loader, dh, connection, coreClient, objectFetchManager,
                  widgetClient, dom, globals, WidgetHandler, ReactDOM }) {
        this._loader = loader;
        this._dh = dh;
        this._connection = connection;
        this._coreClient = coreClient;
        this._objectFetchManager = objectFetchManager;
        this._widgetClient = widgetClient;
        this._dom = dom;
        this._globals = globals;
        this._WidgetHandler = WidgetHandler;
        this._ReactDOM = ReactDOM;
        this._activeResults = [];
    }

    /**
     * Render a widget using the real WidgetHandler pipeline.
     *
     * @param {string} widgetName - Name of the widget
     * @param {object} [options]
     * @param {string} [options.widgetType] - Widget type (auto-detected if not specified)
     * @param {number} [options.timeout=10000] - Timeout in ms
     * @param {function} [options.checkFn] - Custom readiness check: (body) => boolean
     * @returns {Promise<RenderResult>}
     */
    async render(widgetName, options = {}) {
        const { widgetType, timeout = 10000, checkFn } = options;

        // Detect widget type if not specified.
        // Use the render timeout for widget lookup — with overlapped execution
        // the script may still be running when Node.js finishes session.open.
        let type = widgetType;
        if (!type) {
            const definition = await this._widgetClient.fetchVariableDefinition(widgetName, timeout);
            if (definition?.type) {
                type = definition.type;
            }
        }
        if (!type) {
            type = 'deephaven.ui.Element';
        }

        // Classic Figure objects don't implement the JSON-RPC widget protocol
        // that WidgetHandler requires (no getDataAsString). Render them directly
        // using FigureWidgetPlugin, bypassing WidgetHandler entirely.
        if (DIRECT_RENDER_TYPES.has(type)) {
            return this._renderDirect(widgetName, type, options);
        }

        // Create GoldenLayout instance for this render
        const jsdomWindow = this._dom.window;
        const { layout, destroy } = createTestLayout(jsdomWindow);

        const widgetDescriptor = { type, name: widgetName };
        const { act } = React;

        // Create a render container
        const container = jsdomWindow.document.createElement('div');
        container.id = `dh-render-${widgetName}`;
        jsdomWindow.document.body.appendChild(container);
        const root = this._ReactDOM.createRoot(container);

        // Mount WidgetHandler in full provider stack
        await act(async () => {
            root.render(
                h(TestProviderStack, {
                    dh: this._dh,
                    objectFetchManager: this._objectFetchManager,
                    layoutManager: layout,
                },
                    h(this._WidgetHandler, {
                        widgetDescriptor,
                        id: widgetName,
                        onClose: () => {},
                    })
                )
            );
        });

        // Wait for content to render (widget fetch + document patch + React rendering)
        const body = jsdomWindow.document.body;
        const start = Date.now();
        const readyCheck = checkFn || ((b) => {
            // Default: check for real content in panels, and wait for async
            // data to load (figures, table metadata, picker options).
            const panel = b.querySelector('.dh-react-panel');
            if (!panel) return false;
            const hasContent = panel.textContent.length > 0;
            const isLoading = !!b.querySelector('[class*="loading"]');
            const hasLoadingText = b.textContent.includes('[Loading');
            return hasContent && !isLoading && !hasLoadingText;
        });

        while (Date.now() - start < timeout) {
            await act(async () => {
                await new Promise(r => setTimeout(r, 100));
            });
            if (readyCheck(body)) break;
        }

        const result = new RenderResult({
            body,
            container,
            root,
            layout,
            destroy,
            jsdomWindow,
            widgetClient: this._widgetClient,
            widgetName,
            onUnmount: () => {
                const idx = this._activeResults.indexOf(result);
                if (idx >= 0) this._activeResults.splice(idx, 1);
            },
        });

        this._activeResults.push(result);
        return result;
    }

    /**
     * Render a widget directly without WidgetHandler.
     * Used for types like classic Figure that don't implement the JSON-RPC
     * widget protocol. Renders FigureWidgetPlugin (or similar) directly.
     */
    async _renderDirect(widgetName, type, options = {}) {
        const { timeout = 10000, checkFn } = options;
        const jsdomWindow = this._dom.window;
        const { layout, destroy } = createTestLayout(jsdomWindow);
        const { act } = React;

        const container = jsdomWindow.document.createElement('div');
        container.id = `dh-render-${widgetName}`;
        jsdomWindow.document.body.appendChild(container);
        const root = this._ReactDOM.createRoot(container);

        // Build a fetch function that retrieves the object from the server
        const connection = this._connection;
        const fetchFn = async () => connection.getObject({ name: widgetName, type });

        // Pick the right component for the type
        const Component = isFigureType(type) ? FigureWidgetPlugin : FigureWidgetPlugin;

        // Wrap in a .dh-react-panel div so the snapshot builder finds content
        await act(async () => {
            root.render(
                h('div', { className: 'dh-react-panel' },
                    h(Component, { fetch: fetchFn })
                )
            );
        });

        const body = jsdomWindow.document.body;
        const start = Date.now();
        const readyCheck = checkFn || ((b) => {
            const fig = b.querySelector('[role="figure"]');
            if (!fig) return false;
            const isLoading = b.textContent.includes('[Loading');
            return !isLoading;
        });

        while (Date.now() - start < timeout) {
            await act(async () => {
                await new Promise(r => setTimeout(r, 100));
            });
            if (readyCheck(body)) break;
        }

        const result = new RenderResult({
            body,
            container,
            root,
            layout,
            destroy,
            jsdomWindow,
            widgetClient: this._widgetClient,
            widgetName,
            onUnmount: () => {
                const idx = this._activeResults.indexOf(result);
                if (idx >= 0) this._activeResults.splice(idx, 1);
            },
        });

        this._activeResults.push(result);
        return result;
    }

    /** The underlying WidgetClient for low-level operations (table fetching, etc.) */
    get widgetClient() {
        return this._widgetClient;
    }

    /** The dh JSAPI object */
    get dh() {
        return this._dh;
    }

    /** The IdeConnection */
    get connection() {
        return this._connection;
    }

    /** Close the test client and clean up all resources. */
    close() {
        for (const result of this._activeResults) {
            result.unmount();
        }
        this._activeResults = [];
        this._widgetClient.close();
        this._globals.restore();
        this._loader.close();
    }
}

class RenderResult {
    constructor({ body, container, root, layout, destroy,
                  jsdomWindow, widgetClient, widgetName, onUnmount }) {
        this._body = body;
        this._container = container;
        this._root = root;
        this._layout = layout;
        this._destroy = destroy;
        this._jsdomWindow = jsdomWindow;
        this._widgetClient = widgetClient;
        this._widgetName = widgetName;
        this._onUnmount = onUnmount;
    }

    /**
     * The document body — primary query target since ReactPanel portals
     * content into GoldenLayout containers outside the React render root.
     */
    get body() {
        return this._body;
    }

    /** The React render container (may not contain portaled content). */
    get container() {
        return this._container;
    }

    /** The rendered HTML of the full body. */
    get html() {
        return this._body.innerHTML;
    }

    /** The widget name being rendered. */
    get widgetName() {
        return this._widgetName;
    }

    /**
     * Query the full DOM body for an element (includes portaled content).
     * @param {string} selector - CSS selector
     * @returns {Element|null}
     */
    querySelector(selector) {
        return this._body.querySelector(selector);
    }

    /**
     * Query the full DOM body for all matching elements.
     * @param {string} selector - CSS selector
     * @returns {NodeList}
     */
    querySelectorAll(selector) {
        return this._body.querySelectorAll(selector);
    }

    /**
     * Find elements by role attribute.
     * @param {string} role - The ARIA role
     * @returns {Element[]}
     */
    findByRole(role) {
        return Array.from(this._body.querySelectorAll(`[role="${role}"]`));
    }

    /**
     * Find elements containing the given text.
     * @param {string} text - Text to search for
     * @returns {Element[]}
     */
    findByText(text) {
        const elements = [];
        const walker = this._jsdomWindow.document.createTreeWalker(
            this._body, 4 /* NodeFilter.SHOW_TEXT */, null, false
        );
        while (walker.nextNode()) {
            if (walker.currentNode.textContent.includes(text)) {
                elements.push(walker.currentNode.parentElement);
            }
        }
        return elements;
    }

    /**
     * Flush pending React effects (act + setTimeout).
     * Call after DOM interactions (click, input) to process re-renders.
     * @param {number} [ms=100] - Time to wait for effects
     */
    async flush(ms = 100) {
        const { act } = React;
        await act(async () => {
            await new Promise(r => setTimeout(r, ms));
        });
    }

    /**
     * Wait for a condition to become true, flushing React effects periodically.
     * @param {function} checkFn - (body) => boolean
     * @param {number} [timeout=5000] - Timeout in ms
     */
    async waitFor(checkFn, timeout = 5000) {
        const { act } = React;
        const start = Date.now();
        while (Date.now() - start < timeout) {
            await act(async () => {
                await new Promise(r => setTimeout(r, 100));
            });
            if (checkFn(this._body)) return;
        }
        throw new Error(`waitFor timed out after ${timeout}ms`);
    }

    /**
     * Wait for any document update and flush effects.
     * Uses a short polling loop with act() to catch async re-renders.
     * @param {number} [timeout=5000] - Timeout in ms
     */
    async waitForUpdate(timeout = 5000) {
        await this.flush(200);
    }

    /**
     * Unmount the component and clean up.
     */
    unmount() {
        if (this._root) {
            this._root.unmount();
            this._root = null;
        }
        if (this._container) {
            this._container.remove();
            this._container = null;
        }
        if (this._destroy) {
            this._destroy();
            this._destroy = null;
        }
        this._onUnmount?.();
    }

    /**
     * Get a summary of the render for debugging/agent output.
     * @returns {object}
     */
    getSummary() {
        const panels = this._body.querySelectorAll('.dh-react-panel');
        const buttons = this._body.querySelectorAll('button');
        return {
            success: true,
            panelCount: panels.length,
            buttonCount: buttons.length,
            bodyText: this._body.textContent?.substring(0, 500),
            html: this._body.innerHTML,
        };
    }
}
