import { describe, it, expect, beforeAll } from 'vitest';
import { render, ensureComponentSession } from '../helpers/cli-commands.mjs';

describe.runIf(globalThis.__DH_SERVER_AVAILABLE__)('checkboxes component', () => {
    beforeAll(() => ensureComponentSession());
    it('renders with initial state', () => {
        const out = render('checkboxes_widget');
        expect(out).toContain('Checked: False');
        expect(out).toContain('Switched: False');
        expect(out).toContain('Radio: A');
    });
});
