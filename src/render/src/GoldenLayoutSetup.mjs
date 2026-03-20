/**
 * GoldenLayoutSetup — creates and configures a GoldenLayout instance in jsdom
 * for testing Deephaven panel/dashboard components.
 *
 * Handles:
 * - jQuery global installation (required by GoldenLayout)
 * - Container element creation with stubbed dimensions
 * - PortalPanel component registration (React-compatible)
 * - LayoutReactChildren component for rendering GoldenLayout portals
 */

import React from 'react';

let _LayoutManager = null;
let _jQuery = null;

/**
 * Load GoldenLayout dependencies. Must be called after jsdom globals are installed.
 */
export async function loadGoldenLayout() {
    if (_LayoutManager) return;

    const jqueryMod = await import('jquery');
    _jQuery = jqueryMod.default || jqueryMod;
    globalThis.jQuery = _jQuery;
    globalThis.$ = _jQuery;

    const gl = await import('@deephaven/golden-layout');
    _LayoutManager = gl.LayoutManager;
}

const PORTAL_PANEL_NAME = '@deephaven/js-plugin-ui/PortalPanel';
const PORTAL_OPENED = 'PortalPanelEvent.PORTAL_OPENED';
const PORTAL_CLOSED = 'PortalPanelEvent.PORTAL_CLOSED';

/**
 * A React component registered with GoldenLayout as PortalPanel.
 * When rendered by GoldenLayout's ReactComponentHandler, it creates a
 * portal target div and emits events for PortalPanelManager.
 */
const TestPortalPanel = React.forwardRef(function TestPortalPanel(
    { glContainer, glEventHub },
    ref
) {
    const divRef = React.useRef(null);

    React.useEffect(() => {
        const el = divRef.current;
        if (el && glEventHub) {
            glEventHub.emit(PORTAL_OPENED, { container: glContainer, element: el });
        }
        return () => {
            if (glEventHub) {
                glEventHub.emit(PORTAL_CLOSED, { container: glContainer });
            }
        };
    }, [glContainer, glEventHub]);

    return React.createElement('div', { className: 'ui-portal-panel', ref: divRef });
});

TestPortalPanel.displayName = PORTAL_PANEL_NAME;

/**
 * Create a GoldenLayout instance configured for jsdom testing.
 *
 * @param {Window} jsdomWindow - The jsdom window object
 * @param {object} [options]
 * @param {number} [options.width=1024] - Simulated container width
 * @param {number} [options.height=768] - Simulated container height
 * @returns {{ layout: GoldenLayout, container: HTMLElement, portalPanelMap: Map, destroy: () => void }}
 */
export function createTestLayout(jsdomWindow, options = {}) {
    if (!_LayoutManager) {
        throw new Error('GoldenLayout not loaded. Call loadGoldenLayout() first.');
    }

    const { width = 1024, height = 768 } = options;

    // Create a container element with stubbed dimensions
    const container = jsdomWindow.document.createElement('div');
    container.id = 'dh-test-layout';
    jsdomWindow.document.body.appendChild(container);

    // Stub dimension APIs (jsdom returns 0 for everything)
    container.getBoundingClientRect = () => ({
        x: 0, y: 0, width, height,
        top: 0, right: width, bottom: height, left: 0,
    });
    Object.defineProperty(container, 'offsetWidth', { get: () => width });
    Object.defineProperty(container, 'offsetHeight', { get: () => height });

    // Create GoldenLayout
    const layout = new _LayoutManager({ content: [] }, container);

    // Register PortalPanel as a React component with GoldenLayout.
    // ReactComponentHandler will create portals into this component and
    // add them to the layout's react children list.
    layout.registerComponent(PORTAL_PANEL_NAME, TestPortalPanel);

    const portalPanelMap = new Map();

    layout.init();

    return {
        layout,
        container,
        portalPanelMap,
        destroy() {
            try { layout.destroy(); } catch (e) { /* ignore */ }
            container.remove();
            portalPanelMap.clear();
        },
    };
}

/**
 * React component that renders GoldenLayout's React children (portals).
 * Must be mounted in the React tree so that GoldenLayout's ReactComponentHandler
 * portals are actually rendered.
 *
 * @param {{ layout: GoldenLayout }} props
 */
export function LayoutReactChildren({ layout }) {
    const [children, setChildren] = React.useState(
        () => layout.getReactChildren() ?? []
    );

    React.useEffect(() => {
        const handler = () => {
            setChildren([...(layout.getReactChildren() ?? [])]);
        };
        layout.on('reactChildrenChanged', handler);
        // Catch up if we missed events before the handler was registered
        const current = layout.getReactChildren() ?? [];
        if (current.length > 0) {
            setChildren([...current]);
        }
        return () => layout.off('reactChildrenChanged', handler);
    }, [layout]);

    return React.createElement(React.Fragment, null, ...children);
}
