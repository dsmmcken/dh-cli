import { describe, it, expect, beforeAll } from 'vitest';
import { render, ensureComponentSession } from '../helpers/cli-commands.mjs';

describe.runIf(globalThis.__DH_SERVER_AVAILABLE__)('date-time component', () => {
    beforeAll(() => ensureComponentSession());
    it('renders date-time widget', () => {
        const out = render('date_time_widget');
        expect(out).toContain('date_time_widget');
    });
});
