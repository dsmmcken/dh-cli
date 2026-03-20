import { describe, it, expect, beforeAll, afterAll } from 'vitest';
import { loadGoldenLayout, createTestLayout } from '../../src/GoldenLayoutSetup.mjs';

let loaded = false;

beforeAll(async () => {
    await loadGoldenLayout();
    loaded = true;
}, 30000);

describe('loadGoldenLayout', () => {
    it('succeeds without error', () => {
        expect(loaded).toBe(true);
    });

    it('sets up jQuery globally', () => {
        expect(typeof globalThis.jQuery).toBe('function');
        expect(typeof globalThis.$).toBe('function');
        expect(globalThis.jQuery).toBe(globalThis.$);
    });
});

describe('createTestLayout', () => {
    let result;

    beforeAll(() => {
        const win = globalThis.__TEST_DOM__.window;
        result = createTestLayout(win);
    });

    afterAll(() => {
        if (result) {
            result.destroy();
        }
    });

    it('returns a layout instance', () => {
        expect(result.layout).toBeDefined();
        expect(result.layout).not.toBeNull();
    });

    it('returns portalPanelMap as a Map', () => {
        expect(result.portalPanelMap).toBeInstanceOf(Map);
    });

    it('returns a destroy function', () => {
        expect(typeof result.destroy).toBe('function');
    });

    it('layout has expected properties', () => {
        expect(result.layout.eventHub).toBeDefined();
        expect(result.layout.root).toBeDefined();
    });

    it('creates a container element in the DOM', () => {
        expect(result.container).toBeInstanceOf(globalThis.__TEST_DOM__.window.HTMLElement);
        expect(result.container.id).toBe('dh-test-layout');
    });
});

describe('createTestLayout with custom dimensions', () => {
    let result;

    beforeAll(() => {
        const win = globalThis.__TEST_DOM__.window;
        result = createTestLayout(win, { width: 800, height: 600 });
    });

    afterAll(() => {
        if (result) {
            result.destroy();
        }
    });

    it('applies custom width and height to the container', () => {
        const rect = result.container.getBoundingClientRect();
        expect(rect.width).toBe(800);
        expect(rect.height).toBe(600);
        expect(result.container.offsetWidth).toBe(800);
        expect(result.container.offsetHeight).toBe(600);
    });
});

describe('destroy', () => {
    it('cleans up layout and removes container from DOM', () => {
        const win = globalThis.__TEST_DOM__.window;
        const result = createTestLayout(win);
        const container = result.container;
        const portalPanelMap = result.portalPanelMap;

        // Add an entry to verify it gets cleared
        portalPanelMap.set('test-key', 'test-value');

        expect(win.document.getElementById('dh-test-layout')).not.toBeNull();

        result.destroy();

        // Container should be removed from the DOM
        expect(container.parentNode).toBeNull();
        // portalPanelMap should be cleared
        expect(portalPanelMap.size).toBe(0);
    });
});
