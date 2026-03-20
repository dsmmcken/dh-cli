import { describe, it, expect, beforeEach } from 'vitest';
import { buildSnapshot } from '../../src/cli/snapshot.mjs';
import { JSDOM } from 'jsdom';

let dom, doc;

beforeEach(() => {
    dom = new JSDOM('<!DOCTYPE html><html><body></body></html>');
    doc = dom.window.document;
});

/**
 * Helper to build DOM elements concisely.
 */
function el(tag, attrs = {}, ...children) {
    const e = doc.createElement(tag);
    for (const [k, v] of Object.entries(attrs)) e.setAttribute(k, v);
    for (const c of children) {
        if (typeof c === 'string') e.appendChild(doc.createTextNode(c));
        else e.appendChild(c);
    }
    return e;
}

describe('buildSnapshot – complex DOM trees', () => {
    it('Spectrum-style button panel: div > div[role=none] > button', () => {
        const container = el('div', {},
            el('div', { role: 'none' },
                el('button', {}, 'Click Me'),
            ),
        );
        const result = buildSnapshot(container);
        expect(result.text).toContain('[button]');
        expect(result.text).toContain('"Click Me"');
        expect(result.text).toContain('@e1');
        expect(result.refs.has('@e1')).toBe(true);
        expect(result.refs.get('@e1').tagName.toLowerCase()).toBe('button');
        // role="none" wrapper should not appear as [none]
        expect(result.text).not.toContain('[none]');
    });

    it('form with labeled inputs shows textbox with value', () => {
        const container = el('div', { role: 'form' },
            el('label', {}, 'Username'),
            el('input', { type: 'text', value: 'alice', 'aria-label': 'Username' }),
        );
        const result = buildSnapshot(container);
        expect(result.text).toContain('[label]');
        expect(result.text).toContain('"Username"');
        expect(result.text).toContain('[textbox]');
        expect(result.text).toContain('value="alice"');
        expect(result.interactiveCount).toBe(1); // only the input is interactive
    });

    it('checkbox group with checked and unchecked states', () => {
        const container = el('div', { role: 'group', 'aria-label': 'Options' },
            (() => {
                const cb1 = el('input', { type: 'checkbox', 'aria-label': 'Opt A' });
                cb1.checked = true;
                return cb1;
            })(),
            (() => {
                const cb2 = el('input', { type: 'checkbox', 'aria-label': 'Opt B' });
                cb2.checked = false;
                return cb2;
            })(),
            (() => {
                const cb3 = el('input', { type: 'checkbox', 'aria-label': 'Opt C' });
                cb3.checked = true;
                return cb3;
            })(),
        );
        const result = buildSnapshot(container);
        const lines = result.text.split('\n');

        // Should have a group
        expect(result.text).toContain('[group]');
        expect(result.text).toContain('"Options"');

        // Three checkboxes with correct states
        const checkboxLines = lines.filter(l => l.includes('[checkbox]'));
        expect(checkboxLines).toHaveLength(3);
        expect(checkboxLines[0]).toContain('[checked]');
        expect(checkboxLines[0]).toContain('"Opt A"');
        expect(checkboxLines[1]).toContain('[unchecked]');
        expect(checkboxLines[1]).toContain('"Opt B"');
        expect(checkboxLines[2]).toContain('[checked]');
        expect(checkboxLines[2]).toContain('"Opt C"');
        expect(result.interactiveCount).toBe(3);
    });

    it('nested panels with GoldenLayout chrome skipped', () => {
        const container = el('div', {},
            // GoldenLayout chrome — should be skipped entirely
            el('div', { class: 'lm_header' },
                el('div', { class: 'lm_tabs' },
                    el('button', {}, 'Tab Header'),
                ),
                el('div', { class: 'lm_controls' },
                    el('button', {}, 'Close'),
                ),
            ),
            // Actual widget content
            el('div', { class: 'dh-react-panel' },
                el('h2', {}, 'My Panel'),
                el('button', {}, 'Action'),
            ),
        );
        const result = buildSnapshot(container);

        // GoldenLayout chrome buttons should not appear
        expect(result.text).not.toContain('"Tab Header"');
        expect(result.text).not.toContain('"Close"');

        // Widget content should appear
        expect(result.text).toContain('[heading]');
        expect(result.text).toContain('"My Panel"');
        expect(result.text).toContain('[button]');
        expect(result.text).toContain('"Action"');
        // Only the Action button should be counted (GoldenLayout ones skipped)
        expect(result.interactiveCount).toBe(1);
    });

    it('full GoldenLayout structure emits layout markers', () => {
        const container = el('div', {},
            el('div', { class: 'lm_root' },
                el('div', { class: 'lm_item lm_column' },
                    el('div', { class: 'lm_item lm_row' },
                        el('div', { class: 'lm_item lm_stack' },
                            el('div', { class: 'lm_header' },
                                el('div', { class: 'lm_tabs' },
                                    el('div', { class: 'lm_tab lm_active' },
                                        el('span', { class: 'lm_title' }, 'Info'),
                                    ),
                                ),
                            ),
                            el('div', { class: 'lm_items' },
                                el('div', {},
                                    el('h2', {}, 'Welcome'),
                                ),
                            ),
                        ),
                        el('div', { class: 'lm_splitter' }),
                        el('div', { class: 'lm_item lm_stack' },
                            el('div', { class: 'lm_header' },
                                el('div', { class: 'lm_tabs' },
                                    el('div', { class: 'lm_tab lm_active' },
                                        el('span', { class: 'lm_title' }, 'Data'),
                                    ),
                                    el('div', { class: 'lm_tab' },
                                        el('span', { class: 'lm_title' }, 'Chart'),
                                    ),
                                ),
                            ),
                            el('div', { class: 'lm_items' },
                                el('div', {},
                                    el('button', {}, 'Load Data'),
                                ),
                                el('div', {},
                                    el('button', {}, 'Refresh Chart'),
                                ),
                            ),
                        ),
                    ),
                ),
            ),
        );
        const result = buildSnapshot(container);
        const lines = result.text.split('\n');

        // Verify layout hierarchy
        expect(lines[0]).toBe('[dashboard]');
        expect(lines[1]).toBe('  [column]');
        expect(lines[2]).toBe('    [row]');

        // Single-tab stack: [panel] without [stack]
        expect(result.text).toContain('[panel] "Info"');
        expect(result.text).toContain('[heading] "Welcome"');

        // Multi-tab stack: [stack] > [panel] with [active]
        expect(result.text).toContain('[stack]');
        expect(result.text).toContain('[panel] "Data" [active]');
        expect(result.text).toContain('[panel] "Chart"');
        expect(result.text).toContain('[button] "Load Data"');
        expect(result.text).toContain('[button] "Refresh Chart"');

        // Splitter skipped
        expect(result.text).not.toContain('splitter');

        expect(result.interactiveCount).toBe(2);
    });

    it('tab interface with tablist and tabs', () => {
        const container = el('div', {},
            el('div', { role: 'tablist', 'aria-label': 'Settings' },
                el('div', { role: 'tab', 'aria-label': 'General' }, 'General'),
                el('div', { role: 'tab', 'aria-label': 'Advanced' }, 'Advanced'),
                el('div', { role: 'tab', 'aria-label': 'About' }, 'About'),
            ),
            el('div', { role: 'tabpanel' },
                el('p', {}, 'General settings content'),
            ),
        );
        const result = buildSnapshot(container);
        const lines = result.text.split('\n');

        expect(result.text).toContain('[tablist]');
        expect(result.text).toContain('"Settings"');

        const tabLines = lines.filter(l => l.includes('[tab]'));
        expect(tabLines).toHaveLength(3);
        expect(tabLines[0]).toContain('"General"');
        expect(tabLines[1]).toContain('"Advanced"');
        expect(tabLines[2]).toContain('"About"');

        // Each tab should get a ref
        expect(result.refs.has('@e1')).toBe(true);
        expect(result.refs.has('@e2')).toBe(true);
        expect(result.refs.has('@e3')).toBe(true);
        expect(result.interactiveCount).toBe(3);

        // Tab panel content
        expect(result.text).toContain('[tabpanel]');
        expect(result.text).toContain('"General settings content"');
    });

    it('mixed interactive and display: buttons, text, headings', () => {
        const container = el('div', {},
            el('h1', {}, 'Dashboard'),
            el('p', {}, 'Welcome to the app'),
            el('div', { role: 'toolbar', 'aria-label': 'Actions' },
                el('button', {}, 'Save'),
                el('button', {}, 'Delete'),
            ),
            el('span', {}, 'Last saved: 5 min ago'),
            el('a', { href: '/help' }, 'Help'),
        );
        const result = buildSnapshot(container);

        // Headings
        expect(result.text).toContain('[heading]');
        expect(result.text).toContain('"Dashboard"');

        // Plain text
        expect(result.text).toContain('"Welcome to the app"');
        expect(result.text).toContain('"Last saved: 5 min ago"');

        // Toolbar
        expect(result.text).toContain('[toolbar]');
        expect(result.text).toContain('"Actions"');

        // Buttons
        expect(result.text).toContain('"Save"');
        expect(result.text).toContain('"Delete"');

        // Link
        expect(result.text).toContain('[link]');
        expect(result.text).toContain('"Help"');

        // 2 buttons + 1 link = 3 interactive
        expect(result.interactiveCount).toBe(3);
    });

    it('real-world-like tree with 5+ levels of nesting', () => {
        const container = el('div', { role: 'application', 'aria-label': 'App' },
            el('div', { role: 'navigation', 'aria-label': 'Main Nav' },
                el('a', { href: '/', role: 'link' }, 'Home'),
                el('a', { href: '/settings', role: 'link' }, 'Settings'),
            ),
            el('div', { role: 'main' },
                el('div', { role: 'region', 'aria-label': 'Content' },
                    el('h1', {}, 'Page Title'),
                    el('div', { role: 'form', 'aria-label': 'User Form' },
                        el('label', {}, 'Name'),
                        el('input', { type: 'text', 'aria-label': 'Name', value: 'Bob' }),
                        el('label', {}, 'Email'),
                        el('input', { type: 'text', 'aria-label': 'Email', value: 'bob@test.com' }),
                        el('div', { role: 'group', 'aria-label': 'Preferences' },
                            (() => {
                                const cb = el('input', { type: 'checkbox', 'aria-label': 'Newsletter' });
                                cb.checked = true;
                                return cb;
                            })(),
                            (() => {
                                const cb = el('input', { type: 'checkbox', 'aria-label': 'Notifications' });
                                cb.checked = false;
                                return cb;
                            })(),
                        ),
                        el('div', { role: 'none' },
                            el('button', {}, 'Submit'),
                        ),
                    ),
                ),
            ),
            el('div', { role: 'contentinfo', 'aria-label': 'Footer' },
                el('span', {}, '© 2026 Test Corp'),
            ),
        );

        const result = buildSnapshot(container);
        const lines = result.text.split('\n');

        // Top-level structure
        expect(result.text).toContain('[application]');
        expect(result.text).toContain('"App"');
        expect(result.text).toContain('[navigation]');
        expect(result.text).toContain('"Main Nav"');
        expect(result.text).toContain('[main]');
        expect(result.text).toContain('[region]');
        expect(result.text).toContain('"Content"');

        // Form elements
        expect(result.text).toContain('[form]');
        expect(result.text).toContain('"User Form"');
        expect(result.text).toContain('[label] "Name"');
        expect(result.text).toContain('[textbox] "Name" value="Bob"');
        expect(result.text).toContain('[label] "Email"');
        expect(result.text).toContain('[textbox] "Email" value="bob@test.com"');

        // Checkbox group
        expect(result.text).toContain('[group]');
        expect(result.text).toContain('"Preferences"');
        expect(result.text).toContain('[checkbox] "Newsletter" [checked]');
        expect(result.text).toContain('[checkbox] "Notifications" [unchecked]');

        // Button through role="none" wrapper
        expect(result.text).toContain('[button] "Submit"');
        expect(result.text).not.toContain('[none]');

        // Footer
        expect(result.text).toContain('[contentinfo]');
        expect(result.text).toContain('"Footer"');

        // Interactive count: 2 links + 2 textboxes + 2 checkboxes + 1 button = 7
        expect(result.interactiveCount).toBe(7);

        // Verify nesting depth — the submit button should be deeply indented
        const submitLine = lines.find(l => l.includes('"Submit"'));
        expect(submitLine).toBeDefined();
        // It should be indented (at least 4 levels: application > main > region > form > [none transparent] > button)
        const leadingSpaces = submitLine.match(/^(\s*)/)[1].length;
        expect(leadingSpaces).toBeGreaterThanOrEqual(8); // at least 4 levels of 2-space indent

        // Verify refs are assigned for all interactive elements
        expect(result.refs.size).toBe(7);
        for (let i = 1; i <= 7; i++) {
            expect(result.refs.has(`@e${i}`)).toBe(true);
        }
    });
});
