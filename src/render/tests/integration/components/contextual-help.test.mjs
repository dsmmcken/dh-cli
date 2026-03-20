import { describe, it, expect, beforeAll } from 'vitest';
import { render, ensureComponentSession } from '../helpers/cli-commands.mjs';

describe.runIf(globalThis.__DH_SERVER_AVAILABLE__)('contextual-help component', () => {
    beforeAll(() => ensureComponentSession());
    it('renders help text', () => {
        const out = render('contextual_help_widget');
        expect(out).toContain('Hover the help');
    });
});
