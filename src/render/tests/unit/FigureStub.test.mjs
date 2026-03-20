import { describe, it, expect } from 'vitest';
import { FigureStub, FIGURE_TYPE, isFigureType } from '../../src/FigureStub.mjs';
import { renderAndWait, renderWithAct } from '../helpers/render-utils.mjs';
import { createMockExportedFigure } from '../helpers/mock-documents.mjs';

const React = (await import('react')).default;
const { createElement: h } = React;

// --- Helpers ---

/** Scatter payload matching real server output structure */
const SCATTER_PAYLOAD = {
    type: 'NEW_FIGURE',
    figure: {
        plotly: {
            data: [
                {
                    type: 'scattergl',
                    name: 'alpha',
                    mode: 'markers',
                    marker: { color: '#636EFA', symbol: 'circle' },
                    hovertemplate: 'Category=alpha<br>X=%{x}<br>Y=%{y}<extra></extra>',
                    xaxis: 'x',
                    yaxis: 'y',
                },
                {
                    type: 'scattergl',
                    name: 'beta',
                    mode: 'markers',
                    marker: { color: '#EF553B', symbol: 'circle' },
                    hovertemplate: 'Category=beta<br>X=%{x}<br>Y=%{y}<extra></extra>',
                    xaxis: 'x',
                    yaxis: 'y',
                },
            ],
            layout: {
                title: { text: 'Square Root' },
                xaxis: { title: { text: 'X' }, anchor: 'y', domain: [0, 1] },
                yaxis: { title: { text: 'Y' }, anchor: 'x', domain: [0, 1] },
                legend: { title: { text: 'Category' }, tracegroupgap: 0 },
            },
        },
        deephaven: {
            mappings: [
                { table: 0, data_columns: { X: ['/plotly/data/0/x'], Y: ['/plotly/data/0/y'] } },
                { table: 1, data_columns: { X: ['/plotly/data/1/x'], Y: ['/plotly/data/1/y'] } },
            ],
            is_user_set_template: false,
            is_user_set_color: true,
        },
    },
    revision: 0,
    new_references: [0, 1],
    removed_references: [],
};

/** Histogram payload matching real server output */
const HISTOGRAM_PAYLOAD = {
    type: 'NEW_FIGURE',
    figure: {
        plotly: {
            data: [
                {
                    type: 'bar',
                    name: '',
                    orientation: 'v',
                    hovertemplate: 'Y=%{x}<br>count=%{y}<extra></extra>',
                    marker: { color: '#636efa' },
                    xaxis: 'x',
                    yaxis: 'y',
                },
            ],
            layout: {
                title: { text: 'Y Distribution' },
                xaxis: { title: { text: 'Y' } },
                yaxis: { title: { text: 'count' } },
                barmode: 'group',
                bargap: 0,
                showlegend: false,
            },
        },
        deephaven: {
            mappings: [
                { table: 0, data_columns: { Y: ['/plotly/data/0/x'], tmpbar0: ['/plotly/data/0/y'] } },
            ],
            is_user_set_template: false,
            is_user_set_color: false,
        },
    },
    revision: 0,
    new_references: [0],
    removed_references: [],
};

// --- Type helpers ---

describe('FIGURE_TYPE', () => {
    it('equals "deephaven.plot.express.DeephavenFigure"', () => {
        expect(FIGURE_TYPE).toBe('deephaven.plot.express.DeephavenFigure');
    });
});

describe('isFigureType', () => {
    it('returns true for DeephavenFigure type', () => {
        expect(isFigureType('deephaven.plot.express.DeephavenFigure')).toBe(true);
    });

    it('returns true for plain "Figure"', () => {
        expect(isFigureType('Figure')).toBe(true);
    });

    it('returns false for "Table"', () => {
        expect(isFigureType('Table')).toBe(false);
    });

    it('returns false for null/undefined', () => {
        expect(isFigureType(null)).toBe(false);
        expect(isFigureType(undefined)).toBe(false);
    });
});

// --- FigureStub rendering ---

describe('FigureStub', () => {
    it('renders with role="figure"', async () => {
        const mock = createMockExportedFigure(SCATTER_PAYLOAD);
        const { container, cleanup } = await renderWithAct(
            h(FigureStub, { objectId: 0, exportedObject: mock }),
            { checkFn: (body) => body.querySelector('[data-figure-type]') !== null }
        );
        const el = container.querySelector('[role="figure"]');
        expect(el).not.toBeNull();
        cleanup();
    });

    it('renders with data-component="dh.Figure"', async () => {
        const mock = createMockExportedFigure(SCATTER_PAYLOAD);
        const { container, cleanup } = await renderWithAct(
            h(FigureStub, { objectId: 0, exportedObject: mock }),
            { checkFn: (body) => body.querySelector('[data-figure-type]') !== null }
        );
        const el = container.querySelector('[data-component="dh.Figure"]');
        expect(el).not.toBeNull();
        cleanup();
    });

    it('sets data-object-id', async () => {
        const mock = createMockExportedFigure(SCATTER_PAYLOAD);
        const { container, cleanup } = await renderWithAct(
            h(FigureStub, { objectId: 3, exportedObject: mock }),
            { checkFn: (body) => body.querySelector('[data-figure-type]') !== null }
        );
        const el = container.querySelector('[data-object-id="3"]');
        expect(el).not.toBeNull();
        cleanup();
    });
});

describe('FigureStub scatter summary', () => {
    it('sets data-figure-type and data-trace-count after load', async () => {
        const mock = createMockExportedFigure(SCATTER_PAYLOAD);
        const { container, cleanup } = await renderWithAct(
            h(FigureStub, { objectId: 0, exportedObject: mock }),
            { checkFn: (body) => body.querySelector('[data-figure-type]') !== null }
        );
        const el = container.querySelector('[role="figure"]');
        expect(el.getAttribute('data-figure-type')).toBe('scatter');
        expect(el.getAttribute('data-trace-count')).toBe('2');
        cleanup();
    });

    it('sets data-figure-type and data-trace-count', async () => {
        const mock = createMockExportedFigure(SCATTER_PAYLOAD);
        const { container, cleanup } = await renderWithAct(
            h(FigureStub, { objectId: 0, exportedObject: mock }),
            { checkFn: (body) => body.querySelector('[data-figure-type]') !== null }
        );
        const el = container.querySelector('[role="figure"]');
        expect(el.getAttribute('data-figure-type')).toBe('scatter');
        expect(el.getAttribute('data-trace-count')).toBe('2');
        cleanup();
    });

    it('shows title', async () => {
        const mock = createMockExportedFigure(SCATTER_PAYLOAD);
        const { container, cleanup } = await renderWithAct(
            h(FigureStub, { objectId: 0, exportedObject: mock }),
            { checkFn: (body) => body.querySelector('[data-figure-type]') !== null }
        );
        const title = container.querySelector('[data-testid="figure-title"]');
        expect(title).not.toBeNull();
        expect(title.textContent).toBe('Square Root');
        cleanup();
    });

    it('shows axes', async () => {
        const mock = createMockExportedFigure(SCATTER_PAYLOAD);
        const { container, cleanup } = await renderWithAct(
            h(FigureStub, { objectId: 0, exportedObject: mock }),
            { checkFn: (body) => body.querySelector('[data-figure-type]') !== null }
        );
        const axes = container.querySelectorAll('[data-testid="figure-axis"]');
        expect(axes.length).toBe(2);
        expect(axes[0].textContent).toBe('x: X');
        expect(axes[1].textContent).toBe('y: Y');
        cleanup();
    });

    it('shows trace names', async () => {
        const mock = createMockExportedFigure(SCATTER_PAYLOAD);
        const { container, cleanup } = await renderWithAct(
            h(FigureStub, { objectId: 0, exportedObject: mock }),
            { checkFn: (body) => body.querySelector('[data-figure-type]') !== null }
        );
        const traces = container.querySelectorAll('[data-testid="figure-trace"]');
        expect(traces.length).toBe(2);
        expect(traces[0].textContent).toBe('scatter "alpha"');
        expect(traces[1].textContent).toBe('scatter "beta"');
        cleanup();
    });
});

describe('FigureStub histogram summary', () => {
    it('sets bar type and 1 trace for histogram', async () => {
        const mock = createMockExportedFigure(HISTOGRAM_PAYLOAD);
        const { container, cleanup } = await renderWithAct(
            h(FigureStub, { objectId: 0, exportedObject: mock }),
            { checkFn: (body) => body.querySelector('[data-figure-type]') !== null }
        );
        const el = container.querySelector('[role="figure"]');
        expect(el.getAttribute('data-figure-type')).toBe('bar');
        expect(el.getAttribute('data-trace-count')).toBe('1');
        cleanup();
    });

    it('shows histogram title and axes', async () => {
        const mock = createMockExportedFigure(HISTOGRAM_PAYLOAD);
        const { container, cleanup } = await renderWithAct(
            h(FigureStub, { objectId: 0, exportedObject: mock }),
            { checkFn: (body) => body.querySelector('[data-figure-type]') !== null }
        );
        expect(container.querySelector('[data-testid="figure-title"]').textContent).toBe('Y Distribution');
        const axes = container.querySelectorAll('[data-testid="figure-axis"]');
        expect(axes[0].textContent).toBe('x: Y');
        expect(axes[1].textContent).toBe('y: count');
        cleanup();
    });

    it('does not show unnamed traces in trace list', async () => {
        const mock = createMockExportedFigure(HISTOGRAM_PAYLOAD);
        const { container, cleanup } = await renderWithAct(
            h(FigureStub, { objectId: 0, exportedObject: mock }),
            { checkFn: (body) => body.querySelector('[data-figure-type]') !== null }
        );
        const traces = container.querySelectorAll('[data-testid="figure-trace"]');
        expect(traces.length).toBe(0);
        cleanup();
    });
});

describe('FigureStub expand/collapse', () => {
    it('shows expand button after data loads', async () => {
        const mock = createMockExportedFigure(SCATTER_PAYLOAD);
        const { container, cleanup } = await renderWithAct(
            h(FigureStub, { objectId: 0, exportedObject: mock }),
            { checkFn: (body) => body.querySelector('[data-figure-type]') !== null }
        );
        const button = container.querySelector('[data-testid="figure-expand"]');
        expect(button).not.toBeNull();
        expect(button.textContent).toBe('Expand figure');
        cleanup();
    });

    it('does not show data mappings when collapsed', async () => {
        const mock = createMockExportedFigure(SCATTER_PAYLOAD);
        const { container, cleanup } = await renderWithAct(
            h(FigureStub, { objectId: 0, exportedObject: mock }),
            { checkFn: (body) => body.querySelector('[data-figure-type]') !== null }
        );
        expect(container.querySelector('[data-testid="figure-mappings"]')).toBeNull();
        cleanup();
    });

    it('shows data mappings after clicking expand', async () => {
        const mock = createMockExportedFigure(SCATTER_PAYLOAD);
        const { container, cleanup } = await renderWithAct(
            h(FigureStub, { objectId: 0, exportedObject: mock }),
            { checkFn: (body) => body.querySelector('[data-figure-type]') !== null }
        );

        const button = container.querySelector('[data-testid="figure-expand"]');
        await React.act(async () => { button.click(); });

        const mappings = container.querySelectorAll('[data-testid="figure-mapping"]');
        expect(mappings.length).toBe(2);
        expect(mappings[0].textContent).toBe('X → x');
        expect(mappings[1].textContent).toBe('Y → y');

        expect(button.textContent).toBe('Collapse figure');
        cleanup();
    });

    it('shows layout props after clicking expand on histogram', async () => {
        const mock = createMockExportedFigure(HISTOGRAM_PAYLOAD);
        const { container, cleanup } = await renderWithAct(
            h(FigureStub, { objectId: 0, exportedObject: mock }),
            { checkFn: (body) => body.querySelector('[data-figure-type]') !== null }
        );

        const button = container.querySelector('[data-testid="figure-expand"]');
        await React.act(async () => { button.click(); });

        const layoutProps = container.querySelectorAll('[data-testid="figure-prop"]');
        const texts = Array.from(layoutProps).map(el => el.textContent);
        expect(texts).toContain('barmode: group');
        expect(texts).toContain('showlegend: false');
        cleanup();
    });

    it('hides mappings after clicking collapse', async () => {
        const mock = createMockExportedFigure(SCATTER_PAYLOAD);
        const { container, cleanup } = await renderWithAct(
            h(FigureStub, { objectId: 0, exportedObject: mock }),
            { checkFn: (body) => body.querySelector('[data-figure-type]') !== null }
        );

        const button = container.querySelector('[data-testid="figure-expand"]');
        await React.act(async () => { button.click(); });
        expect(container.querySelector('[data-testid="figure-mappings"]')).not.toBeNull();

        await React.act(async () => { button.click(); });
        expect(container.querySelector('[data-testid="figure-mappings"]')).toBeNull();
        expect(button.textContent).toBe('Expand figure');
        cleanup();
    });

    it('skips tmp-prefixed columns in mappings', async () => {
        const mock = createMockExportedFigure(HISTOGRAM_PAYLOAD);
        const { container, cleanup } = await renderWithAct(
            h(FigureStub, { objectId: 0, exportedObject: mock }),
            { checkFn: (body) => body.querySelector('[data-figure-type]') !== null }
        );

        const button = container.querySelector('[data-testid="figure-expand"]');
        await React.act(async () => { button.click(); });

        const mappings = container.querySelectorAll('[data-testid="figure-mapping"]');
        const texts = Array.from(mappings).map(el => el.textContent);
        // tmpbar0 should be filtered out
        expect(texts.some(t => t.includes('tmpbar0'))).toBe(false);
        // Y should still be present
        expect(texts).toContain('Y → x');
        cleanup();
    });
});

describe('FigureStub no title', () => {
    it('does not render title element when figure has no title', async () => {
        const noTitlePayload = {
            ...SCATTER_PAYLOAD,
            figure: {
                ...SCATTER_PAYLOAD.figure,
                plotly: {
                    ...SCATTER_PAYLOAD.figure.plotly,
                    layout: {
                        ...SCATTER_PAYLOAD.figure.plotly.layout,
                        title: undefined,
                    },
                },
            },
        };
        const mock = createMockExportedFigure(noTitlePayload);
        const { container, cleanup } = await renderWithAct(
            h(FigureStub, { objectId: 0, exportedObject: mock }),
            { checkFn: (body) => body.querySelector('[data-figure-type]') !== null }
        );
        expect(container.querySelector('[data-testid="figure-title"]')).toBeNull();
        cleanup();
    });
});

describe('FigureStub error handling', () => {
    it('shows error when figure fetch fails', async () => {
        const errorMock = {
            type: 'deephaven.plot.express.DeephavenFigure',
            reexport: () => Promise.reject(new Error('Connection lost')),
        };

        const { container, cleanup } = await renderWithAct(
            h(FigureStub, { objectId: 0, exportedObject: errorMock }),
            { checkFn: (body) => body.querySelector('[data-testid="figure-error"]') !== null }
        );

        const errorEl = container.querySelector('[data-testid="figure-error"]');
        expect(errorEl).not.toBeNull();
        expect(errorEl.textContent).toContain('Connection lost');
        cleanup();
    });

    it('does not set data-figure-type when fetch fails', async () => {
        const errorMock = {
            type: 'deephaven.plot.express.DeephavenFigure',
            reexport: () => Promise.reject(new Error('fail')),
        };

        const { container, cleanup } = await renderWithAct(
            h(FigureStub, { objectId: 5, exportedObject: errorMock }),
            { checkFn: (body) => body.querySelector('[data-testid="figure-error"]') !== null }
        );

        const el = container.querySelector('[role="figure"]');
        expect(el.getAttribute('data-figure-type')).toBeNull();
        expect(el.getAttribute('data-object-id')).toBe('5');
        cleanup();
    });
});

describe('FigureStub loading state', () => {
    it('shows loading indicator before data arrives', async () => {
        // Create a mock that never resolves
        const slowMock = {
            type: 'deephaven.plot.express.DeephavenFigure',
            reexport: () => new Promise(() => {}), // never resolves
        };

        const { container, cleanup } = await renderAndWait(
            h(FigureStub, { objectId: 0, exportedObject: slowMock }),
            50
        );

        const loading = container.querySelector('[data-testid="figure-loading"]');
        expect(loading).not.toBeNull();
        expect(loading.textContent).toContain('Loading');
        cleanup();
    });
});
