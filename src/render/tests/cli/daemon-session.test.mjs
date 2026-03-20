import { describe, it, expect, beforeEach, afterEach, afterAll } from 'vitest';
import { JSDOM } from 'jsdom';
import { DaemonSession } from '../../src/cli/session.mjs';

/**
 * Helper: create a DaemonSession with a fake body containing the given HTML.
 * This lets us test _resolveTarget and snapshot without a real server.
 */
function sessionWithBody(html) {
    const dom = new JSDOM(`<body>${html}</body>`);
    const session = new DaemonSession();
    // Wire up a fake _renderResult so session._body returns the DOM body
    session._renderResult = {
        body: dom.window.document.body,
        unmount() {},
        flush: async () => {},
    };
    return session;
}

describe('DaemonSession', () => {
    let session;

    beforeEach(() => {
        session = new DaemonSession();
    });

    afterEach(() => {
        // Ensure session is cleaned up even if test fails
        try {
            session.close();
        } catch (_) { /* ignore */ }
    });

    describe('status/state', () => {
        it('initial status shows not connected', () => {
            const result = session.status();
            expect(result.ok).toBe(true);
            expect(result.connected).toBe(false);
            expect(result.serverUrl).toBeNull();
            expect(result.widget).toBeNull();
            expect(result.hasRender).toBe(false);
        });

        it('status shows all null/false initial values', () => {
            const result = session.status();
            expect(result).toEqual({
                ok: true,
                connected: false,
                serverUrl: null,
                widget: null,
                widgetType: null,
                hasRender: false,
                exportedObjects: 0,
            });
        });
    });

    describe('render', () => {
        it('returns error when not connected', async () => {
            const result = await session.render('some_widget');
            expect(result.ok).toBe(false);
            expect(result.error).toMatch(/Not connected/);
        });
    });

    describe('snapshot', () => {
        it('returns error when not rendered', () => {
            const result = session.snapshot();
            expect(result.ok).toBe(false);
            expect(result.error).toMatch(/No widget rendered/);
        });
    });

    describe('click', () => {
        it('returns error when not rendered', async () => {
            const result = await session.click('Some Button');
            expect(result.ok).toBe(false);
            expect(result.error).toMatch(/No widget rendered/);
        });
    });

    describe('fill', () => {
        it('returns error when not rendered', async () => {
            const result = await session.fill('Some Input', 'value');
            expect(result.ok).toBe(false);
            expect(result.error).toMatch(/No widget rendered/);
        });
    });

    describe('close', () => {
        it('succeeds on unconnected session', () => {
            const result = session.close();
            expect(result.ok).toBe(true);
            expect(result.message).toMatch(/closed/i);
        });
    });

    describe('call', () => {
        it('returns not-available error', async () => {
            const result = await session.call('cb0', []);
            expect(result.ok).toBe(false);
            expect(result.error).toMatch(/not available/i);
        });
    });

    describe('html', () => {
        it('returns error when not rendered', () => {
            const result = session.html();
            expect(result.ok).toBe(false);
            expect(result.error).toMatch(/No widget rendered/);
        });
    });

    describe('wait', () => {
        it('returns error when not rendered', async () => {
            const result = await session.wait();
            expect(result.ok).toBe(false);
            expect(result.error).toMatch(/No widget rendered/);
        });
    });

    describe('_resolveTarget — exact vs substring matching', () => {
        it('clicking "A" targets the "A" button, not "Action"', () => {
            const s = sessionWithBody(`
                <button>Action</button>
                <button>A</button>
                <button>B</button>
            `);
            const el = s._resolveTarget('A', ['button']);
            expect(el).not.toBeNull();
            expect(el.textContent.trim()).toBe('A');
        });

        it('clicking "B" targets the "B" button, not a longer name', () => {
            const s = sessionWithBody(`
                <button>Bold</button>
                <button>B</button>
            `);
            const el = s._resolveTarget('B', ['button']);
            expect(el).not.toBeNull();
            expect(el.textContent.trim()).toBe('B');
        });

        it('clicking "Action" still works (exact match exists)', () => {
            const s = sessionWithBody(`
                <button>Action</button>
                <button>A</button>
            `);
            const el = s._resolveTarget('Action', ['button']);
            expect(el).not.toBeNull();
            expect(el.textContent.trim()).toBe('Action');
        });

        it('clicking "Primary" still matches as substring when no exact match', () => {
            const s = sessionWithBody(`
                <button>Primary Button</button>
            `);
            const el = s._resolveTarget('Primary', ['button']);
            expect(el).not.toBeNull();
            expect(el.textContent.trim()).toBe('Primary Button');
        });

        it('clicking quoted target "A" resolves correctly with aria-label', () => {
            const s = sessionWithBody(`
                <button aria-label="Action">Action</button>
                <button aria-label="A">A</button>
            `);
            const el = s._resolveTarget('A', ['button']);
            expect(el).not.toBeNull();
            expect(el.getAttribute('aria-label')).toBe('A');
        });

        it('prefers exact match over earlier substring match', () => {
            const s = sessionWithBody(`
                <button>or</button>
                <button>orange</button>
                <button>order</button>
            `);
            const el = s._resolveTarget('or', ['button']);
            expect(el).not.toBeNull();
            expect(el.textContent.trim()).toBe('or');
        });

        it('returns null when no match exists', () => {
            const s = sessionWithBody(`
                <button>Action</button>
                <button>Bold</button>
            `);
            const el = s._resolveTarget('Z', ['button']);
            expect(el).toBeNull();
        });
    });
});
