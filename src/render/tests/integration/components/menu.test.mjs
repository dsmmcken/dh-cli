import { describe, it, expect, beforeAll } from 'vitest';
import { render, ensureComponentSession } from '../helpers/cli-commands.mjs';

describe.runIf(globalThis.__DH_SERVER_AVAILABLE__)('menu component', () => {
    beforeAll(() => ensureComponentSession());
    it('renders with initial state', () => {
        const out = render('menu_widget');
        expect(out).toContain('Menu action: none');
    });
});
