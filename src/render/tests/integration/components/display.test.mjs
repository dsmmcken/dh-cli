import { describe, it, expect, beforeAll } from 'vitest';
import { render, ensureComponentSession } from '../helpers/cli-commands.mjs';

describe.runIf(globalThis.__DH_SERVER_AVAILABLE__)('display component', () => {
    beforeAll(() => ensureComponentSession());
    it('renders display widget', () => {
        const out = render('display_widget');
        expect(out).toContain('display_widget');
    });
});
