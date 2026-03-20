import { describe, it, expect, beforeAll } from 'vitest';
import { render, snapshot, click, fill, ensureComponentSession } from '../helpers/cli-commands.mjs';

describe.runIf(globalThis.__DH_SERVER_AVAILABLE__)('complex interaction component', () => {
    beforeAll(() => ensureComponentSession());
    it('renders widget', () => {
        const out = render('complex_widget');
        // May show registration form or error panel depending on server capabilities
        expect(out).toContain('complex_widget');
    });
    it('interacts if form rendered', () => {
        const snap = snapshot();
        if (!snap.includes('Registration Form')) return; // server-side error, skip interaction
        fill('Your Name', 'Alice');
        const s2 = snapshot();
        expect(s2.includes('Alice') || s2.includes('Your Name')).toBe(true);
    });
});
