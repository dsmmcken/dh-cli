import { describe, it, expect, beforeAll } from 'vitest';
import { render, ensureComponentSession } from '../helpers/cli-commands.mjs';

describe.runIf(globalThis.__DH_SERVER_AVAILABLE__)('panel component', () => {
    beforeAll(() => ensureComponentSession());
    it('renders panel content', () => {
        const out = render('panel_widget');
        expect(out.includes('Panel Content') || out.includes('panel')).toBe(true);
    });
});
