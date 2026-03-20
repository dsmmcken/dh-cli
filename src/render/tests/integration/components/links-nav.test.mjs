import { describe, it, expect, beforeAll } from 'vitest';
import { render, ensureComponentSession } from '../helpers/cli-commands.mjs';

describe.runIf(globalThis.__DH_SERVER_AVAILABLE__)('links-nav component', () => {
    beforeAll(() => ensureComponentSession());
    it('renders breadcrumb', () => {
        const out = render('links_nav_widget');
        expect(out).toContain('Breadcrumb:');
    });
});
