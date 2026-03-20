import { describe, it, expect, beforeAll } from 'vitest';
import { render, ensureComponentSession } from '../helpers/cli-commands.mjs';

describe.runIf(globalThis.__DH_SERVER_AVAILABLE__)('table component', () => {
    beforeAll(() => ensureComponentSession());
    it('renders table heading', () => {
        const out = render('table_widget');
        expect(out).toContain('Table Component');
    });
});
