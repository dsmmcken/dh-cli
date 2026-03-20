/**
 * UITable stub component — used as an ElementPlugin override to avoid loading
 * iris-grid and its heavy dependencies in the jsdom test environment.
 *
 * Renders table metadata, config props, and (when expanded) column headers
 * and sample data rows. Progressive disclosure: collapsed by default with
 * an "Expand table" button the agent can click to reveal data.
 */
import React from 'react';
import { createRequire } from 'node:module';

const { createElement: h, useState, useEffect } = React;

const DEFAULT_MAX_PREVIEW_ROWS = 5;

// Default prop values for ui.table, extracted from the DH Python source via
// scripts/extract-ui-table-defaults.py. Props matching these defaults are
// hidden from the config display (only user-specified overrides are shown).
const require = createRequire(import.meta.url);
const UI_TABLE_DEFAULTS = require('./ui-table-defaults.json');

function formatCellValue(value) {
    if (value === null || value === undefined) return 'null';
    if (value instanceof Date) return value.toISOString();
    return String(value);
}

function renderConfigProps(rest) {
    const spans = [];
    const handlers = [];

    for (const [key, value] of Object.entries(rest)) {
        if (key.startsWith('__')) continue;
        // Skip props that match the ui.table default values
        if (key in UI_TABLE_DEFAULTS && value === UI_TABLE_DEFAULTS[key]) continue;
        if (key.startsWith('on') && typeof value === 'function') {
            handlers.push(key);
        } else if (Array.isArray(value)) {
            if (value.length > 0 && typeof value[0] === 'object') {
                spans.push(h('span', { 'data-testid': 'table-prop', key }, `${key}: [${value.length} items]`));
            } else {
                spans.push(h('span', { 'data-testid': 'table-prop', key }, `${key}: ${value.join(', ')}`));
            }
        } else if (typeof value === 'object' && value !== null) {
            const len = Object.keys(value).length;
            spans.push(h('span', { 'data-testid': 'table-prop', key }, `${key}: [${len} items]`));
        } else if (typeof value === 'string' || typeof value === 'number' || typeof value === 'boolean') {
            spans.push(h('span', { 'data-testid': 'table-prop', key }, `${key}: ${value}`));
        }
    }

    if (handlers.length > 0) {
        spans.push(h('span', { 'data-testid': 'table-prop', key: '_handlers' }, `handlers: ${handlers.join(', ')}`));
    }

    return spans;
}

export function UITableStub(props) {
    const { table, children, ...rest } = props || {};
    const [expanded, setExpanded] = useState(false);
    const [state, setState] = useState({
        columns: null,
        rowCount: null,
        sampleRows: null,
        error: null,
    });

    useEffect(() => {
        if (!table || typeof table.reexport !== 'function') return;
        let cancelled = false;

        (async () => {
            try {
                const reexported = await table.reexport();
                const fetched = await reexported.fetch();
                if (cancelled) return;

                const fetchedColumns = fetched.columns;
                const columns = fetchedColumns.map(c => ({ name: c.name, type: c.type }));
                const rowCount = fetched.size;

                let sampleRows = [];
                if (rowCount > 0) {
                    const end = Math.min(DEFAULT_MAX_PREVIEW_ROWS, rowCount);
                    fetched.setViewport(0, end - 1);
                    const viewportData = await fetched.getViewportData();
                    if (cancelled) return;
                    sampleRows = viewportData.rows.map(row =>
                        fetchedColumns.map(col => formatCellValue(row.get(col)))
                    );
                }

                setState({ columns, rowCount, sampleRows, error: null });
            } catch (err) {
                if (!cancelled) {
                    setState(prev => ({ ...prev, error: err.message || String(err) }));
                }
            }
        })();

        return () => { cancelled = true; };
    }, [table]);

    const tableType = table?.type || 'unknown';
    const { columns, rowCount, sampleRows, error } = state;

    // Backward-compat: simple props as data-* attributes on root
    const dataProps = {};
    for (const [key, value] of Object.entries(rest)) {
        if (key.startsWith('__')) continue;
        if (key in UI_TABLE_DEFAULTS && value === UI_TABLE_DEFAULTS[key]) continue;
        if (typeof value === 'string' || typeof value === 'number' || typeof value === 'boolean') {
            dataProps[`data-${key.toLowerCase()}`] = String(value);
        }
    }

    // Config props
    const configSpans = renderConfigProps(rest);

    // Root attributes
    const rootAttrs = {
        'data-component': 'deephaven.ui.elements.UITable',
        'data-table-type': tableType,
        role: 'table',
        ...dataProps,
    };
    if (rowCount !== null) {
        rootAttrs['data-row-count'] = String(rowCount);
        rootAttrs['data-column-count'] = String(columns.length);
    }

    return h('div', rootAttrs,
        configSpans.length > 0
            ? h('div', { 'data-testid': 'table-config', 'aria-label': 'table configuration' }, ...configSpans)
            : null,
        expanded && columns
            ? h('div', { role: 'row', 'data-testid': 'table-header' },
                ...columns.map(col => h('span', { role: 'columnheader', key: col.name }, `${col.name} (${col.type})`))
              )
            : null,
        expanded && sampleRows
            ? sampleRows.map((row, i) =>
                h('div', { role: 'row', key: `row-${i}` },
                    ...row.map((cell, j) => h('span', { role: 'cell', key: j }, cell))
                )
              )
            : null,
        expanded && !columns && !error
            ? h('span', { 'data-testid': 'table-loading' }, '[Loading table data...]')
            : null,
        error ? h('span', { 'data-testid': 'table-error' }, `Error: ${error}`) : null,
        table ? h('button', {
            'data-testid': 'table-expand',
            onClick: () => setExpanded(e => !e),
        }, expanded ? 'Collapse table' : 'Expand table') : null,
        children
    );
}

UITableStub.displayName = 'UITableStub';

/**
 * TableWidgetPlugin — WidgetPlugin component for raw Table exported objects.
 *
 * When a raw Table (not wrapped in ui.table()) appears as a child in the
 * document tree, the real DH ObjectView looks for a WidgetPlugin with
 * supportedTypes including "Table". This component receives a `fetch` prop
 * that returns the table directly, and renders the same metadata UI as UITableStub.
 *
 * @param {{ fetch: () => Promise<object> }} props
 */
export function TableWidgetPlugin({ fetch: fetchTable }) {
    const [expanded, setExpanded] = useState(false);
    const [state, setState] = useState({
        columns: null,
        rowCount: null,
        sampleRows: null,
        error: null,
    });

    useEffect(() => {
        if (typeof fetchTable !== 'function') return;
        let cancelled = false;

        (async () => {
            try {
                const fetched = await fetchTable();
                if (cancelled) return;

                const fetchedColumns = fetched.columns;
                const columns = fetchedColumns.map(c => ({ name: c.name, type: c.type }));
                const rowCount = fetched.size;

                let sampleRows = [];
                if (rowCount > 0) {
                    const end = Math.min(DEFAULT_MAX_PREVIEW_ROWS, rowCount);
                    fetched.setViewport(0, end - 1);
                    const viewportData = await fetched.getViewportData();
                    if (cancelled) return;
                    sampleRows = viewportData.rows.map(row =>
                        fetchedColumns.map(col => formatCellValue(row.get(col)))
                    );
                }

                setState({ columns, rowCount, sampleRows, error: null });
            } catch (err) {
                if (!cancelled) {
                    setState(prev => ({ ...prev, error: err.message || String(err) }));
                }
            }
        })();

        return () => { cancelled = true; };
    }, [fetchTable]);

    const { columns, rowCount, sampleRows, error } = state;

    const rootAttrs = {
        'data-component': 'dh.Table',
        role: 'table',
    };
    if (rowCount !== null) {
        rootAttrs['data-row-count'] = String(rowCount);
        rootAttrs['data-column-count'] = String(columns.length);
    }

    return h('div', rootAttrs,
        expanded && columns
            ? h('div', { role: 'row', 'data-testid': 'table-header' },
                ...columns.map(col => h('span', { role: 'columnheader', key: col.name }, `${col.name} (${col.type})`))
              )
            : null,
        expanded && sampleRows
            ? sampleRows.map((row, i) =>
                h('div', { role: 'row', key: `row-${i}` },
                    ...row.map((cell, j) => h('span', { role: 'cell', key: j }, cell))
                )
              )
            : null,
        expanded && !columns && !error
            ? h('span', { 'data-testid': 'table-loading' }, '[Loading table data...]')
            : null,
        error ? h('span', { 'data-testid': 'table-error' }, `Error: ${error}`) : null,
        h('button', {
            'data-testid': 'table-expand',
            onClick: () => setExpanded(e => !e),
        }, expanded ? 'Collapse table' : 'Expand table')
    );
}

TableWidgetPlugin.displayName = 'TableWidgetPlugin';

/**
 * The element name that maps to UITable in the DH element system.
 */
export const UI_TABLE_ELEMENT_NAME = 'deephaven.ui.elements.UITable';

/**
 * Creates a PluginModuleMap containing the UITable stub override.
 * Merge this with any other plugin maps before passing to PluginsContext.
 *
 * @returns {Map<string, object>} PluginModuleMap with the UITable override
 */
export function createUITableStubPluginMap() {
    return new Map([
        ['dh-render-test-ui-table-stub', {
            name: 'dh-render-test-ui-table-stub',
            type: 'ElementPlugin',
            mapping: {
                [UI_TABLE_ELEMENT_NAME]: UITableStub,
            },
        }],
    ]);
}

/**
 * Creates a PluginModuleMap entry for the Table stub as a WidgetPlugin.
 * This handles raw Table exported objects rendered via ObjectView.
 * @returns {Map<string, object>}
 */
export function createTableWidgetPluginMap() {
    return new Map([
        ['dh-render-test-table-widget', {
            name: 'dh-render-test-table-widget',
            type: 'WidgetPlugin',
            supportedTypes: 'Table',
            component: TableWidgetPlugin,
        }],
    ]);
}
