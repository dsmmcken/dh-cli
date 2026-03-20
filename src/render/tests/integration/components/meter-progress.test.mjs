import { describe, it, expect, beforeAll } from 'vitest';
import { render, ensureComponentSession } from '../helpers/cli-commands.mjs';

describe.runIf(globalThis.__DH_SERVER_AVAILABLE__)('meter and progress component', () => {
    beforeAll(() => ensureComponentSession());
    it('renders with initial progress value', () => {
        const out = render('meter_progress_widget');
        expect(out).toContain('Progress: 35%');
    });
});
