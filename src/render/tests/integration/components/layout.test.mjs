import { describe, it, expect, beforeAll } from 'vitest';
import { render, ensureComponentSession } from '../helpers/cli-commands.mjs';

describe.runIf(globalThis.__DH_SERVER_AVAILABLE__)('layout component', () => {
    beforeAll(() => ensureComponentSession());
    it('renders flex and grid layout', () => {
        const out = render('layout_widget');
        expect(out).toContain('Flex Row');
        expect(out).toContain('Grid Layout');
        expect(out).toContain('Item 1');
    });
});
