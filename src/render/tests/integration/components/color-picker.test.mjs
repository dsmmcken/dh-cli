import { describe, it, expect, beforeAll } from 'vitest';
import { render, ensureComponentSession } from '../helpers/cli-commands.mjs';

describe.runIf(globalThis.__DH_SERVER_AVAILABLE__)('color-picker component', () => {
    beforeAll(() => ensureComponentSession());
    it('renders color display', () => {
        const out = render('color_picker_widget');
        expect(out).toContain('Color:');
    });
});
