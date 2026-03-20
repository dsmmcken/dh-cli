/**
 * FigureStub — renders a plotly-express figure as a textual summary.
 *
 * Fetches the NEW_FIGURE payload from the exported object via
 * reexport() → fetch() → getDataAsString(), parses the plotly JSON
 * and deephaven data mappings, and renders a progressive-disclosure
 * summary (collapsed by default, expandable to show data mappings
 * and layout details).
 */
import React from 'react';

const { createElement: h, useState, useEffect } = React;

/** The plugin type string reported by the server for plotly-express figures. */
export const FIGURE_TYPE = 'deephaven.plot.express.DeephavenFigure';

/**
 * Check whether an exported object type string represents a figure.
 * @param {string|null|undefined} type
 * @returns {boolean}
 */
export function isFigureType(type) {
    if (!type) return false;
    return type.includes('Figure');
}

/** Normalize plotly trace type to a short human-readable name. */
function normalizeTraceType(type) {
    const map = {
        scattergl: 'scatter', scatter: 'scatter',
        bar: 'bar', histogram: 'histogram',
        heatmap: 'heatmap', pie: 'pie',
        ohlc: 'OHLC', candlestick: 'candlestick',
        treemap: 'treemap',
    };
    return map[type] || type || 'unknown';
}

/** Get the most common trace type across all traces. */
function primaryTraceType(traces) {
    if (!traces || traces.length === 0) return 'unknown';
    const counts = {};
    for (const t of traces) {
        const type = normalizeTraceType(t.type);
        counts[type] = (counts[type] || 0) + 1;
    }
    return Object.entries(counts).sort((a, b) => b[1] - a[1])[0][0];
}

/**
 * Parse the NEW_FIGURE payload into a structured summary.
 * @param {object} payload - The full JSON payload from getDataAsString()
 */
function parseFigurePayload(payload) {
    const { plotly, deephaven } = payload.figure;
    const traces = plotly?.data || [];
    const layout = plotly?.layout || {};

    const title = layout.title?.text || '';
    const traceType = primaryTraceType(traces);

    // Axes (supports multi-axis: xaxis, xaxis2, yaxis, yaxis2, ...)
    const axes = [];
    for (const key of Object.keys(layout)) {
        const match = key.match(/^(x|y)axis(\d*)$/);
        if (match) {
            const axisLabel = layout[key]?.title?.text;
            if (axisLabel) {
                axes.push({ axis: match[1] + (match[2] || ''), label: axisLabel });
            }
        }
    }

    // Trace summaries
    const traceList = traces.map(t => ({
        type: normalizeTraceType(t.type),
        name: t.name || '',
        mode: t.mode,
        color: t.marker?.color,
    }));

    // Data mappings (from deephaven.mappings)
    const columnMappings = [];
    for (const mapping of (deephaven?.mappings || [])) {
        for (const [colName, paths] of Object.entries(mapping.data_columns || {})) {
            if (colName.startsWith('tmp')) continue;
            const props = paths.map(p => p.split('/').pop());
            const uniqueProps = [...new Set(props)];
            columnMappings.push({ column: colName, properties: uniqueProps });
        }
    }
    // Deduplicate mappings (same column may appear in multiple table mappings)
    const seenMappings = new Set();
    const dedupedMappings = columnMappings.filter(m => {
        const key = `${m.column}:${m.properties.join(',')}`;
        if (seenMappings.has(key)) return false;
        seenMappings.add(key);
        return true;
    });

    // Layout properties worth showing (skip defaults/noise)
    const layoutProps = {};
    if (layout.barmode) layoutProps.barmode = layout.barmode;
    if (layout.bargap !== undefined && layout.bargap !== 0.2) layoutProps.bargap = layout.bargap;
    if (layout.showlegend === false) layoutProps.showlegend = false;

    return {
        title,
        traceType,
        traceCount: traces.length,
        axes,
        traceList,
        columnMappings: dedupedMappings,
        layoutProps,
    };
}

/**
 * Fetch and parse figure data from an exported object.
 * @param {object} exportedObject
 */
async function fetchFigureData(exportedObject) {
    const reexported = await exportedObject.reexport();
    const widget = await reexported.fetch();
    const payload = JSON.parse(widget.getDataAsString());
    return parseFigurePayload(payload);
}

/**
 * FigureStub React component.
 *
 * @param {{ objectId: number, exportedObject: object }} props
 */
export function FigureStub({ objectId, exportedObject }) {
    const [expanded, setExpanded] = useState(false);
    const [data, setData] = useState(null);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState(null);

    useEffect(() => {
        if (!exportedObject || typeof exportedObject.reexport !== 'function') {
            setLoading(false);
            return;
        }
        let cancelled = false;
        fetchFigureData(exportedObject)
            .then(d => { if (!cancelled) setData(d); })
            .catch(e => { if (!cancelled) setError(e); })
            .finally(() => { if (!cancelled) setLoading(false); });
        return () => { cancelled = true; };
    }, [exportedObject]);

    const attrs = {
        role: 'figure',
        'data-component': 'dh.Figure',
        'data-object-id': String(objectId),
    };
    if (data) {
        attrs['data-figure-type'] = data.traceType;
        attrs['data-trace-count'] = String(data.traceCount);
    }

    const children = [];

    if (loading) {
        children.push(h('span', { 'data-testid': 'figure-loading', key: 'loading' }, '[Loading figure data...]'));
    }

    if (error) {
        children.push(h('span', { 'data-testid': 'figure-error', key: 'error' }, `Error: ${error.message}`));
    }

    if (data) {
        if (data.title) {
            children.push(h('div', { 'data-testid': 'figure-title', key: 'title' }, data.title));
        }

        if (data.axes.length > 0) {
            children.push(h('div', { 'data-testid': 'figure-axes', 'aria-label': 'axes', key: 'axes' },
                ...data.axes.map((a, i) =>
                    h('span', { key: i, 'data-testid': 'figure-axis' }, `${a.axis}: ${a.label}`)
                )
            ));
        }

        const namedTraces = data.traceList.filter(t => t.name);
        if (namedTraces.length > 0) {
            children.push(h('div', { 'data-testid': 'figure-traces', 'aria-label': 'traces', key: 'traces' },
                ...namedTraces.map((t, i) =>
                    h('span', { key: i, 'data-testid': 'figure-trace' }, `${t.type} "${t.name}"`)
                )
            ));
        }

        // Expanded content
        if (expanded && data.columnMappings.length > 0) {
            children.push(h('div', { 'data-testid': 'figure-mappings', 'aria-label': 'data mappings', key: 'mappings' },
                ...data.columnMappings.map((m, i) =>
                    h('span', { key: i, 'data-testid': 'figure-mapping' },
                        `${m.column} → ${m.properties.join(', ')}`)
                )
            ));
        }

        if (expanded && Object.keys(data.layoutProps).length > 0) {
            children.push(h('div', { 'data-testid': 'figure-layout', 'aria-label': 'layout', key: 'layout' },
                ...Object.entries(data.layoutProps).map(([k, v], i) =>
                    h('span', { key: i, 'data-testid': 'figure-prop' }, `${k}: ${v}`)
                )
            ));
        }

        children.push(h('button', {
            'data-testid': 'figure-expand',
            key: 'expand',
            onClick: () => setExpanded(e => !e),
        }, expanded ? 'Collapse figure' : 'Expand figure'));
    }

    return h('div', attrs, ...children);
}

FigureStub.displayName = 'FigureStub';

/**
 * FigureWidgetPlugin — WidgetPlugin component for the CLI/WidgetHandler path.
 *
 * Receives a `fetch` prop (from WidgetView) that returns the widget directly.
 * Calls fetch(), parses the figure payload, and renders the same UI as FigureStub.
 *
 * @param {{ fetch: () => Promise<object> }} props
 */
export function FigureWidgetPlugin({ fetch: fetchWidget }) {
    const [expanded, setExpanded] = useState(false);
    const [data, setData] = useState(null);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState(null);

    useEffect(() => {
        if (typeof fetchWidget !== 'function') {
            setLoading(false);
            return;
        }
        let cancelled = false;
        fetchWidget()
            .then(widget => {
                const payload = JSON.parse(widget.getDataAsString());
                return parseFigurePayload(payload);
            })
            .then(d => { if (!cancelled) setData(d); })
            .catch(e => { if (!cancelled) setError(e); })
            .finally(() => { if (!cancelled) setLoading(false); });
        return () => { cancelled = true; };
    }, [fetchWidget]);

    const attrs = {
        role: 'figure',
        'data-component': 'dh.Figure',
    };
    if (data) {
        attrs['data-figure-type'] = data.traceType;
        attrs['data-trace-count'] = String(data.traceCount);
    }

    const children = [];

    if (loading) {
        children.push(h('span', { 'data-testid': 'figure-loading', key: 'loading' }, '[Loading figure data...]'));
    }

    if (error) {
        children.push(h('span', { 'data-testid': 'figure-error', key: 'error' }, `Error: ${error.message}`));
    }

    if (data) {
        if (data.title) {
            children.push(h('div', { 'data-testid': 'figure-title', key: 'title' }, data.title));
        }

        if (data.axes.length > 0) {
            children.push(h('div', { 'data-testid': 'figure-axes', 'aria-label': 'axes', key: 'axes' },
                ...data.axes.map((a, i) =>
                    h('span', { key: i, 'data-testid': 'figure-axis' }, `${a.axis}: ${a.label}`)
                )
            ));
        }

        const namedTraces = data.traceList.filter(t => t.name);
        if (namedTraces.length > 0) {
            children.push(h('div', { 'data-testid': 'figure-traces', 'aria-label': 'traces', key: 'traces' },
                ...namedTraces.map((t, i) =>
                    h('span', { key: i, 'data-testid': 'figure-trace' }, `${t.type} "${t.name}"`)
                )
            ));
        }

        if (expanded && data.columnMappings.length > 0) {
            children.push(h('div', { 'data-testid': 'figure-mappings', 'aria-label': 'data mappings', key: 'mappings' },
                ...data.columnMappings.map((m, i) =>
                    h('span', { key: i, 'data-testid': 'figure-mapping' },
                        `${m.column} → ${m.properties.join(', ')}`)
                )
            ));
        }

        if (expanded && Object.keys(data.layoutProps).length > 0) {
            children.push(h('div', { 'data-testid': 'figure-layout', 'aria-label': 'layout', key: 'layout' },
                ...Object.entries(data.layoutProps).map(([k, v], i) =>
                    h('span', { key: i, 'data-testid': 'figure-prop' }, `${k}: ${v}`)
                )
            ));
        }

        children.push(h('button', {
            'data-testid': 'figure-expand',
            key: 'expand',
            onClick: () => setExpanded(e => !e),
        }, expanded ? 'Collapse figure' : 'Expand figure'));
    }

    return h('div', attrs, ...children);
}

FigureWidgetPlugin.displayName = 'FigureWidgetPlugin';

/**
 * Creates a PluginModuleMap entry for the Figure stub as a WidgetPlugin.
 * @returns {Map<string, object>}
 */
export function createFigureStubPluginMap() {
    return new Map([
        ['dh-render-test-figure-stub', {
            name: 'dh-render-test-figure-stub',
            type: 'WidgetPlugin',
            supportedTypes: FIGURE_TYPE,
            component: FigureWidgetPlugin,
        }],
    ]);
}
