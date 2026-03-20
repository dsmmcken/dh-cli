import { describe, it, expect, beforeAll } from 'vitest';
import { render, ensureComponentSession } from '../helpers/cli-commands.mjs';

describe.runIf(globalThis.__DH_SERVER_AVAILABLE__)('inline-alert component', () => {
    beforeAll(() => ensureComponentSession());
    it('renders inline alert', () => {
        const out = render('inline_alert_widget');
        expect(out).toContain('[alert]');
        expect(out).toContain('Success');
        expect(out).toContain('Operation completed.');
    });
});
