import { describe, it, expect, beforeAll } from 'vitest';
import { render, snapshot, click, html, ensureComponentSession } from '../helpers/cli-commands.mjs';

describe.runIf(globalThis.__DH_SERVER_AVAILABLE__)('html component', () => {
    beforeAll(() => ensureComponentSession());
    it('renders HTML elements text', () => {
        const out = render('html_widget');
        expect(out).toContain('HTML Elements Test');
        expect(out).toContain('bold');
        expect(out).toContain('italic');
        expect(out).toContain('First item');
        expect(out).toContain('Nested span inside div');
        expect(out).toContain('Count: 0');
        expect(out).toContain('x = 42');
    });
    it('HTML elements render as real tags', () => {
        const h = html();
        // HTML tags have __dhid attributes, so check for tag opening without >
        expect(h).toContain('<h1');
        expect(h).toContain('<p');
        expect(h).toContain('<b');
        expect(h).toContain('<i');
        expect(h).toContain('<ul');
        expect(h).toContain('<li');
        expect(h).toContain('<hr');
        expect(h).toContain('<pre');
        expect(h).toContain('<code');
    });
    it('no data-unknown fallbacks for HTML elements', () => {
        const h = html();
        const unknowns = h.match(/deephaven\.ui\.html\.\w+" data-unknown/g);
        expect(unknowns).toBeNull();
    });
    it('DH button works alongside HTML elements', () => {
        click('"Increment"');
        expect(snapshot()).toContain('Count: 1');
    });
});
