/**
 * TestProviderStack — wraps children in the full Deephaven provider hierarchy
 * so that real UI components from @deephaven/js-plugin-ui render correctly in jsdom.
 *
 * Provider chain (outermost → innermost):
 *   ReduxProvider → ThemeProvider → PluginsContext → LayoutManagerContext → PortalPanelManager → ApiContext/ObjectFetchManager → children
 */
import React from 'react';
import { createUITableStubPluginMap, createTableWidgetPluginMap } from './UITableStub.mjs';
import { createFigureStubPluginMap } from './FigureStub.mjs';
import { LayoutReactChildren } from './GoldenLayoutSetup.mjs';

const { createElement: h } = React;

// Lazy-loaded providers (to avoid import-order issues with jsdom globals)
let _ThemeProvider = null;
let _PluginsContext = null;
let _ApiContext = null;
let _ObjectFetchManagerContext = null;
let _LayoutManagerContext = null;
let _PortalPanelManager = null;
let _ReduxProvider = null;
let _reduxStore = null;

/**
 * Load the provider components. Must be called after jsdom globals are installed.
 * Imports are parallelized in dependency-safe groups to avoid the Node.js ESM
 * race condition (modules that share the same dependency graph can't be
 * Promise.all'd together on Node 24+).
 */
export async function loadProviders() {
    if (_ThemeProvider) return;

    // Group 1: independent leaf packages (no shared transitive deps)
    const [components, plugin, bootstrap, reactRedux, redux] = await Promise.all([
        import('@deephaven/components'),
        import('@deephaven/plugin'),
        import('@deephaven/jsapi-bootstrap'),
        import('react-redux'),
        import('redux'),
    ]);

    _ThemeProvider = components.ThemeProvider;
    _PluginsContext = plugin.PluginsContext;
    _ApiContext = bootstrap.ApiContext;
    _ObjectFetchManagerContext = bootstrap.ObjectFetchManagerContext;
    _ReduxProvider = reactRedux.Provider;

    _reduxStore = redux.createStore(() => ({
        workspace: { data: { settings: {} } },
        defaultWorkspaceSettings: {},
        storage: {
            workspaceStorage: null,
            commandHistoryStorage: null,
            fileStorage: null,
        },
        user: { name: 'test', groups: [] },
        activeTool: 'default',
        plugins: new Map(),
        api: null,
        dashboardData: {},
        serverConfigValues: {},
    }));

    // Group 2: @deephaven/dashboard (depends on @deephaven/components)
    const dashboard = await import('@deephaven/dashboard');
    _LayoutManagerContext = dashboard.LayoutManagerContext;

    // Group 3: @deephaven/js-plugin-ui (depends on dashboard + components)
    const pluginUi = await import('@deephaven/js-plugin-ui');
    _PortalPanelManager = pluginUi.PortalPanelManager;
}

/**
 * Default plugin map with UITable stub override.
 */
const DEFAULT_PLUGIN_MAP = new Map([
    ...createUITableStubPluginMap(),
    ...createTableWidgetPluginMap(),
    ...createFigureStubPluginMap(),
]);

/**
 * TestProviderStack component.
 *
 * @param {object} props
 * @param {React.ReactNode} props.children
 * @param {Map} [props.pluginMap] - Plugin map (default includes UITable stub)
 * @param {object} [props.dh] - Deephaven JSAPI object for ApiContext
 * @param {object} [props.objectFetchManager] - ObjectFetchManager for widget fetching
 * @param {object} [props.layoutManager] - GoldenLayout instance for LayoutManagerContext
 */
export function TestProviderStack({
    children,
    pluginMap = DEFAULT_PLUGIN_MAP,
    dh = null,
    objectFetchManager = null,
    layoutManager = undefined,
}) {
    if (!_ThemeProvider) {
        throw new Error('Providers not loaded. Call loadProviders() first.');
    }

    let inner = children;

    // ObjectFetchManager (Phase 2)
    if (_ObjectFetchManagerContext && objectFetchManager) {
        inner = h(_ObjectFetchManagerContext.Provider, { value: objectFetchManager }, inner);
    }

    // ApiContext (Phase 2)
    if (_ApiContext && dh) {
        inner = h(_ApiContext.Provider, { value: dh }, inner);
    }

    // LayoutManagerContext + PortalPanelManager + LayoutReactChildren (Phase 3/5)
    // PortalPanelManager must be inside LayoutManagerContext since it uses useLayoutManager()
    // LayoutReactChildren renders the GoldenLayout React portals in the React tree
    if (_LayoutManagerContext && layoutManager !== undefined) {
        if (_PortalPanelManager) {
            inner = h(_PortalPanelManager, null,
                inner,
                h(LayoutReactChildren, { layout: layoutManager })
            );
        }
        inner = h(_LayoutManagerContext.Provider, { value: layoutManager }, inner);
    }

    let tree = h(_ThemeProvider, { themes: [] },
        h(_PluginsContext.Provider, { value: pluginMap },
            inner
        )
    );

    // Redux Provider (outermost) — supplies store for useSelector(getSettings) etc.
    if (_ReduxProvider && _reduxStore) {
        tree = h(_ReduxProvider, { store: _reduxStore }, tree);
    }

    return tree;
}
