import { describe, it, expect, beforeAll } from 'vitest';
import { render, snapshot, click, ensureComponentSession } from '../helpers/cli-commands.mjs';

describe.runIf(globalThis.__DH_SERVER_AVAILABLE__)('button component', () => {
    beforeAll(() => ensureComponentSession());

    it('renders with initial state', () => {
        const out = render('button_widget');
        expect(out).toContain('Button clicks: 0');
        expect(out).toContain('[button] "Primary"');
        expect(out).toContain('[button] "Secondary"');
        expect(out).toContain('[button] "Action"');
        expect(out).toContain('[button] "A"');
        expect(out).toContain('[button] "B"');
        expect(out).toContain('[button] "Toggle"');
        expect(out).toContain('Toggle: False');
        expect(out).toContain('[button] "or"');
        expect(out).toContain('Logic: or');
    });

    it('Primary click increments by 1', () => {
        click('"Primary"');
        expect(snapshot()).toContain('Button clicks: 1');
    });

    it('Secondary click increments by 2', () => {
        click('"Secondary"');
        expect(snapshot()).toContain('Button clicks: 3');
    });

    it('Action click increments by 10', () => {
        click('"Action"');
        expect(snapshot()).toContain('Button clicks: 13');
    });

    it('button "A" increments by 100 (not confused with "Action")', () => {
        click('"A"');
        expect(snapshot()).toContain('Button clicks: 113');
    });

    it('button "B" increments by 200', () => {
        click('"B"');
        expect(snapshot()).toContain('Button clicks: 313');
    });

    it('Toggle changes state', () => {
        click('"Toggle"');
        expect(snapshot()).toContain('Toggle: True');
    });

    it('Logic button toggles variant', () => {
        click('"or"');
        const snap = snapshot();
        expect(snap).toContain('Logic: and');
        expect(snap).toContain('[button] "and"');
    });
});
