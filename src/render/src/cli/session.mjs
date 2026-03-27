/**
 * DaemonSession - Holds all state for a dh-render session.
 *
 * Uses the real WidgetHandler rendering pipeline with full GoldenLayout
 * portal support and real Spectrum components. Interactions use DOM events
 * which WidgetHandler routes to server-side callables automatically.
 */
import React from 'react';
import { createTestClient } from '../index.mjs';
import { buildSnapshot } from './snapshot.mjs';

export class DaemonSession {
    constructor() {
        this.serverUrl = null;
        this._testClient = null;
        this._renderResult = null;
        this._renderResults = [];
        this._snapshotRefs = new Map();
        this._widgetName = null;
        this._widgetType = null;
    }

    /**
     * Connect to a Deephaven server. Loads JSAPI and establishes connection.
     */
    async open(serverUrl) {
        this.serverUrl = serverUrl;
        this._testClient = await createTestClient(serverUrl);
        return { ok: true, message: `Connected to ${serverUrl}` };
    }

    /**
     * Render a widget using WidgetHandler with real DH components.
     * If widgetName is empty/null, auto-discovers renderable widgets
     * (Dashboard > Element) and renders all of them.
     */
    async render(widgetName, widgetType, timeout = 15000) {
        if (!this._testClient) {
            return { ok: false, error: 'Not connected. Use "open" first.' };
        }

        // Clean up previous renders
        for (const r of this._renderResults) {
            r.unmount();
        }
        this._renderResults = [];
        this._renderResult = null;

        // If no widget name, discover renderable widgets from the server
        let widgets;
        if (!widgetName) {
            const wc = this._widgetClient;
            if (!wc) {
                return { ok: false, error: 'Not connected.' };
            }
            widgets = await wc.discoverWidgets(timeout);
            if (widgets.length === 0) {
                return {
                    ok: false,
                    error: 'No renderable widgets found. The script must define a '
                         + 'ui.dashboard() or @ui.component decorated function.\n'
                         + 'Use --widget to specify a variable name manually.',
                };
            }
        } else {
            widgets = [{ name: widgetName, type: widgetType }];
        }

        // Render each widget — all go into the same jsdom body
        const names = [];
        for (const w of widgets) {
            const result = await this._testClient.render(w.name, {
                widgetType: w.type, timeout,
            });
            this._renderResults.push(result);
            names.push(w.name);
        }

        // Use the first result as the primary (for _body access)
        this._renderResult = this._renderResults[0];
        this._widgetName = names.join(', ');
        this._widgetType = widgetType;

        // Build initial snapshot from body (portals render outside container)
        const body = this._body;
        const snapshot = buildSnapshot(body);
        this._snapshotRefs = snapshot.refs;

        // Count elements, buttons/interactive, and any exported objects
        const panels = body.querySelectorAll('.dh-react-panel');
        let elementCount = 0;
        for (const panel of panels) {
            elementCount += panel.querySelectorAll('*').length;
        }
        const callableCount = body.querySelectorAll('button, [role="button"], a[href], [role="link"]').length;

        const label = names.length === 1 ? `"${names[0]}"` : `${names.length} widgets (${names.join(', ')})`;
        return {
            ok: true,
            message: `Rendered ${label}`,
            snapshot: snapshot.text,
            interactiveCount: snapshot.interactiveCount,
            elementCount,
            callableCount,
            exportedObjectCount: 0,
        };
    }

    /**
     * Get the current accessibility snapshot.
     */
    snapshot() {
        if (!this._body) {
            return { ok: false, error: 'No widget rendered. Use "render" first.' };
        }
        const snapshot = buildSnapshot(this._body);
        this._snapshotRefs = snapshot.refs;
        return {
            ok: true,
            snapshot: snapshot.text,
            interactiveCount: snapshot.interactiveCount,
        };
    }

    /**
     * Click a button by text or @ref.
     *
     * With real components, clicking the DOM element fires the React synthetic
     * event which WidgetHandler routes to the server-side callable automatically.
     */
    async click(target) {
        if (!this._body) {
            return { ok: false, error: 'No widget rendered.' };
        }

        const element = this._resolveTarget(target, ['button', 'link', 'tab', 'menuitem']);
        if (!element) {
            return { ok: false, error: `Could not find clickable element: "${target}"` };
        }

        const buttonText = element.textContent?.trim();

        // Fire DOM click event within act() — WidgetHandler handles the callback
        const { act } = React;
        await act(async () => {
            element.click();
        });

        // Flush to process the server response and re-render
        await this._renderResult.flush(500);

        return { ok: true, message: `Clicked "${buttonText}"` };
    }

    /**
     * Fill a text field by label or @ref.
     *
     * Finds the input, sets its value, and dispatches input/change events.
     * WidgetHandler routes the onChange callback to the server automatically.
     */
    async fill(target, value) {
        if (!this._body) {
            return { ok: false, error: 'No widget rendered.' };
        }

        const element = this._resolveTarget(target, ['textbox', 'spinbutton']);
        if (!element) {
            return { ok: false, error: `Could not find input element: "${target}"` };
        }

        // Find the actual input element
        let input = element;
        if (element.tagName !== 'INPUT' && element.tagName !== 'TEXTAREA') {
            input = element.querySelector('input,textarea');
        }
        // Spectrum TextField may render the <input> as a sibling rather than
        // a child of the role="textbox" wrapper — search the parent field container.
        if (!input) {
            const field = element.closest('[class*="spectrum-Textfield"], [class*="spectrum-Field"], [class*="spectrum-TextField"]')
                || element.parentElement;
            input = field?.querySelector('input,textarea');
        }
        // Follow <label for="..."> → input
        if (!input && element.tagName === 'LABEL' && element.htmlFor) {
            const doc = this._body.ownerDocument || this._body;
            input = doc.getElementById?.(element.htmlFor);
        }
        if (!input) {
            return { ok: false, error: `No input found for "${target}"` };
        }

        const label = element.getAttribute('aria-label') || target;
        const { act } = React;

        // Set value and dispatch events — use the correct prototype for the element type
        const proto = input.tagName === 'TEXTAREA'
            ? globalThis.HTMLTextAreaElement?.prototype
            : globalThis.HTMLInputElement?.prototype;
        const nativeValueSetter = Object.getOwnPropertyDescriptor(
            proto || input.constructor.prototype, 'value'
        )?.set;
        await act(async () => {
            if (nativeValueSetter) {
                nativeValueSetter.call(input, value);
            } else {
                input.value = value;
            }
            input.dispatchEvent(new globalThis.Event('input', { bubbles: true }));
            input.dispatchEvent(new globalThis.Event('change', { bubbles: true }));
        });

        // Flush to process the server response
        await this._renderResult.flush(500);

        return { ok: true, message: `Filled "${label}" with "${value}"` };
    }

    /**
     * Select a value in a picker/combobox.
     *
     * Strategy:
     * 1. Try clicking the trigger to open the popup, then click the [role="option"].
     * 2. If the popup doesn't render (jsdom limitation with Spectrum overlays),
     *    fall back to the hidden <select> element that Spectrum renders for
     *    accessibility and set its value + fire change events.
     */
    async select(target, value) {
        if (!this._body) {
            return { ok: false, error: 'No widget rendered.' };
        }

        // Find the picker trigger button
        const element = this._resolveTarget(target, ['listbox', 'combobox', 'button']);
        if (!element) {
            return { ok: false, error: `Could not find picker: "${target}"` };
        }

        const { act } = React;

        // Click to open the picker
        await act(async () => {
            element.click();
        });
        await this._renderResult.flush(200);

        // Strategy 1: Find and click the option in a popup/listbox
        const options = this._body.querySelectorAll('[role="option"]');
        let found = false;
        for (const opt of options) {
            if (opt.textContent?.trim() === value) {
                await act(async () => {
                    opt.click();
                });
                found = true;
                break;
            }
        }

        if (found) {
            await this._renderResult.flush(500);
            return { ok: true, message: `Selected "${value}" in "${target}"` };
        }

        // Strategy 2: Use the hidden <select> element (Spectrum Picker fallback).
        // Spectrum renders a hidden <select> for accessibility that mirrors the
        // picker's items. Changing its value triggers React's onChange handler.
        const pickerField = element.closest('.dh-picker, .dh-picker-normalized, [class*="spectrum-Field"]');
        const hiddenSelect = pickerField?.querySelector('select[tabindex="-1"]')
            || element.parentElement?.querySelector('select[tabindex="-1"]');

        if (hiddenSelect) {
            const option = Array.from(hiddenSelect.options).find(
                o => o.value === value || o.textContent === value
            );
            if (option) {
                await act(async () => {
                    hiddenSelect.value = option.value;
                    hiddenSelect.dispatchEvent(new globalThis.Event('change', { bubbles: true }));
                });
                await this._renderResult.flush(500);
                return { ok: true, message: `Selected "${value}" in "${target}"` };
            }
        }

        return { ok: false, error: `Could not find option "${value}" in picker "${target}"` };
    }

    /**
     * List options available in a picker/combobox.
     *
     * Reads the hidden <select> element that Spectrum renders for accessibility.
     * This contains all loaded options, even for table-backed pickers.
     *
     * @param {string} target - Label, text, or @ref of the picker
     * @param {number} [offset=0] - Start index for pagination
     * @param {number} [limit=20] - Max options to return
     */
    async options(target, offset = 0, limit = 20) {
        if (!this._body) {
            return { ok: false, error: 'No widget rendered.' };
        }

        // Resolve the target element
        const element = this._resolveTarget(target, ['combobox', 'button']);
        if (!element) {
            return { ok: false, error: `Could not find picker: "${target}"` };
        }

        // Walk up to the .dh-picker or .dh-combobox wrapper
        const picker = element.closest('.dh-picker')
            || element.closest('.dh-combobox')
            || element.closest('[class*="spectrum-Field"]');
        if (!picker) {
            return { ok: false, error: `Element "${target}" is not a picker` };
        }

        // Find the hidden <select> that Spectrum renders for accessibility.
        // Pickers always have one; ComboBoxes do not (their options load on open).
        const hiddenSelect = picker.querySelector('select[tabindex="-1"]');
        if (!hiddenSelect) {
            const isCombo = picker.classList?.contains('dh-combobox');
            const hint = isCombo
                ? 'ComboBox options are not pre-loaded. Use "select" to type a value directly.'
                : `No options found for picker "${target}"`;
            return { ok: false, error: hint };
        }

        // Extract all options (skip the empty placeholder)
        const allOptions = Array.from(hiddenSelect.options)
            .filter(o => o.value)
            .map(o => o.textContent?.trim() || o.value);

        const total = allOptions.length;
        const page = allOptions.slice(offset, offset + limit);

        // Get the picker label
        const labelEl = picker.querySelector('[class*="spectrum-FieldLabel"], label');
        const label = labelEl?.textContent?.trim() || target;

        return { ok: true, label, options: page, total, offset, limit };
    }

    /**
     * List exported tables with metadata and sample data.
     * Note: Table access requires the WidgetClient's direct connection.
     */
    async tables(maxRows = 3) {
        const wc = this._widgetClient;
        if (!wc) {
            return { ok: false, error: 'Not connected.' };
        }

        const tableEntries = [...wc.exportedObjectMap.entries()]
            .filter(([_, obj]) => obj.type === 'Table');

        const tables = [];
        for (const [id, obj] of tableEntries) {
            const info = { id, columns: [], rowCount: null, sampleRows: [], error: null };
            try {
                const table = await wc.fetchTable(id);
                info.columns = table.columns.map(c => ({ name: c.name, type: c.type }));
                info.rowCount = table.size;

                if (maxRows > 0) {
                    const endRow = Math.min(maxRows - 1, table.size - 1);
                    if (endRow >= 0) {
                        info.sampleRows = await wc.getTableData(table, 0, endRow);
                    }
                }
            } catch (e) {
                info.error = typeof e === 'string' ? e : e.message;
            }
            tables.push(info);
        }

        return { ok: true, tables };
    }

    /**
     * Fetch a specific table by object ID.
     */
    async table(objectId, maxRows = 20) {
        const wc = this._widgetClient;
        if (!wc) {
            return { ok: false, error: 'Not connected.' };
        }

        try {
            const table = await wc.fetchTable(objectId);
            const columns = table.columns.map(c => ({ name: c.name, type: c.type }));
            const endRow = Math.min(maxRows - 1, table.size - 1);
            const rows = endRow >= 0 ? await wc.getTableData(table, 0, endRow) : [];
            return { ok: true, columns, rowCount: table.size, rows };
        } catch (e) {
            return { ok: false, error: typeof e === 'string' ? e : e.message };
        }
    }

    /**
     * Get the DH document tree (pretty-printed from DOM).
     */
    document() {
        if (!this._body) {
            return { ok: false, error: 'No widget rendered. Use "render" first.' };
        }
        // Build a simplified tree from the DOM
        const tree = this._buildDocumentTree(this._body.querySelector('.dh-react-panel') || this._body);
        return { ok: true, tree: JSON.stringify(tree, null, 2) };
    }

    /**
     * List server-side callables discovered from DOM event handlers.
     */
    callables() {
        if (!this._body) {
            return { ok: false, error: 'No widget rendered. Use "render" first.' };
        }
        // In the real WidgetHandler pipeline, callables are wired internally.
        // We can list interactive elements as callable targets.
        const callables = [];
        const buttons = this._body.querySelectorAll('button, [role="button"]');
        let idx = 0;
        for (const btn of buttons) {
            const text = btn.textContent?.trim();
            if (text) {
                callables.push({
                    id: `btn-${idx++}`,
                    path: text,
                    parentElement: btn.closest('[class*="panel"]')?.className?.split(' ')[0] || 'root',
                });
            }
        }
        return { ok: true, callables };
    }

    /**
     * Call a server-side callable by clicking the associated element.
     */
    async call(callableId, args = []) {
        // In WidgetHandler mode, "call" means click the element
        return { ok: false, error: 'Direct callable invocation not available in WidgetHandler mode. Use "click" instead.' };
    }

    /**
     * Get rendered HTML.
     */
    html() {
        if (!this._body) {
            return { ok: false, error: 'No widget rendered.' };
        }
        return { ok: true, html: this._body.innerHTML };
    }

    /**
     * Wait for next document update (flushes React effects).
     */
    async wait(timeout = 5000) {
        if (!this._renderResult) {
            return { ok: false, error: 'No widget rendered.' };
        }

        try {
            await this._renderResult.flush(timeout);
            return { ok: true, message: 'Effects flushed' };
        } catch (e) {
            return { ok: false, error: typeof e === 'string' ? e : e.message };
        }
    }

    /**
     * Get session status.
     */
    status() {
        return {
            ok: true,
            connected: !!this._testClient,
            serverUrl: this.serverUrl,
            widget: this._widgetName,
            widgetType: this._widgetType,
            hasRender: !!this._renderResult,
            exportedObjects: 0,
        };
    }

    /**
     * Clean up everything.
     */
    close() {
        for (const r of this._renderResults) {
            r.unmount();
        }
        this._renderResults = [];
        this._renderResult = null;
        if (this._testClient) {
            this._testClient.close();
            this._testClient = null;
        }
        return { ok: true, message: 'Session closed' };
    }

    // ── Private ──

    /** Convenience accessor for the widget client. */
    get _widgetClient() {
        return this._testClient?.widgetClient;
    }

    /** Convenience accessor for the DOM body (includes portaled content). */
    get _body() {
        return this._renderResult?.body;
    }

    /** Build a simplified document tree from a DOM element. */
    _buildDocumentTree(el) {
        if (!el || el.nodeType !== 1) return null;
        const tag = el.tagName?.toLowerCase();
        const role = el.getAttribute('role');
        const text = el.childNodes.length === 1 && el.childNodes[0].nodeType === 3
            ? el.childNodes[0].textContent.trim() : undefined;
        const node = { tag };
        if (role) node.role = role;
        if (el.getAttribute('aria-label')) node.label = el.getAttribute('aria-label');
        if (text) node.text = text;
        const children = [];
        for (const child of el.children) {
            const c = this._buildDocumentTree(child);
            if (c) children.push(c);
        }
        if (children.length > 0) node.children = children;
        return node;
    }

    /**
     * Get the accessible name of an element, checking aria-label, aria-labelledby,
     * associated <label> elements, and textContent.
     */
    _getAccessibleName(el, body) {
        // 1. Explicit aria-label
        const ariaLabel = el.getAttribute('aria-label');
        if (ariaLabel) return ariaLabel;

        // 2. aria-labelledby → resolve referenced element text
        const labelledBy = el.getAttribute('aria-labelledby');
        if (labelledBy) {
            const doc = body.ownerDocument || body;
            const name = labelledBy.split(/\s+/)
                .map(id => doc.getElementById?.(id)?.textContent?.trim())
                .filter(Boolean)
                .join(' ');
            if (name) return name;
        }

        // 3. Associated <label for="..."> (for input/textarea/select)
        if (el.id) {
            const label = body.querySelector(`label[for="${el.id}"]`);
            if (label) return label.textContent?.trim();
        }

        // 4. Fall back to textContent
        return el.textContent?.trim() || '';
    }

    /**
     * Resolve a target string to a DOM element.
     * @param {string} target - @ref or text
     * @param {string[]} roles - Roles to search
     */
    _resolveTarget(target, roles) {
        const body = this._body;
        if (!body) return null;

        // @ref resolution
        if (target.startsWith('@e')) {
            return this._snapshotRefs.get(target) || null;
        }

        // Map roles to CSS selectors (includes both explicit role and native elements)
        const ROLE_SELECTORS = {
            button: '[role="button"], button',
            link: '[role="link"], a[href]',
            textbox: '[role="textbox"], input:not([type]), input[type="text"], input[type="email"], input[type="url"], input[type="search"], input[type="tel"], textarea',
            checkbox: '[role="checkbox"], input[type="checkbox"]',
            radio: '[role="radio"], input[type="radio"]',
            slider: '[role="slider"], input[type="range"]',
            spinbutton: '[role="spinbutton"], input[type="number"]',
            combobox: '[role="combobox"], select',
            tab: '[role="tab"]',
            menuitem: '[role="menuitem"]',
            option: '[role="option"], option',
        };

        // Try by accessible name within each role.
        // First pass: exact match only. Second pass: substring (includes) match.
        // This prevents "A" from matching "Action" when a button literally named "A" exists.
        let substringMatch = null;
        for (const role of roles) {
            const selector = ROLE_SELECTORS[role] || `[role="${role}"]`;
            const candidates = body.querySelectorAll(selector);
            for (const el of candidates) {
                const name = this._getAccessibleName(el, body);
                if (!name) continue;
                if (name === target) {
                    return el; // Exact match — return immediately
                }
                if (!substringMatch && name.includes(target)) {
                    substringMatch = el; // Remember first substring match as fallback
                }
            }
        }
        if (substringMatch) return substringMatch;

        // Try by text content
        const allElements = body.querySelectorAll('*');
        for (const el of allElements) {
            if (el.textContent?.trim() === target && el.children.length === 0) {
                // Walk up to find clickable parent
                let node = el;
                while (node && node !== body) {
                    const role = node.getAttribute('role');
                    if (role && roles.includes(role)) return node;
                    node = node.parentElement;
                }
                return el;
            }
        }

        return null;
    }
}
