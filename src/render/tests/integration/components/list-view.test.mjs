import { describe, it, expect, beforeAll } from 'vitest';
import { render, ensureComponentSession } from '../helpers/cli-commands.mjs';

describe.runIf(globalThis.__DH_SERVER_AVAILABLE__)('list-view component', () => {
    beforeAll(() => ensureComponentSession());
    it('renders list view widget', () => {
        const out = render('list_view_widget');
        expect(out).toContain('list_view_widget');
    });
});
