/**
 * FigureStub — renders both plotly-express (dx) and classic
 * deephaven.plot.figure.Figure objects as textual summaries.
 *
 * dx figures:      reexport() → fetch() → getDataAsString() → plotly JSON
 * classic figures:  reexport() → fetch() → dh.plot.Figure (has .charts/.series)
 *
 * Both paths produce the same { title, traceType, traceCount, axes, traceList, … }
 * shape and render through the shared FigureDataView component.
 */
import React from 'react';

const { createElement: h, useState, useEffect } = React;

/** The plugin type string reported by the server for plotly-express figures. */
export const FIGURE_TYPE = 'deephaven.plot.express.DeephavenFigure';

/** The type string for classic deephaven.plot.figure.Figure objects. */
export const CLASSIC_FIGURE_TYPE = 'Figure';

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
        api: 'express',
        title,
        traceType,
        traceCount: traces.length,
        axes,
        traceList,
        columnMappings: dedupedMappings,
        layoutProps,
    };
}

/** Map classic Figure plotStyle enum values to short display names.
 *  The JSAPI may return numeric enum ordinals OR string names. */
function normalizeClassicPlotStyle(plotStyle) {
    if (plotStyle === null || plotStyle === undefined) return 'unknown';
    // Numeric enum ordinals (PlotStyle Java enum order)
    const byOrdinal = {
        0: 'bar', 1: 'stacked bar', 2: 'line', 3: 'area',
        4: 'stacked area', 5: 'pie', 6: 'histogram', 7: 'OHLC',
        8: 'scatter', 9: 'step', 10: 'error bar', 11: 'treemap',
    };
    if (typeof plotStyle === 'number' || /^\d+$/.test(String(plotStyle))) {
        return byOrdinal[Number(plotStyle)] || `style-${plotStyle}`;
    }
    // String names
    const s = String(plotStyle).toUpperCase();
    const byName = {
        LINE: 'line', BAR: 'bar', SCATTER: 'scatter',
        AREA: 'area', STACKED_AREA: 'stacked area',
        STEP: 'step', PIE: 'pie', HISTOGRAM: 'histogram',
        OHLC: 'OHLC', TREEMAP: 'treemap', ERROR_BAR: 'error bar',
        STACKED_BAR: 'stacked bar',
    };
    return byName[s] || s.toLowerCase();
}

/**
 * Parse a classic dh.plot.Figure object into the same summary shape
 * that parseFigurePayload() returns for dx figures.
 * @param {object} figure - A dh.plot.Figure instance (has .charts, .title)
 */
function parseClassicFigure(figure) {
    const title = figure.title || '';
    const charts = figure.charts || [];

    const axes = [];
    const traceList = [];

    for (const chart of charts) {
        // Collect axes — axis.type may be a string ('X','Y') or numeric enum
        for (const axis of (chart.axes || [])) {
            const label = axis.label;
            if (label) {
                let axisType = axis.type;
                if (typeof axisType === 'number') {
                    axisType = axisType === 0 ? 'x' : 'y';
                } else {
                    axisType = String(axisType || 'x').toLowerCase();
                }
                axes.push({ axis: axisType, label });
            }
        }
        // Collect series as traces
        for (const series of (chart.series || [])) {
            traceList.push({
                type: normalizeClassicPlotStyle(series.plotStyle),
                name: series.name || '',
                mode: undefined,
                color: undefined,
            });
        }
    }

    // Primary trace type = most common plotStyle
    const typeCounts = {};
    for (const t of traceList) {
        typeCounts[t.type] = (typeCounts[t.type] || 0) + 1;
    }
    const traceType = Object.entries(typeCounts).sort((a, b) => b[1] - a[1])[0]?.[0] || 'unknown';

    return {
        api: 'classic',
        title,
        traceType,
        traceCount: traceList.length,
        axes,
        traceList,
        columnMappings: [],
        layoutProps: {},
    };
}

/**
 * Resolve a fetched widget to figure summary data.
 * Handles both dx figures (plotly JSON via getDataAsString) and classic
 * dh.plot.Figure objects (have .charts property).
 * @param {object} widget - The fetched widget/figure object
 */
function resolveWidgetToFigureData(widget) {
    // dx / plotly-express path
    if (typeof widget.getDataAsString === 'function') {
        const payload = JSON.parse(widget.getDataAsString());
        return parseFigurePayload(payload);
    }
    // Classic dh.plot.Figure path
    if (widget.charts) {
        return parseClassicFigure(widget);
    }
    throw new Error('Unknown figure format');
}

/**
 * Fetch and parse figure data from an exported object.
 * Works for both dx and classic figures.
 * @param {object} exportedObject
 */
async function fetchFigureData(exportedObject) {
    const reexported = await exportedObject.reexport();
    const widget = await reexported.fetch();
    return resolveWidgetToFigureData(widget);
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
        attrs['data-figure-api'] = data.api;
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
            .then(widget => resolveWidgetToFigureData(widget))
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
        attrs['data-figure-api'] = data.api;
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
 * Creates PluginModuleMap entries for both dx and classic figure types.
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
        ['dh-render-test-classic-figure-stub', {
            name: 'dh-render-test-classic-figure-stub',
            type: 'WidgetPlugin',
            supportedTypes: CLASSIC_FIGURE_TYPE,
            component: FigureWidgetPlugin,
        }],
    ]);
}
