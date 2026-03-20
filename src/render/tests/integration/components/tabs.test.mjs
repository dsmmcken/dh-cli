import { describe, it, expect, beforeAll } from 'vitest';
import { render, ensureComponentSession } from '../helpers/cli-commands.mjs';

describe.runIf(globalThis.__DH_SERVER_AVAILABLE__)('tabs component', () => {
    beforeAll(() => ensureComponentSession());
    it('renders tabs widget', () => {
        const out = render('tabs_widget');
        expect(out).toContain('tabs_widget');
    });
});
