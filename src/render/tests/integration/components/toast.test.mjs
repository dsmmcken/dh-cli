import { describe, it, expect, beforeAll } from 'vitest';
import { render, snapshot, click, ensureComponentSession } from '../helpers/cli-commands.mjs';

describe.runIf(globalThis.__DH_SERVER_AVAILABLE__)('toast component', () => {
    beforeAll(() => ensureComponentSession());
    it('renders with initial state', () => {
        const out = render('toast_widget');
        expect(out).toContain('Last toast: none');
    });
    it('show info toast works', () => {
        click('"Show Info Toast"');
        expect(snapshot()).toContain('Last toast: info');
    });
});
