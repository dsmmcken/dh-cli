import { describe, it, expect, beforeAll } from 'vitest';
import { render, ensureComponentSession } from '../helpers/cli-commands.mjs';

describe.runIf(globalThis.__DH_SERVER_AVAILABLE__)('accordion component', () => {
    beforeAll(() => ensureComponentSession());
    it('renders accordion sections', () => {
        const out = render('accordion_widget');
        expect(out.includes('Section 1') || out.includes('accordion')).toBe(true);
    });
});
