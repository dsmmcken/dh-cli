import { describe, it, expect } from 'vitest';
import { DEFAULT_COMPONENT_MAP, createComponentMap } from '../../src/ComponentMap.mjs';
import { renderAndWait } from '../helpers/render-utils.mjs';

const React = (await import('react')).default;
const { createElement: h } = React;

describe('DEFAULT_COMPONENT_MAP', () => {
    it('is a plain object with many entries', () => {
        expect(typeof DEFAULT_COMPONENT_MAP).toBe('object');
        expect(DEFAULT_COMPONENT_MAP).not.toBeNull();
        const keys = Object.keys(DEFAULT_COMPONENT_MAP);
        expect(keys.length).toBeGreaterThan(20);
    });

    it('has entries for common components', () => {
        const expected = [
            'deephaven.ui.components.Button',
            'deephaven.ui.components.Text',
            'deephaven.ui.components.Panel',
            'deephaven.ui.components.Flex',
            'deephaven.ui.components.TextField',
            'deephaven.ui.components.Checkbox',
            'deephaven.ui.components.Slider',
            'deephaven.ui.components.Tabs',
            'deephaven.ui.components.Dialog',
        ];
        for (const name of expected) {
            expect(DEFAULT_COMPONENT_MAP).toHaveProperty(name);
            expect(typeof DEFAULT_COMPONENT_MAP[name]).toBe('function');
        }
    });

    it('Button stub renders with data-component and role="button"', async () => {
        const Component = DEFAULT_COMPONENT_MAP['deephaven.ui.components.Button'];
        const { container, cleanup } = await renderAndWait(h(Component, null, 'Click'));
        const btn = container.querySelector('[data-component="deephaven.ui.components.Button"]');
        expect(btn).not.toBeNull();
        expect(btn.getAttribute('role')).toBe('button');
        cleanup();
    });

    it('Text stub renders with data-component', async () => {
        const Component = DEFAULT_COMPONENT_MAP['deephaven.ui.components.Text'];
        const { container, cleanup } = await renderAndWait(h(Component, null, 'Hello'));
        const el = container.querySelector('[data-component="deephaven.ui.components.Text"]');
        expect(el).not.toBeNull();
        expect(el.textContent).toBe('Hello');
        cleanup();
    });

    it('Panel stub renders children', async () => {
        const Panel = DEFAULT_COMPONENT_MAP['deephaven.ui.components.Panel'];
        const { container, cleanup } = await renderAndWait(
            h(Panel, { title: 'My Panel' }, h('span', null, 'child content'))
        );
        const panel = container.querySelector('[data-component="deephaven.ui.components.Panel"]');
        expect(panel).not.toBeNull();
        expect(panel.textContent).toContain('child content');
        cleanup();
    });

    it('stub components pass through children as text', async () => {
        const Component = DEFAULT_COMPONENT_MAP['deephaven.ui.components.Button'];
        const { container, cleanup } = await renderAndWait(h(Component, null, 'Press me'));
        const btn = container.querySelector('[data-component="deephaven.ui.components.Button"]');
        expect(btn.textContent).toBe('Press me');
        cleanup();
    });

    it('TextField stub has role="textbox"', async () => {
        const Component = DEFAULT_COMPONENT_MAP['deephaven.ui.components.TextField'];
        const { container, cleanup } = await renderAndWait(h(Component, { label: 'Name' }));
        const input = container.querySelector('[role="textbox"]');
        expect(input).not.toBeNull();
        cleanup();
    });

    it('Checkbox stub has role="checkbox"', async () => {
        const Component = DEFAULT_COMPONENT_MAP['deephaven.ui.components.Checkbox'];
        const { container, cleanup } = await renderAndWait(h(Component, null, 'Accept'));
        const el = container.querySelector('[role="checkbox"]');
        expect(el).not.toBeNull();
        cleanup();
    });

    it('Panel stub has role="region"', async () => {
        const Panel = DEFAULT_COMPONENT_MAP['deephaven.ui.components.Panel'];
        const { container, cleanup } = await renderAndWait(h(Panel, { title: 'Test' }));
        const el = container.querySelector('[role="region"]');
        expect(el).not.toBeNull();
        cleanup();
    });

    it('Flex stub renders with data-component and role="group"', async () => {
        const Flex = DEFAULT_COMPONENT_MAP['deephaven.ui.components.Flex'];
        const { container, cleanup } = await renderAndWait(
            h(Flex, { direction: 'row' }, h('span', null, 'A'), h('span', null, 'B'))
        );
        const el = container.querySelector('[data-component="deephaven.ui.components.Flex"]');
        expect(el).not.toBeNull();
        expect(el.getAttribute('role')).toBe('group');
        expect(el.textContent).toContain('A');
        expect(el.textContent).toContain('B');
        cleanup();
    });

    it('Button calls onPress when clicked', async () => {
        const Component = DEFAULT_COMPONENT_MAP['deephaven.ui.components.Button'];
        let pressed = false;
        const { container, cleanup } = await renderAndWait(
            h(Component, { onPress: () => { pressed = true; } }, 'Go')
        );
        const btn = container.querySelector('[data-component="deephaven.ui.components.Button"]');
        btn.click();
        expect(pressed).toBe(true);
        cleanup();
    });
});

describe('createComponentMap', () => {
    it('merges overrides with defaults', () => {
        const CustomButton = () => h('div', null, 'custom');
        const map = createComponentMap({
            'my.custom.Widget': CustomButton,
        });
        expect(map['my.custom.Widget']).toBe(CustomButton);
        // Defaults still present
        expect(map['deephaven.ui.components.Button']).toBe(
            DEFAULT_COMPONENT_MAP['deephaven.ui.components.Button']
        );
    });

    it('override replaces existing component', () => {
        const MyButton = () => h('div', null, 'my button');
        const map = createComponentMap({
            'deephaven.ui.components.Button': MyButton,
        });
        expect(map['deephaven.ui.components.Button']).toBe(MyButton);
        expect(map['deephaven.ui.components.Button']).not.toBe(
            DEFAULT_COMPONENT_MAP['deephaven.ui.components.Button']
        );
    });

    it('returns a new object, not the same reference', () => {
        const map = createComponentMap();
        expect(map).not.toBe(DEFAULT_COMPONENT_MAP);
        expect(map).toEqual(DEFAULT_COMPONENT_MAP);
    });
});
