import { describe, it, expect, beforeAll } from 'vitest';
import { render, ensureComponentSession } from '../helpers/cli-commands.mjs';

describe.runIf(globalThis.__DH_SERVER_AVAILABLE__)('sliders component', () => {
    beforeAll(() => ensureComponentSession());
    it('renders with initial value', () => {
        const out = render('sliders_widget');
        expect(out).toContain('Slider: 50');
    });
});
