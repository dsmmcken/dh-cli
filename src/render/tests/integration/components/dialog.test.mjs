import { describe, it, expect, beforeAll } from 'vitest';
import { render, ensureComponentSession } from '../helpers/cli-commands.mjs';

describe.runIf(globalThis.__DH_SERVER_AVAILABLE__)('dialog component', () => {
    beforeAll(() => ensureComponentSession());
    it('renders dialog trigger', () => {
        const out = render('dialog_widget');
        expect(out.includes('Open Dialog') || out.includes('Action: none')).toBe(true);
    });
});
