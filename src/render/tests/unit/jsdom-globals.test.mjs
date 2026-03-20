import { describe, it, expect, beforeEach, afterEach } from 'vitest';
import { installJsdomGlobals } from '../../src/jsdom-globals.mjs';
import { JSDOM } from 'jsdom';

describe('installJsdomGlobals', () => {
    let savedGlobals;

    // Save any globals that might be modified so we can restore them after each test.
    // This is critical because unit-setup.mjs already installed globals, and we need
    // to leave globalThis in the same state we found it.
    const KEYS_TO_SAVE = [
        'window', 'document', 'HTMLElement', 'HTMLInputElement',
        'HTMLTextAreaElement', 'HTMLSelectElement', 'HTMLAnchorElement',
        'HTMLButtonElement', 'HTMLDivElement', 'HTMLSpanElement',
        'HTMLFormElement', 'HTMLLabelElement', 'HTMLTableElement',
        'HTMLTableRowElement', 'SVGElement',
        'Node', 'Text', 'Element', 'DocumentFragment',
        'Event', 'CustomEvent', 'MouseEvent', 'KeyboardEvent',
        'InputEvent', 'FocusEvent',
        'MutationObserver', 'ResizeObserver', 'IntersectionObserver',
        'getComputedStyle', 'requestAnimationFrame', 'cancelAnimationFrame',
        'Range', 'NodeFilter', 'TreeWalker', 'DOMParser', 'XMLSerializer',
        'navigator',
        'localStorage', 'sessionStorage',
        'CSSStyleDeclaration', 'CSSStyleSheet', 'StyleSheet',
        'Image', 'HTMLImageElement',
        'URL', 'URLSearchParams',
        'AbortController', 'AbortSignal',
        'fetch', 'Request', 'Response', 'Headers',
        'matchMedia',
    ];

    beforeEach(() => {
        savedGlobals = {};
        for (const key of KEYS_TO_SAVE) {
            savedGlobals[key] = globalThis[key];
        }
    });

    afterEach(() => {
        for (const [k, v] of Object.entries(savedGlobals)) {
            if (v === undefined) {
                try { delete globalThis[k]; } catch (e) { /* ignore */ }
            } else {
                try { globalThis[k] = v; } catch (e) { /* ignore */ }
            }
        }
    });

    it('installs window on globalThis', () => {
        const dom = new JSDOM('<!DOCTYPE html><html><body></body></html>');
        const result = installJsdomGlobals(dom);
        expect(globalThis.window).toBe(dom.window);
        result.restore();
    });

    it('installs document on globalThis', () => {
        const dom = new JSDOM('<!DOCTYPE html><html><body></body></html>');
        const result = installJsdomGlobals(dom);
        expect(globalThis.document).toBe(dom.window.document);
        result.restore();
    });

    it('installs HTMLElement and other HTML type constructors', () => {
        const dom = new JSDOM('<!DOCTYPE html><html><body></body></html>');
        const result = installJsdomGlobals(dom);
        expect(globalThis.HTMLElement).toBe(dom.window.HTMLElement);
        expect(globalThis.HTMLInputElement).toBe(dom.window.HTMLInputElement);
        expect(globalThis.HTMLSelectElement).toBe(dom.window.HTMLSelectElement);
        expect(globalThis.HTMLButtonElement).toBe(dom.window.HTMLButtonElement);
        result.restore();
    });

    it('installs Event classes', () => {
        const dom = new JSDOM('<!DOCTYPE html><html><body></body></html>');
        const result = installJsdomGlobals(dom);
        expect(globalThis.Event).toBe(dom.window.Event);
        expect(globalThis.CustomEvent).toBe(dom.window.CustomEvent);
        expect(globalThis.MouseEvent).toBe(dom.window.MouseEvent);
        expect(globalThis.KeyboardEvent).toBe(dom.window.KeyboardEvent);
        expect(globalThis.FocusEvent).toBe(dom.window.FocusEvent);
        result.restore();
    });

    it('installs navigator on globalThis', () => {
        delete globalThis.navigator;
        expect(globalThis.navigator).toBeUndefined();

        const dom = new JSDOM('<!DOCTYPE html><html><body></body></html>', { url: 'http://localhost' });
        const result = installJsdomGlobals(dom);
        expect(globalThis.navigator).toBeDefined();
        expect(typeof globalThis.navigator.userAgent).toBe('string');
        result.restore();
    });

    it('installs polyfills for matchMedia, ResizeObserver, and IntersectionObserver', () => {
        // Clear any existing polyfills to ensure installJsdomGlobals creates them
        delete globalThis.matchMedia;
        delete globalThis.ResizeObserver;
        delete globalThis.IntersectionObserver;

        const dom = new JSDOM('<!DOCTYPE html><html><body></body></html>');
        const result = installJsdomGlobals(dom);

        // matchMedia should be a function that returns a MediaQueryList-like object
        expect(typeof globalThis.matchMedia).toBe('function');
        const mql = globalThis.matchMedia('(min-width: 768px)');
        expect(mql.matches).toBe(false);
        expect(mql.media).toBe('(min-width: 768px)');

        // ResizeObserver should be a class with observe/unobserve/disconnect
        expect(typeof globalThis.ResizeObserver).toBe('function');
        const ro = new globalThis.ResizeObserver();
        expect(typeof ro.observe).toBe('function');
        expect(typeof ro.unobserve).toBe('function');
        expect(typeof ro.disconnect).toBe('function');

        // IntersectionObserver should be a class with observe/unobserve/disconnect
        expect(typeof globalThis.IntersectionObserver).toBe('function');
        const io = new globalThis.IntersectionObserver();
        expect(typeof io.observe).toBe('function');
        expect(typeof io.unobserve).toBe('function');
        expect(typeof io.disconnect).toBe('function');

        result.restore();
    });

    it('restore() removes installed globals that were not previously set', () => {
        // Use a fresh key that definitely does not exist on globalThis
        // We test indirectly: clear matchMedia, install, then restore
        delete globalThis.matchMedia;
        expect(globalThis.matchMedia).toBeUndefined();

        const dom = new JSDOM('<!DOCTYPE html><html><body></body></html>');
        const result = installJsdomGlobals(dom);
        expect(globalThis.matchMedia).toBeDefined();

        result.restore();
        expect(globalThis.matchMedia).toBeUndefined();
    });

    it('restore() brings back original values', () => {
        const originalWindow = globalThis.window;
        const originalDocument = globalThis.document;

        const dom = new JSDOM('<!DOCTYPE html><html><body></body></html>');
        const result = installJsdomGlobals(dom);

        // Globals should now point to the new dom
        expect(globalThis.window).toBe(dom.window);
        expect(globalThis.document).toBe(dom.window.document);

        result.restore();

        // Globals should be restored to their original values
        expect(globalThis.window).toBe(originalWindow);
        expect(globalThis.document).toBe(originalDocument);
    });

    it('installs DOMRect polyfill', () => {
        delete globalThis.DOMRect;

        const dom = new JSDOM('<!DOCTYPE html><html><body></body></html>');
        const result = installJsdomGlobals(dom);

        expect(typeof globalThis.DOMRect).toBe('function');
        const rect = new globalThis.DOMRect(10, 20, 100, 50);
        expect(rect.x).toBe(10);
        expect(rect.y).toBe(20);
        expect(rect.width).toBe(100);
        expect(rect.height).toBe(50);
        expect(rect.top).toBe(20);
        expect(rect.right).toBe(110);
        expect(rect.bottom).toBe(70);
        expect(rect.left).toBe(10);

        result.restore();
    });

    it('installs getAnimations polyfill on document and Element.prototype', () => {
        const dom = new JSDOM('<!DOCTYPE html><html><body></body></html>');

        // Ensure they don't exist before install
        expect(dom.window.document.getAnimations).toBeUndefined();
        expect(dom.window.Element.prototype.getAnimations).toBeUndefined();

        const result = installJsdomGlobals(dom);

        // After install, getAnimations should be polyfilled
        expect(typeof dom.window.document.getAnimations).toBe('function');
        expect(dom.window.document.getAnimations()).toEqual([]);

        expect(typeof dom.window.Element.prototype.getAnimations).toBe('function');
        const el = dom.window.document.createElement('div');
        expect(el.getAnimations()).toEqual([]);

        result.restore();
    });
});
