import { describe, it, expect } from 'vitest';
import { UITableStub, UI_TABLE_ELEMENT_NAME, createUITableStubPluginMap } from '../../src/UITableStub.mjs';
import { renderAndWait, renderWithAct } from '../helpers/render-utils.mjs';
import { createMockExportedTable } from '../helpers/mock-documents.mjs';

const React = (await import('react')).default;
const { createElement: h } = React;

describe('UI_TABLE_ELEMENT_NAME', () => {
    it('equals "deephaven.ui.elements.UITable"', () => {
        expect(UI_TABLE_ELEMENT_NAME).toBe('deephaven.ui.elements.UITable');
    });
});

describe('createUITableStubPluginMap', () => {
    it('returns a Map', () => {
        const map = createUITableStubPluginMap();
        expect(map).toBeInstanceOf(Map);
    });

    it('has one entry of type "ElementPlugin"', () => {
        const map = createUITableStubPluginMap();
        expect(map.size).toBe(1);
        const entry = [...map.values()][0];
        expect(entry.type).toBe('ElementPlugin');
    });

    it('plugin map entry has a name property', () => {
        const map = createUITableStubPluginMap();
        const entry = [...map.values()][0];
        expect(typeof entry.name).toBe('string');
        expect(entry.name.length).toBeGreaterThan(0);
    });

    it('plugin map mapping includes UITable element name', () => {
        const map = createUITableStubPluginMap();
        const entry = [...map.values()][0];
        expect(entry.mapping).toHaveProperty(UI_TABLE_ELEMENT_NAME);
        expect(entry.mapping[UI_TABLE_ELEMENT_NAME]).toBe(UITableStub);
    });
});

describe('UITableStub', () => {
    it('renders with role="table"', async () => {
        const { container, cleanup } = await renderAndWait(h(UITableStub, {}));
        const el = container.querySelector('[role="table"]');
        expect(el).not.toBeNull();
        cleanup();
    });

    it('renders with data-component attribute', async () => {
        const { container, cleanup } = await renderAndWait(h(UITableStub, {}));
        const el = container.querySelector('[data-component="deephaven.ui.elements.UITable"]');
        expect(el).not.toBeNull();
        cleanup();
    });

    it('shows table type via data attribute when table prop is provided', async () => {
        const { container, cleanup } = await renderAndWait(
            h(UITableStub, { table: { type: 'TreeTable' } })
        );
        const el = container.querySelector('[data-table-type="TreeTable"]');
        expect(el).not.toBeNull();
        cleanup();
    });

    it('shows "unknown" when table prop has no type', async () => {
        const { container, cleanup } = await renderAndWait(
            h(UITableStub, { table: {} })
        );
        const el = container.querySelector('[data-table-type="unknown"]');
        expect(el).not.toBeNull();
        cleanup();
    });

    it('renders children', async () => {
        const { container, cleanup } = await renderAndWait(
            h(UITableStub, {}, h('span', { className: 'child' }, 'extra content'))
        );
        const child = container.querySelector('.child');
        expect(child).not.toBeNull();
        expect(child.textContent).toBe('extra content');
        cleanup();
    });

    it('does not set data-table-type when no table prop', async () => {
        const { container, cleanup } = await renderAndWait(h(UITableStub, {}));
        const el = container.querySelector('[role="table"]');
        expect(el.getAttribute('data-table-type')).toBe('unknown');
        cleanup();
    });

    it('passes through extra string/number/boolean props as data attributes', async () => {
        const { container, cleanup } = await renderAndWait(
            h(UITableStub, { reverse: true, pageSize: 100 })
        );
        const el = container.querySelector('[data-component="deephaven.ui.elements.UITable"]');
        expect(el.getAttribute('data-reverse')).toBe('true');
        expect(el.getAttribute('data-pagesize')).toBe('100');
        cleanup();
    });
});

describe('UITableStub config props', () => {
    it('renders string, boolean, and array config props', async () => {
        const { container, cleanup } = await renderAndWait(
            h(UITableStub, {
                table: { type: 'Table' },
                reverse: true,
                density: 'compact',
                frontColumns: ['Timestamp', 'Species'],
            })
        );

        const config = container.querySelector('[data-testid="table-config"]');
        expect(config).not.toBeNull();

        const props = Array.from(config.querySelectorAll('[data-testid="table-prop"]'));
        const texts = props.map(p => p.textContent);
        expect(texts).toContain('reverse: true');
        expect(texts).toContain('density: compact');
        expect(texts).toContain('frontColumns: Timestamp, Species');

        cleanup();
    });

    it('shows handler names for function props', async () => {
        const { container, cleanup } = await renderAndWait(
            h(UITableStub, {
                table: { type: 'Table' },
                onRowDoublePress: () => {},
                onCellPress: () => {},
            })
        );

        const config = container.querySelector('[data-testid="table-config"]');
        const props = Array.from(config.querySelectorAll('[data-testid="table-prop"]'));
        const texts = props.map(p => p.textContent);
        expect(texts).toContain('handlers: onRowDoublePress, onCellPress');

        cleanup();
    });

    it('shows complex array props with item count', async () => {
        const { container, cleanup } = await renderAndWait(
            h(UITableStub, {
                table: { type: 'Table' },
                sorts: [{ column: 'X', direction: 'ASC' }, { column: 'Y', direction: 'DESC' }],
            })
        );

        const config = container.querySelector('[data-testid="table-config"]');
        const props = Array.from(config.querySelectorAll('[data-testid="table-prop"]'));
        const texts = props.map(p => p.textContent);
        expect(texts).toContain('sorts: [2 items]');

        cleanup();
    });

    it('does not render config section when no config props', async () => {
        const { container, cleanup } = await renderAndWait(
            h(UITableStub, { table: { type: 'Table' } })
        );
        const config = container.querySelector('[data-testid="table-config"]');
        expect(config).toBeNull();
        cleanup();
    });
});

describe('UITableStub expand/collapse', () => {
    function createMock(opts = {}) {
        return createMockExportedTable({
            columns: opts.columns || [
                { name: 'X', type: 'int' },
                { name: 'Y', type: 'double' },
            ],
            rows: opts.rows !== undefined ? opts.rows : [
                { X: 1, Y: 3.14 },
                { X: 2, Y: 2.72 },
            ],
        });
    }

    it('shows expand button when table prop has reexport', async () => {
        const { container, cleanup } = await renderWithAct(
            h(UITableStub, { table: createMock() }),
            { checkFn: (body) => body.querySelector('[data-row-count]') !== null }
        );
        const button = container.querySelector('[data-testid="table-expand"]');
        expect(button).not.toBeNull();
        expect(button.textContent).toBe('Expand table');
        cleanup();
    });

    it('does not show expand button without table prop', async () => {
        const { container, cleanup } = await renderAndWait(h(UITableStub, {}));
        expect(container.querySelector('[data-testid="table-expand"]')).toBeNull();
        cleanup();
    });

    it('does not show rows when collapsed', async () => {
        const { container, cleanup } = await renderWithAct(
            h(UITableStub, { table: createMock() }),
            { checkFn: (body) => body.querySelector('[data-row-count]') !== null }
        );
        expect(container.querySelectorAll('[role="columnheader"]').length).toBe(0);
        expect(container.querySelectorAll('[role="cell"]').length).toBe(0);
        cleanup();
    });

    it('shows headers and data after clicking expand', async () => {
        const { container, cleanup } = await renderWithAct(
            h(UITableStub, { table: createMock() }),
            { checkFn: (body) => body.querySelector('[data-row-count]') !== null }
        );

        const button = container.querySelector('[data-testid="table-expand"]');
        await React.act(async () => { button.click(); });

        const headers = container.querySelectorAll('[role="columnheader"]');
        expect(headers.length).toBe(2);
        expect(headers[0].textContent).toBe('X (int)');
        expect(headers[1].textContent).toBe('Y (double)');

        const cells = container.querySelectorAll('[role="cell"]');
        expect(cells.length).toBe(4);
        expect(cells[0].textContent).toBe('1');
        expect(cells[1].textContent).toBe('3.14');
        expect(cells[2].textContent).toBe('2');
        expect(cells[3].textContent).toBe('2.72');

        expect(button.textContent).toBe('Collapse table');
        cleanup();
    });

    it('hides rows after clicking collapse', async () => {
        const { container, cleanup } = await renderWithAct(
            h(UITableStub, { table: createMock() }),
            { checkFn: (body) => body.querySelector('[data-row-count]') !== null }
        );

        const button = container.querySelector('[data-testid="table-expand"]');

        await React.act(async () => { button.click(); });
        expect(container.querySelectorAll('[role="cell"]').length).toBe(4);

        await React.act(async () => { button.click(); });
        expect(container.querySelectorAll('[role="cell"]').length).toBe(0);
        expect(container.querySelectorAll('[role="columnheader"]').length).toBe(0);
        expect(button.textContent).toBe('Expand table');
        cleanup();
    });

    it('populates data attributes after async load', async () => {
        const { container, cleanup } = await renderWithAct(
            h(UITableStub, { table: createMock() }),
            { checkFn: (body) => body.querySelector('[data-row-count]') !== null }
        );

        const el = container.querySelector('[role="table"]');
        expect(el.getAttribute('data-row-count')).toBe('2');
        expect(el.getAttribute('data-column-count')).toBe('2');
        cleanup();
    });

    it('populates row and column count data attributes after async load', async () => {
        const { container, cleanup } = await renderWithAct(
            h(UITableStub, { table: createMock() }),
            { checkFn: (body) => body.querySelector('[data-row-count]') !== null }
        );

        const el = container.querySelector('[role="table"]');
        expect(el.getAttribute('data-row-count')).toBe('2');
        expect(el.getAttribute('data-column-count')).toBe('2');
        cleanup();
    });

    it('handles zero-row table', async () => {
        const mock = createMock({
            columns: [{ name: 'X', type: 'int' }],
            rows: [],
        });
        const { container, cleanup } = await renderWithAct(
            h(UITableStub, { table: mock }),
            { checkFn: (body) => body.querySelector('[data-row-count]') !== null }
        );

        const el = container.querySelector('[role="table"]');
        expect(el.getAttribute('data-row-count')).toBe('0');
        expect(el.getAttribute('data-column-count')).toBe('1');

        const button = container.querySelector('[data-testid="table-expand"]');
        await React.act(async () => { button.click(); });

        expect(container.querySelectorAll('[role="columnheader"]').length).toBe(1);
        expect(container.querySelectorAll('[role="cell"]').length).toBe(0);
        cleanup();
    });
});

describe('UITableStub error handling', () => {
    it('shows error when table fetch fails', async () => {
        const errorTable = {
            type: 'Table',
            reexport: () => Promise.reject(new Error('Connection lost')),
        };

        const { container, cleanup } = await renderWithAct(
            h(UITableStub, { table: errorTable }),
            { checkFn: (body) => body.querySelector('[data-testid="table-error"]') !== null }
        );

        const errorEl = container.querySelector('[data-testid="table-error"]');
        expect(errorEl).not.toBeNull();
        expect(errorEl.textContent).toContain('Connection lost');
        cleanup();
    });
});
