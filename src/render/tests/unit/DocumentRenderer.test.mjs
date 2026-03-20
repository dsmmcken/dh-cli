import { describe, it, expect, vi } from 'vitest';
import { DocumentRenderer } from '../../src/DocumentRenderer.mjs';
import { DEFAULT_COMPONENT_MAP } from '../../src/ComponentMap.mjs';
import { renderAndWait } from '../helpers/render-utils.mjs';
import {
    SIMPLE_TEXT_DOC, BUTTON_DOC, NESTED_DOC, CALLABLE_DOC,
    HTML_DOC, EMPTY_DOC, MULTI_OBJECT_DOC,
    FIGURE_DOC, FIGURE_AND_TABLE_DOC, createMockExportedFigure,
} from '../helpers/mock-documents.mjs';
import { renderWithAct } from '../helpers/render-utils.mjs';

const React = (await import('react')).default;
const { createElement: h } = React;

/**
 * Helper to create a renderer with standard defaults.
 */
function createRenderer(overrides = {}) {
    return new DocumentRenderer({
        exportedObjectMap: overrides.exportedObjectMap || new Map(),
        callCallable: overrides.callCallable || vi.fn(),
        componentMap: overrides.componentMap || DEFAULT_COMPONENT_MAP,
    });
}

describe('DocumentRenderer', () => {
    it('renders simple text document', async () => {
        const renderer = createRenderer();
        const element = renderer.render(SIMPLE_TEXT_DOC);
        const { container, cleanup } = await renderAndWait(element);
        const textEl = container.querySelector('[data-component="deephaven.ui.components.Text"]');
        expect(textEl).not.toBeNull();
        expect(textEl.textContent).toBe('Hello');
        cleanup();
    });

    it('renders button with onPress callback', async () => {
        const callCallable = vi.fn();
        const renderer = createRenderer({ callCallable });
        const element = renderer.render(BUTTON_DOC);
        const { container, cleanup } = await renderAndWait(element);
        const btn = container.querySelector('[data-component="deephaven.ui.components.Button"]');
        expect(btn).not.toBeNull();
        expect(btn.textContent).toBe('Click Me');
        // The button should be clickable and invoke the callable
        btn.click();
        expect(callCallable).toHaveBeenCalledWith('cb0', expect.any(Array));
        cleanup();
    });

    it('renders nested documents', async () => {
        const renderer = createRenderer();
        const element = renderer.render(NESTED_DOC);
        const { container, cleanup } = await renderAndWait(element);
        // Should have Flex > Panel > Button + Text
        const flex = container.querySelector('[data-component="deephaven.ui.components.Flex"]');
        expect(flex).not.toBeNull();
        const panel = container.querySelector('[data-component="deephaven.ui.components.Panel"]');
        expect(panel).not.toBeNull();
        const btn = container.querySelector('[data-component="deephaven.ui.components.Button"]');
        expect(btn).not.toBeNull();
        expect(btn.textContent).toBe('Nested Button');
        const text = container.querySelector('[data-component="deephaven.ui.components.Text"]');
        expect(text).not.toBeNull();
        expect(text.textContent).toBe('Nested Text');
        cleanup();
    });

    it('converts callables to functions that call callCallable', async () => {
        const callCallable = vi.fn();
        const renderer = createRenderer({ callCallable });
        const element = renderer.render(CALLABLE_DOC);
        const { container, cleanup } = await renderAndWait(element);
        // CALLABLE_DOC has cb0 (Increment), cb1 (Decrement), cb2 (onChange for TextField)
        const buttons = container.querySelectorAll('[data-component="deephaven.ui.components.Button"]');
        expect(buttons.length).toBe(2);
        // Click Increment button
        buttons[0].click();
        expect(callCallable).toHaveBeenCalledWith('cb0', expect.any(Array));
        // Click Decrement button
        buttons[1].click();
        expect(callCallable).toHaveBeenCalledWith('cb1', expect.any(Array));
        cleanup();
    });

    it('renders HTML elements (deephaven.ui.html.*) as native tags', async () => {
        const renderer = createRenderer();
        const element = renderer.render(HTML_DOC);
        const { container, cleanup } = await renderAndWait(element);
        const h1 = container.querySelector('h1');
        expect(h1).not.toBeNull();
        expect(h1.textContent).toBe('Title');
        const p = container.querySelector('p');
        expect(p).not.toBeNull();
        expect(p.textContent).toBe('Paragraph text');
        cleanup();
    });

    it('renders exported object placeholders', async () => {
        const exportedObjectMap = new Map([
            [0, { type: 'Table' }],
            [1, { type: 'Figure' }],
        ]);
        const renderer = createRenderer({ exportedObjectMap });
        const element = renderer.render(MULTI_OBJECT_DOC);
        const { container, cleanup } = await renderAndWait(element);
        // UITable components receive the exported object as their table prop
        const tables = container.querySelectorAll('[data-component="deephaven.ui.elements.UITable"]');
        expect(tables.length).toBe(2);
        cleanup();
    });

    it('renders arrays as fragments', async () => {
        const renderer = createRenderer();
        const doc = [
            { __dhElemName: 'deephaven.ui.components.Text', props: { children: ['A'] } },
            { __dhElemName: 'deephaven.ui.components.Text', props: { children: ['B'] } },
        ];
        const element = renderer.render(doc);
        const { container, cleanup } = await renderAndWait(element);
        const texts = container.querySelectorAll('[data-component="deephaven.ui.components.Text"]');
        expect(texts.length).toBe(2);
        expect(texts[0].textContent).toBe('A');
        expect(texts[1].textContent).toBe('B');
        cleanup();
    });

    it('renders primitive string children', async () => {
        const renderer = createRenderer();
        const doc = {
            __dhElemName: 'deephaven.ui.components.Panel',
            props: {
                children: ['Just a string'],
            },
        };
        const element = renderer.render(doc);
        const { container, cleanup } = await renderAndWait(element);
        expect(container.textContent).toContain('Just a string');
        cleanup();
    });

    it('handles empty children array', async () => {
        const renderer = createRenderer();
        const element = renderer.render(EMPTY_DOC);
        const { container, cleanup } = await renderAndWait(element);
        const panel = container.querySelector('[data-component="deephaven.ui.components.Panel"]');
        expect(panel).not.toBeNull();
        cleanup();
    });

    it('handles unknown element names with fallback', async () => {
        const renderer = createRenderer();
        const doc = {
            __dhElemName: 'some.unknown.Component',
            props: {
                children: ['Fallback content'],
            },
        };
        const element = renderer.render(doc);
        const { container, cleanup } = await renderAndWait(element);
        const el = container.querySelector('[data-component="some.unknown.Component"]');
        expect(el).not.toBeNull();
        expect(el.getAttribute('data-unknown')).toBe('true');
        expect(el.textContent).toBe('Fallback content');
        cleanup();
    });

    it('uses custom componentMap when provided', async () => {
        const CustomWidget = (props) => h('div', {
            'data-component': 'custom-widget',
            'data-custom': 'true',
        }, props.children);

        const renderer = createRenderer({
            componentMap: {
                ...DEFAULT_COMPONENT_MAP,
                'my.custom.Widget': CustomWidget,
            },
        });

        const doc = {
            __dhElemName: 'my.custom.Widget',
            props: { children: ['Custom!'] },
        };
        const element = renderer.render(doc);
        const { container, cleanup } = await renderAndWait(element);
        const el = container.querySelector('[data-component="custom-widget"]');
        expect(el).not.toBeNull();
        expect(el.getAttribute('data-custom')).toBe('true');
        expect(el.textContent).toBe('Custom!');
        cleanup();
    });

    it('renders multiple elements at same level', async () => {
        const renderer = createRenderer();
        const doc = {
            __dhElemName: 'deephaven.ui.components.Flex',
            props: {
                children: [
                    { __dhElemName: 'deephaven.ui.components.Button', props: { children: ['A'] } },
                    { __dhElemName: 'deephaven.ui.components.Button', props: { children: ['B'] } },
                    { __dhElemName: 'deephaven.ui.components.Button', props: { children: ['C'] } },
                ],
            },
        };
        const element = renderer.render(doc);
        const { container, cleanup } = await renderAndWait(element);
        const buttons = container.querySelectorAll('[data-component="deephaven.ui.components.Button"]');
        expect(buttons.length).toBe(3);
        expect(buttons[0].textContent).toBe('A');
        expect(buttons[1].textContent).toBe('B');
        expect(buttons[2].textContent).toBe('C');
        cleanup();
    });

    it('returns null for null or non-object document', () => {
        const renderer = createRenderer();
        expect(renderer.render(null)).toBeNull();
        expect(renderer.render(undefined)).toBeNull();
        expect(renderer.render('just a string')).toBeNull();
        expect(renderer.render(42)).toBeNull();
    });

    it('renders TextField with onChange callable', async () => {
        const callCallable = vi.fn();
        const renderer = createRenderer({ callCallable });
        const element = renderer.render(CALLABLE_DOC);
        const { container, cleanup } = await renderAndWait(element);
        const input = container.querySelector('input[role="textbox"]');
        expect(input).not.toBeNull();
        cleanup();
    });

    it('renders figure exported objects with FigureStub instead of placeholder', async () => {
        const scatterPayload = {
            type: 'NEW_FIGURE',
            figure: {
                plotly: {
                    data: [{ type: 'scattergl', name: 'alpha', mode: 'markers' }],
                    layout: { title: { text: 'Test' }, xaxis: { title: { text: 'X' } }, yaxis: { title: { text: 'Y' } } },
                },
                deephaven: { mappings: [], is_user_set_template: false, is_user_set_color: false },
            },
            revision: 0,
            new_references: [],
            removed_references: [],
        };
        const figureMock = createMockExportedFigure(scatterPayload);
        const exportedObjectMap = new Map([[0, figureMock]]);
        const renderer = createRenderer({ exportedObjectMap });
        const element = renderer.render(FIGURE_DOC);
        const { container, cleanup } = await renderWithAct(element, {
            checkFn: (body) => body.querySelector('[data-figure-type]') !== null,
        });

        // Should render FigureStub, not ExportedObjectPlaceholder
        const figure = container.querySelector('[role="figure"]');
        expect(figure).not.toBeNull();
        expect(figure.getAttribute('data-component')).toBe('dh.Figure');

        // Should NOT have the generic exported object placeholder
        const placeholder = container.querySelector('[data-component="dh.ExportedObject"]');
        expect(placeholder).toBeNull();

        cleanup();
    });

    it('renders Table exported objects with normal placeholder (not FigureStub)', async () => {
        const exportedObjectMap = new Map([
            [0, { type: 'Table' }],
            [1, { type: 'Table' }],
        ]);
        const renderer = createRenderer({ exportedObjectMap });
        const element = renderer.render(MULTI_OBJECT_DOC);
        const { container, cleanup } = await renderAndWait(element);

        // Tables go through UITable component (not FigureStub)
        const tables = container.querySelectorAll('[data-component="deephaven.ui.elements.UITable"]');
        expect(tables.length).toBe(2);

        // No figures
        const figures = container.querySelectorAll('[role="figure"]');
        expect(figures.length).toBe(0);

        cleanup();
    });

    it('renders mixed figure and table objects correctly', async () => {
        const scatterPayload = {
            type: 'NEW_FIGURE',
            figure: {
                plotly: {
                    data: [{ type: 'scattergl', name: 's1', mode: 'markers' }],
                    layout: { title: { text: 'Mixed' } },
                },
                deephaven: { mappings: [] },
            },
            revision: 0, new_references: [], removed_references: [],
        };
        const exportedObjectMap = new Map([
            [0, createMockExportedFigure(scatterPayload)],
            [1, createMockExportedFigure(scatterPayload)],
            [2, { type: 'Table' }],
        ]);
        const renderer = createRenderer({ exportedObjectMap });
        const element = renderer.render(FIGURE_AND_TABLE_DOC);
        const { container, cleanup } = await renderWithAct(element, {
            checkFn: (body) => body.querySelector('[data-figure-type]') !== null,
        });

        const figures = container.querySelectorAll('[role="figure"]');
        expect(figures.length).toBe(2);

        // The UITable element renders the table (not as ExportedObjectPlaceholder)
        const tables = container.querySelectorAll('[data-component="deephaven.ui.elements.UITable"]');
        expect(tables.length).toBe(1);

        cleanup();
    });
});
