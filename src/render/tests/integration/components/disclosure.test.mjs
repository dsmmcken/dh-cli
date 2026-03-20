import { describe, it, expect, beforeAll } from 'vitest';
import { render, ensureComponentSession } from '../helpers/cli-commands.mjs';

describe.runIf(globalThis.__DH_SERVER_AVAILABLE__)('disclosure component', () => {
    beforeAll(() => ensureComponentSession());
    it('renders collapsed state', () => {
        const out = render('disclosure_widget');
        expect(out).toContain('Expanded: False');
    });
});
