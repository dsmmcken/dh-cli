import { describe, it, expect, beforeAll } from 'vitest';
import { render, ensureComponentSession } from '../helpers/cli-commands.mjs';

describe.runIf(globalThis.__DH_SERVER_AVAILABLE__)('markdown component', () => {
    beforeAll(() => ensureComponentSession());
    it('renders markdown content', () => {
        const out = render('markdown_widget');
        expect(out.includes('Hello Markdown') || out.includes('markdown')).toBe(true);
    });
});
