import { describe, it, expect, beforeAll } from 'vitest';
import { render, snapshot, click, ensureComponentSession } from '../helpers/cli-commands.mjs';

describe.runIf(globalThis.__DH_SERVER_AVAILABLE__)('fragment component', () => {
    beforeAll(() => ensureComponentSession());
    it('renders with heading and initial count', () => {
        const out = render('fragment_widget');
        expect(out).toContain('Fragment Test');
        expect(out).toContain('Count: 0');
    });
    it('increment button works', () => {
        click('"Increment"');
        expect(snapshot()).toContain('Count: 1');
    });
});
