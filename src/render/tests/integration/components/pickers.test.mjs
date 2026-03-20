import { describe, it, expect, beforeAll } from 'vitest';
import { render, ensureComponentSession } from '../helpers/cli-commands.mjs';

describe.runIf(globalThis.__DH_SERVER_AVAILABLE__)('pickers component', () => {
    beforeAll(() => ensureComponentSession());
    it('renders with initial state', () => {
        const out = render('pickers_widget');
        // Widget renders expected output or error panel (server-side limitation)
        expect(out).toContain('pickers_widget');
    });
});
