import { describe, it, expect, beforeAll } from 'vitest';
import { render, ensureComponentSession } from '../helpers/cli-commands.mjs';

describe.runIf(globalThis.__DH_SERVER_AVAILABLE__)('tag-group component', () => {
    beforeAll(() => ensureComponentSession());
    it('renders tags', () => {
        const out = render('tag_group_widget');
        expect(out).toContain('Tags:');
    });
});
