import { describe, it, expect, beforeEach } from 'vitest';
import { buildSnapshot } from '../../src/cli/snapshot.mjs';

const doc = globalThis.__TEST_DOM__.window.document;

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

describe('buildSnapshot', () => {
    let container;

    beforeEach(() => {
        container = doc.createElement('div');
    });

    it('returns empty text for an empty container', () => {
        const result = buildSnapshot(container);
        expect(result.text).toBe('');
        expect(result.refs.size).toBe(0);
        expect(result.interactiveCount).toBe(0);
    });

    it('assigns [button] role and @e1 ref to a button element', () => {
        container.appendChild(el('button', {}, 'Click'));
        const result = buildSnapshot(container);
        expect(result.text).toContain('[button]');
        expect(result.text).toContain('"Click"');
        expect(result.text).toContain('@e1');
        expect(result.refs.has('@e1')).toBe(true);
        expect(result.refs.get('@e1').tagName.toLowerCase()).toBe('button');
    });

    it('shows value for a textbox input', () => {
        const input = el('input', { type: 'text', value: 'hello' });
        container.appendChild(input);
        const result = buildSnapshot(container);
        expect(result.text).toContain('[textbox]');
        expect(result.text).toContain('value="hello"');
        expect(result.refs.has('@e1')).toBe(true);
    });

    it('shows [checked] state for a checked checkbox', () => {
        const cb = el('input', { type: 'checkbox' });
        cb.checked = true;
        container.appendChild(cb);
        const result = buildSnapshot(container);
        expect(result.text).toContain('[checkbox]');
        expect(result.text).toContain('[checked]');
        expect(result.refs.has('@e1')).toBe(true);
    });

    it('shows [unchecked] state for an unchecked checkbox', () => {
        const cb = el('input', { type: 'checkbox' });
        cb.checked = false;
        container.appendChild(cb);
        const result = buildSnapshot(container);
        expect(result.text).toContain('[checkbox]');
        expect(result.text).toContain('[unchecked]');
    });

    it('assigns [heading] role to h1-h6 elements', () => {
        container.appendChild(el('h1', {}, 'Title'));
        container.appendChild(el('h3', {}, 'Subtitle'));
        const result = buildSnapshot(container);
        const lines = result.text.split('\n');
        expect(lines[0]).toContain('[heading]');
        expect(lines[0]).toContain('"Title"');
        expect(lines[1]).toContain('[heading]');
        expect(lines[1]).toContain('"Subtitle"');
    });

    it('assigns [link] role and @ref to an anchor element', () => {
        container.appendChild(el('a', { href: '#' }, 'Home'));
        const result = buildSnapshot(container);
        expect(result.text).toContain('[link]');
        expect(result.text).toContain('"Home"');
        expect(result.refs.has('@e1')).toBe(true);
    });

    it('produces indented tree for nested elements', () => {
        const wrapper = el('div', { role: 'toolbar' },
            el('button', {}, 'Save'),
            el('button', {}, 'Cancel'),
        );
        container.appendChild(wrapper);
        const result = buildSnapshot(container);
        const lines = result.text.split('\n');
        // toolbar line should not be indented
        expect(lines[0]).toMatch(/^\[toolbar\]/);
        // buttons should be indented under toolbar
        expect(lines[1]).toMatch(/^\s+@e1 \[button\] "Save"/);
        expect(lines[2]).toMatch(/^\s+@e2 \[button\] "Cancel"/);
    });

    it('skips GoldenLayout chrome elements with class lm_header', () => {
        container.appendChild(el('div', { class: 'lm_header' },
            el('button', {}, 'Should be skipped'),
        ));
        container.appendChild(el('button', {}, 'Visible'));
        const result = buildSnapshot(container);
        expect(result.text).not.toContain('Should be skipped');
        expect(result.text).toContain('"Visible"');
        expect(result.interactiveCount).toBe(1);
    });

    it('assigns sequential @eN refs to multiple interactive elements', () => {
        container.appendChild(el('button', {}, 'First'));
        container.appendChild(el('button', {}, 'Second'));
        container.appendChild(el('a', { href: '#' }, 'Third'));
        const result = buildSnapshot(container);
        expect(result.refs.has('@e1')).toBe(true);
        expect(result.refs.has('@e2')).toBe(true);
        expect(result.refs.has('@e3')).toBe(true);
        expect(result.text).toContain('@e1');
        expect(result.text).toContain('@e2');
        expect(result.text).toContain('@e3');
    });

    it('treats role="none" as transparent — shows children but not the element', () => {
        const wrapper = el('div', { role: 'none' },
            el('button', {}, 'Inner'),
        );
        container.appendChild(wrapper);
        const result = buildSnapshot(container);
        // The role="none" wrapper itself should not appear
        expect(result.text).not.toContain('[none]');
        // But its child button should be visible
        expect(result.text).toContain('[button]');
        expect(result.text).toContain('"Inner"');
    });

    it('role="none" with mixed text and child elements shows both', () => {
        // Simulates Spectrum WidgetErrorView: <span role="none">error msg<button>Info</button></span>
        const span = doc.createElement('span');
        span.setAttribute('role', 'none');
        span.appendChild(doc.createTextNode('division by zero'));
        span.appendChild(el('button', { 'aria-label': 'Information' }));
        container.appendChild(span);
        const result = buildSnapshot(container);
        expect(result.text).toContain('division by zero');
        expect(result.text).toContain('[button]');
    });

    it('shows selected value for combobox via data-selectedkey', () => {
        const select = el('select', { 'data-selectedkey': 'opt2' },
            el('option', { value: 'opt1' }, 'Option 1'),
            el('option', { value: 'opt2' }, 'Option 2'),
        );
        container.appendChild(select);
        const result = buildSnapshot(container);
        expect(result.text).toContain('[combobox]');
        expect(result.text).toContain('selected="opt2"');
    });

    it('assigns [spinbutton] role to input type=number', () => {
        const input = el('input', { type: 'number', value: '42' });
        container.appendChild(input);
        const result = buildSnapshot(container);
        expect(result.text).toContain('[spinbutton]');
        expect(result.text).toContain('value="42"');
        expect(result.refs.has('@e1')).toBe(true);
    });

    it('assigns [slider] role to input type=range', () => {
        const input = el('input', { type: 'range' });
        container.appendChild(input);
        const result = buildSnapshot(container);
        expect(result.text).toContain('[slider]');
        expect(result.refs.has('@e1')).toBe(true);
    });

    it('has interactiveCount matching refs.size', () => {
        container.appendChild(el('button', {}, 'A'));
        container.appendChild(el('input', { type: 'text' }));
        container.appendChild(el('a', { href: '#' }, 'Link'));
        container.appendChild(el('h1', {}, 'Not interactive'));
        const result = buildSnapshot(container);
        expect(result.interactiveCount).toBe(3);
        expect(result.refs.size).toBe(3);
        expect(result.interactiveCount).toBe(result.refs.size);
    });

    it('shows [label] prefix for label elements with text', () => {
        container.appendChild(el('label', {}, 'Username'));
        const result = buildSnapshot(container);
        expect(result.text).toContain('[label]');
        expect(result.text).toContain('"Username"');
    });

    it('uses explicit role attribute over implicit tag role', () => {
        // A div with explicit role="button" should be treated as button
        const div = el('div', { role: 'button' }, 'Custom Button');
        container.appendChild(div);
        const result = buildSnapshot(container);
        expect(result.text).toContain('[button]');
        expect(result.text).toContain('"Custom Button"');
        expect(result.refs.has('@e1')).toBe(true);
    });

    it('shows figure type and trace count for role="figure"', () => {
        const figure = el('div', {
            role: 'figure',
            'data-figure-type': 'scatter',
            'data-trace-count': '3',
        },
            el('span', { 'data-testid': 'figure-ref' }, '[Figure: scatter | 3 traces]'),
            el('button', { 'data-testid': 'figure-expand' }, 'Expand figure'),
        );
        container.appendChild(figure);
        const result = buildSnapshot(container);
        expect(result.text).toContain('[figure]');
        expect(result.text).toContain('scatter');
        expect(result.text).toContain('3 traces');
    });

    it('shows figure without type/count gracefully', () => {
        const figure = el('div', { role: 'figure' },
            el('span', {}, '[Figure #0]'),
        );
        container.appendChild(figure);
        const result = buildSnapshot(container);
        expect(result.text).toContain('[figure]');
    });

    // --- GoldenLayout structure tests ---

    it('single-panel GL layout produces no layout wrappers', () => {
        // A single stack with one tab — should pass through to content
        const root = el('div', { class: 'lm_root' },
            el('div', { class: 'lm_item lm_column' },
                el('div', { class: 'lm_item lm_stack' },
                    el('div', { class: 'lm_header' },
                        el('div', { class: 'lm_tabs' },
                            el('div', { class: 'lm_tab lm_active' },
                                el('span', { class: 'lm_title' }, 'My Panel'),
                            ),
                        ),
                    ),
                    el('div', { class: 'lm_items' },
                        el('div', { class: 'lm_item_container' },
                            el('h1', {}, 'Hello World'),
                            el('button', {}, 'Click'),
                        ),
                    ),
                ),
            ),
        );
        container.appendChild(root);
        const result = buildSnapshot(container);
        // No layout markers
        expect(result.text).not.toContain('[dashboard]');
        expect(result.text).not.toContain('[column]');
        expect(result.text).not.toContain('[row]');
        expect(result.text).not.toContain('[stack]');
        expect(result.text).not.toContain('[panel]');
        // Content should appear at root depth
        expect(result.text).toContain('[heading] "Hello World"');
        expect(result.text).toContain('[button] "Click"');
        expect(result.interactiveCount).toBe(1);
    });

    it('multi-panel GL layout shows [dashboard] > [column] > [row] > [stack] > [panel]', () => {
        const root = el('div', { class: 'lm_root' },
            el('div', { class: 'lm_item lm_column' },
                el('div', { class: 'lm_item lm_row' },
                    // First stack: single tab (no [stack] wrapper)
                    el('div', { class: 'lm_item lm_stack' },
                        el('div', { class: 'lm_header' },
                            el('div', { class: 'lm_tabs' },
                                el('div', { class: 'lm_tab lm_active' },
                                    el('span', { class: 'lm_title' }, 'About'),
                                ),
                            ),
                        ),
                        el('div', { class: 'lm_items' },
                            el('div', {},
                                el('h1', {}, 'About Page'),
                            ),
                        ),
                    ),
                    el('div', { class: 'lm_splitter' }),
                    // Second stack: multiple tabs ([stack] wrapper)
                    el('div', { class: 'lm_item lm_stack' },
                        el('div', { class: 'lm_header' },
                            el('div', { class: 'lm_tabs' },
                                el('div', { class: 'lm_tab lm_active' },
                                    el('span', { class: 'lm_title' }, 'Average'),
                                ),
                                el('div', { class: 'lm_tab' },
                                    el('span', { class: 'lm_title' }, 'Max'),
                                ),
                            ),
                            el('div', { class: 'lm_controls' },
                                el('button', { class: 'lm_close_tab' }, 'x'),
                            ),
                        ),
                        el('div', { class: 'lm_items' },
                            el('div', {},
                                el('button', {}, 'Avg Button'),
                            ),
                            el('div', {},
                                el('button', {}, 'Max Button'),
                            ),
                        ),
                    ),
                ),
            ),
        );
        container.appendChild(root);
        const result = buildSnapshot(container);
        const text = result.text;
        const lines = text.split('\n');

        // Top-level layout markers
        expect(text).toContain('[dashboard]');
        expect(text).toContain('[column]');
        expect(text).toContain('[row]');
        expect(text).toContain('[stack]');

        // Single-tab stack: [panel] without [stack]
        expect(text).toContain('[panel] "About"');
        expect(text).toContain('[heading] "About Page"');

        // Multi-tab stack: [stack] > [panel] with [active]
        expect(text).toContain('[panel] "Average" [active]');
        expect(text).toContain('[panel] "Max"');
        // Max should NOT have [active]
        const maxLine = lines.find(l => l.includes('"Max"') && l.includes('[panel]'));
        expect(maxLine).not.toContain('[active]');

        // Content from both tabs visible
        expect(text).toContain('[button] "Avg Button"');
        expect(text).toContain('[button] "Max Button"');

        // Splitter skipped
        expect(text).not.toContain('lm_splitter');

        // GL chrome skipped (close button)
        expect(text).not.toContain('"x"');
        expect(result.interactiveCount).toBe(2); // Avg Button + Max Button
    });

    it('GL stack shows [active] marker only on the active tab', () => {
        const stack = el('div', { class: 'lm_stack' },
            el('div', { class: 'lm_header' },
                el('div', { class: 'lm_tabs' },
                    el('div', { class: 'lm_tab' },
                        el('span', { class: 'lm_title' }, 'Tab A'),
                    ),
                    el('div', { class: 'lm_tab lm_active' },
                        el('span', { class: 'lm_title' }, 'Tab B'),
                    ),
                    el('div', { class: 'lm_tab' },
                        el('span', { class: 'lm_title' }, 'Tab C'),
                    ),
                ),
            ),
            el('div', { class: 'lm_items' },
                el('div', {}, el('span', {}, 'Content A')),
                el('div', {}, el('span', {}, 'Content B')),
                el('div', {}, el('span', {}, 'Content C')),
            ),
        );
        container.appendChild(stack);
        const result = buildSnapshot(container);
        const lines = result.text.split('\n');

        // Only Tab B should have [active]
        const tabALine = lines.find(l => l.includes('"Tab A"'));
        const tabBLine = lines.find(l => l.includes('"Tab B"'));
        const tabCLine = lines.find(l => l.includes('"Tab C"'));
        expect(tabALine).not.toContain('[active]');
        expect(tabBLine).toContain('[active]');
        expect(tabCLine).not.toContain('[active]');

        // All content visible (including background tabs)
        expect(result.text).toContain('"Content A"');
        expect(result.text).toContain('"Content B"');
        expect(result.text).toContain('"Content C"');
    });

    it('GL panel titles extracted from .lm_title elements', () => {
        const stack = el('div', { class: 'lm_stack' },
            el('div', { class: 'lm_header' },
                el('div', { class: 'lm_tabs' },
                    el('div', { class: 'lm_tab lm_active' },
                        el('span', { class: 'lm_title' }, 'My Custom Title'),
                    ),
                    el('div', { class: 'lm_tab' },
                        el('span', { class: 'lm_title' }, 'Another Panel'),
                    ),
                ),
            ),
            el('div', { class: 'lm_items' },
                el('div', {}),
                el('div', {}),
            ),
        );
        container.appendChild(stack);
        const result = buildSnapshot(container);
        expect(result.text).toContain('[panel] "My Custom Title" [active]');
        expect(result.text).toContain('[panel] "Another Panel"');
    });

    // --- Date/time field collapsing ---

    it('collapses date field spinbutton group into single [date_field] line', () => {
        const group = el('div', { role: 'group' },
            el('input', { role: 'spinbutton', 'aria-label': 'month, ' }),
            el('input', { role: 'spinbutton', 'aria-label': 'day, ' }),
            el('input', { role: 'spinbutton', 'aria-label': 'year, ' }),
            el('input', { role: 'spinbutton', 'aria-label': 'hour, ' }),
            el('input', { role: 'spinbutton', 'aria-label': 'minute, ' }),
            el('input', { role: 'spinbutton', 'aria-label': 'second, ' }),
            el('input', { role: 'spinbutton', 'aria-label': 'AM/PM, ' }),
        );
        container.appendChild(group);
        const result = buildSnapshot(container);
        expect(result.text).toContain('[date_field]');
        expect(result.text).not.toContain('[spinbutton]');
        expect(result.text).not.toContain('[group]');
        expect(result.refs.has('@e1')).toBe(true);
        expect(result.interactiveCount).toBe(1);
    });

    it('collapses time field spinbutton group into single [time_field] line', () => {
        const group = el('div', { role: 'group' },
            el('input', { role: 'spinbutton', 'aria-label': 'hour, ' }),
            el('input', { role: 'spinbutton', 'aria-label': 'minute, ' }),
            el('input', { role: 'spinbutton', 'aria-label': 'second, ' }),
            el('input', { role: 'spinbutton', 'aria-label': 'AM/PM, ' }),
        );
        container.appendChild(group);
        const result = buildSnapshot(container);
        expect(result.text).toContain('[time_field]');
        expect(result.text).not.toContain('[spinbutton]');
        expect(result.interactiveCount).toBe(1);
    });

    it('collapses date picker (spinbuttons + Calendar button) into [date_picker]', () => {
        const group = el('div', { role: 'group' },
            el('input', { role: 'spinbutton', 'aria-label': 'month, ' }),
            el('input', { role: 'spinbutton', 'aria-label': 'day, ' }),
            el('input', { role: 'spinbutton', 'aria-label': 'year, ' }),
            el('input', { role: 'spinbutton', 'aria-label': 'hour, ' }),
            el('input', { role: 'spinbutton', 'aria-label': 'minute, ' }),
            el('input', { role: 'spinbutton', 'aria-label': 'second, ' }),
            el('input', { role: 'spinbutton', 'aria-label': 'AM/PM, ' }),
            el('button', {}, 'Calendar'),
        );
        container.appendChild(group);
        const result = buildSnapshot(container);
        expect(result.text).toContain('[date_picker]');
        expect(result.text).toContain('[button] "Calendar"');
        expect(result.text).not.toContain('[spinbutton]');
        // Refs: one for the group, one for the Calendar button
        expect(result.refs.has('@e1')).toBe(true);
        expect(result.refs.has('@e2')).toBe(true);
        expect(result.interactiveCount).toBe(2);
    });

    it('collapses date range picker spinbuttons into [date_range_picker]', () => {
        const group = el('div', { role: 'group' },
            el('input', { role: 'spinbutton', 'aria-label': 'month, Start Date, ' }),
            el('input', { role: 'spinbutton', 'aria-label': 'day, Start Date, ' }),
            el('input', { role: 'spinbutton', 'aria-label': 'year, Start Date, ' }),
            el('input', { role: 'spinbutton', 'aria-label': 'hour, Start Date, ' }),
            el('input', { role: 'spinbutton', 'aria-label': 'minute, Start Date, ' }),
            el('input', { role: 'spinbutton', 'aria-label': 'second, Start Date, ' }),
            el('input', { role: 'spinbutton', 'aria-label': 'AM/PM, Start Date, ' }),
            el('input', { role: 'spinbutton', 'aria-label': 'month, End Date, ' }),
            el('input', { role: 'spinbutton', 'aria-label': 'day, End Date, ' }),
            el('input', { role: 'spinbutton', 'aria-label': 'year, End Date, ' }),
            el('input', { role: 'spinbutton', 'aria-label': 'hour, End Date, ' }),
            el('input', { role: 'spinbutton', 'aria-label': 'minute, End Date, ' }),
            el('input', { role: 'spinbutton', 'aria-label': 'second, End Date, ' }),
            el('input', { role: 'spinbutton', 'aria-label': 'AM/PM, End Date, ' }),
            el('button', {}, 'Calendar'),
        );
        container.appendChild(group);
        const result = buildSnapshot(container);
        expect(result.text).toContain('[date_range_picker]');
        expect(result.text).toContain('[button] "Calendar"');
        expect(result.text).not.toContain('[spinbutton]');
        expect(result.interactiveCount).toBe(2);
    });

    it('includes label from aria-label on date field group', () => {
        const group = el('div', { role: 'group', 'aria-label': 'Birth Date' },
            el('input', { role: 'spinbutton', 'aria-label': 'month, ' }),
            el('input', { role: 'spinbutton', 'aria-label': 'day, ' }),
            el('input', { role: 'spinbutton', 'aria-label': 'year, ' }),
        );
        container.appendChild(group);
        const result = buildSnapshot(container);
        expect(result.text).toContain('[date_field] "Birth Date"');
    });

    it('does not collapse a group with non-date spinbuttons', () => {
        const group = el('div', { role: 'group' },
            el('input', { role: 'spinbutton', 'aria-label': 'quantity' }),
            el('input', { role: 'spinbutton', 'aria-label': 'price' }),
            el('input', { role: 'spinbutton', 'aria-label': 'discount' }),
        );
        container.appendChild(group);
        const result = buildSnapshot(container);
        expect(result.text).not.toContain('[date_field]');
        expect(result.text).toContain('[spinbutton]');
    });

    // --- Calendar collapsing ---

    it('collapses calendar grid into compact summary', () => {
        // Build a minimal calendar structure
        const grid = el('div', { role: 'grid', 'aria-label': 'Calendar, January 2024' });
        for (let day = 1; day <= 31; day++) {
            const dow = ['Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday', 'Sunday'][(day - 1) % 7];
            const label = day === 15 ? `${dow}, January ${day}, 2024 selected` : `${dow}, January ${day}, 2024`;
            grid.appendChild(
                el('div', { role: 'gridcell' },
                    el('button', { 'aria-label': label }, String(day)),
                ),
            );
        }
        const calendar = el('div', { role: 'application', 'aria-label': 'Calendar, January 2024' },
            el('h2', { role: 'heading' }, 'Calendar, January 2024'),
            el('button', { class: 'spectrum-ActionButton' }, 'Previous'),
            el('button', { class: 'spectrum-ActionButton' }, 'Next'),
            grid,
        );
        container.appendChild(calendar);
        const result = buildSnapshot(container);
        const text = result.text;

        // Should have collapsed calendar with summary
        expect(text).toContain('[calendar] "Calendar, January 2024"');
        expect(text).toContain('selected="Jan 15, 2024"');
        expect(text).toContain('[grid] 31 days');
        expect(text).toContain('Jan 1, 2024');
        expect(text).toContain('Jan 31, 2024');

        // Should NOT have individual day buttons
        expect(text).not.toContain('[gridcell]');
        expect(text).not.toContain('Monday, January');
        expect(text).not.toContain('Tuesday, January');

        // Should still have prev/next buttons
        expect(text).toContain('"Previous"');
        expect(text).toContain('"Next"');

        // Ref count: calendar + 2 action_buttons = 3 (not 31+ for days)
        expect(result.interactiveCount).toBe(3);
    });

    it('collapses range calendar and shows today marker', () => {
        const grid = el('div', { role: 'grid', 'aria-label': 'Range Calendar, March 2026' });
        for (let day = 1; day <= 31; day++) {
            const dow = ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday'][(day - 1) % 7];
            let label = `${dow}, March ${day}, 2026`;
            if (day === 18) label = `Today, ${label}`;
            grid.appendChild(
                el('div', { role: 'gridcell' },
                    el('button', { 'aria-label': label }, String(day)),
                ),
            );
        }
        const calendar = el('div', { role: 'application', 'aria-label': 'Range Calendar, March 2026' },
            el('h2', { role: 'heading' }, 'Range Calendar, March 2026'),
            el('button', { class: 'spectrum-ActionButton' }, 'Previous'),
            el('button', { class: 'spectrum-ActionButton' }, 'Next'),
            grid,
        );
        container.appendChild(calendar);
        const result = buildSnapshot(container);
        const text = result.text;

        expect(text).toContain('[range_calendar] "Range Calendar, March 2026"');
        expect(text).toContain('today="Mar 18, 2026"');
        expect(text).toContain('[grid] 31 days');
        expect(text).not.toContain('[gridcell]');
    });

    // --- Breadcrumbs: show all items, even aria-hidden truncated ones ---

    it('breadcrumbs shows all links including aria-hidden truncated items', () => {
        const nav = el('nav', { 'aria-label': 'Breadcrumbs' },
            el('ul', {},
                el('li', {},
                    el('a', { 'aria-hidden': 'true', href: '#' }, 'Home'),
                ),
                el('li', {},
                    el('button', { 'aria-label': '…' }, '…'),
                ),
                el('li', {},
                    el('a', { 'aria-hidden': 'true', href: '#' }, 'Products'),
                ),
                el('li', {},
                    el('a', { href: '#' }, 'Detail'),
                ),
            ),
        );
        container.appendChild(nav);
        const result = buildSnapshot(container);
        const text = result.text;

        expect(text).toContain('[breadcrumbs] "Breadcrumbs"');
        // All three items should appear, even the aria-hidden ones
        expect(text).toContain('[link] "Home"');
        expect(text).toContain('[link] "Products"');
        expect(text).toContain('[link] "Detail"');
        // The "…" menu button should NOT appear
        expect(text).not.toContain('"…"');
        // Each link gets a ref
        expect(result.refs.has('@e1')).toBe(true); // breadcrumbs nav
        expect(result.refs.has('@e2')).toBe(true); // Home
        expect(result.refs.has('@e3')).toBe(true); // Products
        expect(result.refs.has('@e4')).toBe(true); // Detail
        expect(result.interactiveCount).toBe(4);
    });

    it('does not collapse application without a calendar grid', () => {
        const app = el('div', { role: 'application', 'aria-label': 'Some App' },
            el('button', {}, 'Action'),
        );
        container.appendChild(app);
        const result = buildSnapshot(container);
        expect(result.text).toContain('[application]');
        expect(result.text).not.toContain('[calendar]');
    });

    it('GL chrome (controls, close, dropdown) still skipped in multi-panel', () => {
        const root = el('div', { class: 'lm_root' },
            el('div', { class: 'lm_item lm_column' },
                el('div', { class: 'lm_item lm_stack' },
                    el('div', { class: 'lm_header' },
                        el('div', { class: 'lm_tabs' },
                            el('div', { class: 'lm_tab lm_active' },
                                el('span', { class: 'lm_title' }, 'Panel 1'),
                            ),
                        ),
                        el('div', { class: 'lm_controls' },
                            el('button', {}, 'Close'),
                        ),
                        el('div', { class: 'lm_tabdropdown' }),
                    ),
                    el('div', { class: 'lm_items' },
                        el('div', {}, el('button', {}, 'Visible')),
                    ),
                ),
                el('div', { class: 'lm_item lm_stack' },
                    el('div', { class: 'lm_header' },
                        el('div', { class: 'lm_tabs' },
                            el('div', { class: 'lm_tab lm_active' },
                                el('span', { class: 'lm_title' }, 'Panel 2'),
                            ),
                        ),
                    ),
                    el('div', { class: 'lm_items' },
                        el('div', {}, el('button', {}, 'Also Visible')),
                    ),
                ),
            ),
        );
        container.appendChild(root);
        const result = buildSnapshot(container);
        expect(result.text).not.toContain('"Close"');
        expect(result.text).toContain('"Visible"');
        expect(result.text).toContain('"Also Visible"');
    });
});
