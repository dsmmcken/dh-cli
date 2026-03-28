/**
 * Accessibility snapshot builder.
 *
 * Walks the rendered DOM tree and produces a structured text representation
 * with @eN refs for interactive elements that agents can target.
 */

const INTERACTIVE_ROLES = new Set([
    'button', 'textbox', 'checkbox', 'radio', 'switch', 'slider',
    'combobox', 'link', 'spinbutton', 'tab', 'menuitem', 'option',
]);

/**
 * Map native HTML elements to their implicit ARIA roles.
 * Real Spectrum components render native elements without explicit role attributes.
 */
const IMPLICIT_ROLE_MAP = {
    button: 'button',
    a: 'link',
    input: (el) => {
        const type = el.getAttribute('type') || 'text';
        switch (type) {
            case 'checkbox': return 'checkbox';
            case 'radio': return 'radio';
            case 'range': return 'slider';
            case 'number': return 'spinbutton';
            case 'hidden': return null; // hidden inputs are not interactive
            default: return 'textbox';
        }
    },
    textarea: 'textbox',
    select: 'combobox',
    form: 'form',
    h1: 'heading', h2: 'heading', h3: 'heading', h4: 'heading', h5: 'heading', h6: 'heading',
};

/** GoldenLayout chrome classes to always skip. */
const GL_SKIP_CLASSES = ['lm_header', 'lm_controls', 'lm_tabdropdown', 'lm_splitter', 'lm_close_tab', 'lm_tabs'];

/** Spinbutton label segments that indicate date/time input fields. */
const DATE_SEGMENTS = ['month', 'day', 'year'];
const TIME_SEGMENTS = ['hour', 'minute', 'second'];

/**
 * Detect if a role="group" element is a Spectrum date/time input field.
 * These render as a group of spinbuttons for each segment (month, day, year, etc.).
 * Returns { type, label, calendarButton } or null.
 */
function getDateTimeGroupInfo(groupEl) {
    const children = Array.from(groupEl.children);
    if (children.length < 3) return null;

    // Partition children into spinbuttons and non-spinbuttons
    const spinbuttons = [];
    let calendarButton = null;
    let hasNonSpinNonCal = false;

    for (const child of children) {
        const r = getEffectiveRole(child);
        if (r === 'spinbutton') {
            spinbuttons.push(child);
        } else if (r === 'button' && /calendar/i.test(
            child.getAttribute('aria-label') || child.textContent || ''
        )) {
            calendarButton = child;
        } else {
            hasNonSpinNonCal = true;
        }
    }

    if (hasNonSpinNonCal || spinbuttons.length < 3) return null;

    // Check spinbutton labels for date/time segment names
    const labels = spinbuttons.map(s => (s.getAttribute('aria-label') || '').toLowerCase());
    const hasDate = labels.some(l => DATE_SEGMENTS.some(seg => l.includes(seg)));
    const hasTime = labels.some(l => TIME_SEGMENTS.some(seg => l.includes(seg)));
    if (!hasDate && !hasTime) return null;

    // Determine component type
    const hasRange = labels.some(l => l.includes('start date') || l.includes('end date'));
    let type;
    if (hasRange) {
        type = 'date_range_picker';
    } else if (hasDate && hasTime) {
        type = calendarButton ? 'date_picker' : 'date_field';
    } else if (hasTime && !hasDate) {
        type = 'time_field';
    } else {
        type = calendarButton ? 'date_picker' : 'date_field';
    }

    // Try to find a label from the group itself or its parent wrapper
    const label = groupEl.getAttribute('aria-label')
        || (() => {
            const lblBy = groupEl.getAttribute('aria-labelledby');
            if (lblBy) {
                const lblEl = groupEl.ownerDocument?.getElementById(lblBy);
                if (lblEl) return lblEl.textContent?.trim();
            }
            const parent = groupEl.parentElement;
            if (parent) {
                const fieldLabel = parent.querySelector('[class*="spectrum-FieldLabel"], label');
                if (fieldLabel) return fieldLabel.textContent?.trim();
            }
            return '';
        })()
        || '';

    return { type, label, calendarButton };
}

/**
 * Detect if a role="application" element is a Spectrum Calendar or RangeCalendar.
 * Returns { type, heading, gridEl, dayCount, firstDate, lastDate, selected, today } or null.
 */
function getCalendarInfo(appEl) {
    const grid = appEl.querySelector('[role="grid"]');
    if (!grid) return null;

    // Find all day buttons inside gridcells
    const dayButtons = grid.querySelectorAll('[role="gridcell"] button, [role="gridcell"] [role="button"]');
    if (dayButtons.length < 20) return null;

    // Verify buttons have date-like labels (weekday name prefix)
    const firstLabel = dayButtons[0]?.getAttribute('aria-label') || dayButtons[0]?.textContent || '';
    const dayNames = ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday', 'Today'];
    if (!dayNames.some(d => firstLabel.includes(d))) return null;

    const heading = appEl.querySelector('[role="heading"]')?.textContent?.trim()
        || appEl.getAttribute('aria-label') || '';

    // Determine if range calendar from heading
    const isRange = /range/i.test(heading);

    // Extract selected dates and today
    const selected = [];
    let today = null;
    for (const btn of dayButtons) {
        const lbl = btn.getAttribute('aria-label') || '';
        if (lbl.includes('selected')) selected.push(shortenDateLabel(lbl.replace(/ selected$/, '')));
        if (lbl.startsWith('Today')) today = shortenDateLabel(lbl.replace(/ selected$/, ''));
    }

    // Extract date range from first and last day buttons
    const allLabels = Array.from(dayButtons).map(b => b.getAttribute('aria-label') || '');
    const firstDate = shortenDateLabel(allLabels[0].replace(/^Today, /, '').replace(/ selected$/, ''));
    const lastDate = shortenDateLabel(allLabels[allLabels.length - 1].replace(/^Today, /, '').replace(/ selected$/, ''));

    return {
        type: isRange ? 'range_calendar' : 'calendar',
        heading,
        gridEl: grid,
        dayCount: dayButtons.length,
        firstDate,
        lastDate,
        selected,
        today,
    };
}

/**
 * Shorten a full date label for compact display.
 * "Sunday, December 31, 2023" → "Dec 31, 2023"
 */
function shortenDateLabel(label) {
    // Strip "Today, " prefix and day-of-week
    let s = label.replace(/^Today,\s*/, '');
    s = s.replace(/^(Sunday|Monday|Tuesday|Wednesday|Thursday|Friday|Saturday),\s*/, '');
    // Shorten month name
    s = s.replace(/^(January|February|March|April|May|June|July|August|September|October|November|December)/, m => m.substring(0, 3));
    return s;
}

/**
 * Infer a DH component name from CSS classes when data-component is absent.
 * Real Spectrum components rendered via DH's WidgetHandler get wrapper classes
 * like "dh-flex", "dh-grid", etc.
 */
const DH_CLASS_MAP = {
    'dh-flex': 'flex',
    'dh-grid': 'grid',
};

/**
 * Detect DH Picker or ComboBox wrapper from CSS class.
 * Returns 'picker', 'combobox', or null.
 */
function getPickerType(el) {
    const cls = el.className;
    if (!cls || typeof cls !== 'string') return null;
    if (cls.includes('dh-combobox')) return 'combobox';
    if (cls.includes('dh-picker')) return 'picker';
    return null;
}

/**
 * Detect DH component type from Spectrum DOM markers.
 * Returns the component name override, or null if no match.
 *
 * Each check uses a stable Spectrum attribute — not hashed class names.
 */
function getComponentOverride(el, role, cls) {
    const tag = el.tagName?.toLowerCase();

    // --- Buttons with specific subtypes ---
    if (role === 'button' || tag === 'button') {
        if (el.querySelector('[aria-roledescription="color swatch"]')) return 'color_picker';
        if (el.hasAttribute('aria-pressed')) return 'toggle_button';
        if (cls.includes('spectrum-LogicButton')) return 'logic_button';
        if (cls.includes('ContextualHelp-button')) return 'contextual_help';
        if (cls.includes('spectrum-Accordion-itemHeader')) return 'disclosure';
        // action_menu trigger: has aria-haspopup and no visible text (icon-only button)
        // Exclude breadcrumb buttons (inside spectrum-Breadcrumbs)
        if (el.getAttribute('aria-haspopup') === 'true' && el.getAttribute('aria-label')
            && !el.querySelector('[class*="spectrum-Button-label"]')
            && !el.closest('[class*="spectrum-Breadcrumbs"]')) return 'action_menu';
        if (cls.includes('spectrum-ActionButton')) return 'action_button';
    }

    // --- Input fields with specific subtypes ---
    if (tag === 'input') {
        const type = el.getAttribute('type') || 'text';
        if (type === 'search') return 'search_field';
        if (el.getAttribute('aria-roledescription') === 'Number field') return 'number_field';
    }

    // --- Wrapper elements with Spectrum patterns ---
    if (cls.includes('spectrum-Search') && el.querySelector('input[type="search"]')) return 'search_field';
    if (cls.includes('spectrum-Stepper-container')) return 'number_field';

    // --- Text field wrappers ---
    // text_area: spectrum-Textfield wrapper containing a <textarea>
    if (cls.includes('spectrum-Textfield-wrapper') && el.querySelector('textarea')) return 'text_area';
    // text_field: spectrum-Textfield wrapper containing <input type="text"> (but not search/number)
    if (cls.includes('spectrum-Textfield-wrapper') && el.querySelector('input[type="text"]')) return 'text_field';

    // --- Group-type components ---
    // button_group: exact class match (not spectrum-ButtonGroup-Button on child buttons)
    if (cls.includes('spectrum-ButtonGroup') && !cls.includes('spectrum-ButtonGroup-Button')
        && !cls.includes('spectrum-ActionGroup')) return 'button_group';
    // action_group: dh-action-group with role="toolbar" (skip outer flex-container wrapper)
    if (cls.includes('dh-action-group') && role === 'toolbar') return 'action_group';
    // checkbox_group: spectrum-FieldGroup (but not the inner -group child) containing checkboxes
    if (cls.includes('spectrum-FieldGroup') && !cls.includes('spectrum-FieldGroup-group')
        && el.querySelector('input[type="checkbox"]')) return 'checkbox_group';
    // radio_group: spectrum-FieldGroup (not inner) containing radiogroup, OR role="radiogroup"
    if (cls.includes('spectrum-FieldGroup') && !cls.includes('spectrum-FieldGroup-group')
        && el.querySelector('[role="radiogroup"]')) return 'radio_group';
    if (role === 'radiogroup') return 'radio_group';

    // --- Slider wrappers ---
    // spectrum-Slider with role="group" — distinguish slider vs range_slider by handle count
    if (cls.includes('spectrum-Slider') && role === 'group') {
        const handles = el.querySelectorAll('[class*="spectrum-Slider-handle"]');
        return handles.length > 1 ? 'range_slider' : 'slider';
    }

    // --- Progress / Meter ---
    if (cls.includes('spectrum-CircleLoader')) return 'progress_circle';
    // meter: spectrum-BarLoader with is-positive/is-warning/etc (Spectrum doesn't set role="meter")
    if (cls.includes('spectrum-BarLoader') && (cls.includes('is-positive') || cls.includes('is-warning')
        || cls.includes('is-critical') || cls.includes('is-notice')
        || el.querySelector('[class*="spectrum-BarLoader-label"]'))) {
        // Distinguish meter from progress_bar: meter has a variant class
        if (cls.includes('is-positive') || cls.includes('is-warning')
            || cls.includes('is-critical') || cls.includes('is-notice')) return 'meter';
    }

    // --- Display elements ---
    if (cls.includes('spectrum-Badge')) return 'badge';
    if (tag === 'hr' && cls.includes('spectrum-Rule')) return 'divider';
    if (tag === 'img' && cls.includes('spectrum-Avatar')) return 'avatar';
    if (tag === 'img' && cls.includes('spectrum-Image')) return 'image';
    if (tag === 'svg' && el.getAttribute('data-icon')) return 'icon';
    // labeled_value: spectrum-LabeledValue
    if (cls.includes('spectrum-LabeledValue')) return 'labeled_value';
    // inline_alert: spectrum-InLineAlert
    if (cls.includes('spectrum-InLineAlert')) return 'inline_alert';
    // illustrated_message: spectrum-IllustratedMessage
    if (cls.includes('spectrum-IllustratedMessage')) return 'illustrated_message';

    // --- DH wrapper classes ---
    if (cls.includes('dh-list-view') && !cls.includes('dh-list-view-wrapper')) return 'list_view';

    // --- Native elements ---
    if (tag === 'form') return 'form';
    if (tag === 'nav' && (el.getAttribute('aria-label') === 'Breadcrumbs' || cls.includes('spectrum-Breadcrumb'))) return 'breadcrumbs';

    // --- Markdown ---
    if (cls.includes('ui-markdown')) return 'markdown';

    return null;
}

function inferComponentFromClass(el) {
    const cls = el.className;
    if (!cls || typeof cls !== 'string') return null;
    for (const [prefix, name] of Object.entries(DH_CLASS_MAP)) {
        if (cls.includes(prefix)) return name;
    }
    return null;
}

/**
 * Check if a GoldenLayout root contains a single panel (one stack, one tab).
 */
function isSinglePanelLayout(rootEl) {
    const stacks = rootEl.querySelectorAll('.lm_stack');
    if (stacks.length !== 1) return false;
    const tabs = stacks[0].querySelectorAll('.lm_tab');
    return tabs.length <= 1;
}

function getEffectiveRole(el) {
    const explicit = el.getAttribute('role');
    if (explicit) return explicit;
    const tag = el.tagName?.toLowerCase();
    const mapped = IMPLICIT_ROLE_MAP[tag];
    if (typeof mapped === 'function') return mapped(el);
    return mapped || null;
}

/**
 * Build an accessibility snapshot of a rendered DOM container.
 *
 * @param {Element} container - The DOM container
 * @returns {{ text: string, refs: Map<string, Element> }}
 */
export function buildSnapshot(container) {
    const refs = new Map();
    let refCounter = 0;
    const lines = [];

    function getDirectText(el) {
        let text = '';
        for (const child of el.childNodes) {
            if (child.nodeType === 3) { // TEXT_NODE
                text += child.textContent;
            }
        }
        return text.trim();
    }

    /** Roles where textContent would pull in child element text (buttons, etc.) */
    const CONTAINER_ROLES = new Set([
        'table', 'figure', 'tablist', 'tabpanel', 'group', 'region',
        'list', 'menu', 'dialog', 'form',
    ]);

    function getAccessibleName(el, role, isComponentWithoutRole = false) {
        return el.getAttribute('aria-label')
            || el.getAttribute('data-title')
            || el.getAttribute('title')
            || getDirectText(el)
            || (CONTAINER_ROLES.has(role) || isComponentWithoutRole ? '' : el.textContent?.trim())
            || '';
    }

    /**
     * Walk a GoldenLayout stack: extract tab titles and active state,
     * then emit [panel] markers with content for each tab.
     */
    function walkStack(stackEl, depth, insideInteractive) {
        const tabs = [];
        const header = Array.from(stackEl.children).find(c =>
            (c.className || '').includes('lm_header')
        );
        if (header) {
            for (const tabEl of header.querySelectorAll('.lm_tab')) {
                const titleEl = tabEl.querySelector('.lm_title');
                const title = titleEl ? titleEl.textContent.trim() : '';
                const active = (tabEl.className || '').includes('lm_active');
                tabs.push({ title, active });
            }
        }

        const itemsContainer = Array.from(stackEl.children).find(c =>
            (c.className || '').includes('lm_items')
        );
        const contentEls = itemsContainer ? Array.from(itemsContainer.children) : [];

        const singleTab = tabs.length <= 1;
        if (!singleTab) {
            lines.push(`${'  '.repeat(depth)}[stack]`);
        }
        const panelDepth = singleTab ? depth : depth + 1;
        const count = Math.max(tabs.length, contentEls.length);
        for (let i = 0; i < count; i++) {
            const tab = tabs[i] || { title: '', active: false };
            const content = contentEls[i];

            let panelLabel = `[panel] "${tab.title}"`;
            if (tab.active && !singleTab) panelLabel += ' [active]';
            lines.push(`${'  '.repeat(panelDepth)}${panelLabel}`);

            if (content) {
                for (const child of content.children) {
                    walk(child, panelDepth + 1, insideInteractive);
                }
            }
        }
    }

    function walk(element, depth, insideInteractive = false) {
        if (!element || element.nodeType !== 1) return; // ELEMENT_NODE only

        // Skip aria-hidden elements (e.g. Spectrum's hidden tab picker <select>)
        // Exception: SVG icons with data-icon are DH icons we want to show.
        if (element.getAttribute('aria-hidden') === 'true') {
            if (!(element.tagName?.toLowerCase() === 'svg' && element.getAttribute('data-icon'))) {
                return;
            }
        }

        // --- GoldenLayout structure handling ---
        const cls = element.className || '';
        if (typeof cls === 'string') {
            // Skip GL chrome (headers, controls, splitters, etc.)
            if (GL_SKIP_CLASSES.some(c => cls.includes(c))) return;

            // GL root: single-panel passthrough vs multi-panel dashboard
            if (cls.includes('lm_root')) {
                if (isSinglePanelLayout(element)) {
                    const items = element.querySelector('.lm_items');
                    if (items) {
                        for (const item of items.children) {
                            for (const child of item.children) {
                                walk(child, depth, insideInteractive);
                            }
                        }
                    }
                    return;
                }
                lines.push(`${'  '.repeat(depth)}[dashboard]`);
                for (const child of element.children) {
                    walk(child, depth + 1, insideInteractive);
                }
                return;
            }

            // GL column / row layout containers
            if (cls.includes('lm_column')) {
                lines.push(`${'  '.repeat(depth)}[column]`);
                for (const child of element.children) {
                    walk(child, depth + 1, insideInteractive);
                }
                return;
            }
            if (cls.includes('lm_row')) {
                lines.push(`${'  '.repeat(depth)}[row]`);
                for (const child of element.children) {
                    walk(child, depth + 1, insideInteractive);
                }
                return;
            }

            // GL stack: emit [stack] (if >1 tab) + [panel] per tab
            if (cls.includes('lm_stack')) {
                walkStack(element, depth, insideInteractive);
                return;
            }
        }

        // --- DH Picker / ComboBox detection ---
        // Spectrum Picker wraps with "dh-picker", ComboBox with "dh-combobox".
        // Emit a clean [picker] or [combobox] node with label, selected value,
        // and option count instead of exposing internal trigger elements.
        const pickerType = getPickerType(element);
        if (pickerType) {
            const pickerIndent = '  '.repeat(depth);
            const labelEl = element.querySelector('[class*="spectrum-FieldLabel"], label');
            const label = labelEl?.textContent?.trim() || '';

            // Picker uses a trigger <button> with aria-haspopup="listbox"
            // ComboBox uses a wrapper <div> with role="button" and aria-haspopup="dialog"
            const triggerBtn = element.querySelector('button[aria-haspopup="listbox"]')
                || element.querySelector('[role="button"][aria-haspopup]');

            // Picker has a hidden <select> with all options; ComboBox does not
            const hiddenSelect = element.querySelector('select[tabindex="-1"]');
            const optionCount = hiddenSelect
                ? Array.from(hiddenSelect.options).filter(o => o.value).length
                : -1; // -1 = unknown

            // Selected value: check trigger button text (skip placeholder "Select…")
            let selected = '';
            if (pickerType === 'picker') {
                if (triggerBtn) {
                    const isPlaceholder = triggerBtn.querySelector('[class*="is-placeholder"]');
                    if (!isPlaceholder) {
                        const triggerText = triggerBtn.textContent?.trim();
                        if (triggerText) selected = triggerText;
                    }
                }
            } else {
                // ComboBox: selected value is in the mobile-value span
                const valueSpan = element.querySelector('[class*="mobile-value"]');
                if (valueSpan?.textContent?.trim()) {
                    selected = valueSpan.textContent.trim();
                }
            }

            refCounter++;
            const refId = `@e${refCounter}`;
            refs.set(refId, triggerBtn || element);

            let line = `${pickerIndent}${refId} [${pickerType}] "${label}"`;
            if (selected) line += ` selected="${selected}"`;
            if (optionCount >= 0) line += ` (${optionCount} options)`;
            lines.push(line);
            return; // Don't walk into picker/combobox internals
        }

        const role = getEffectiveRole(element);
        const elCls = typeof cls === 'string' ? cls : '';
        const componentOverride = getComponentOverride(element, role, elCls);
        const component = componentOverride
            || element.getAttribute('data-component')
            || inferComponentFromClass(element);
        const tag = element.tagName?.toLowerCase();

        // --- Component override handling ---
        // Emit a clean node using the DH component name instead of ARIA roles.
        if (componentOverride) {
            const oi = '  '.repeat(depth);

            // -- Leaf display elements (no ref, no children) --
            if (componentOverride === 'divider') {
                lines.push(`${oi}-----`);
                return;
            }
            if (componentOverride === 'icon') {
                const iconName = element.getAttribute('data-icon') || '';
                lines.push(`${oi}[icon] "${iconName}"`);
                return;
            }
            if (componentOverride === 'badge') {
                const badgeText = element.textContent?.trim() || '';
                lines.push(`${oi}[badge] "${badgeText}"`);
                return;
            }
            if (componentOverride === 'labeled_value') {
                const lvLabel = element.querySelector('[class*="spectrum-FieldLabel"]')?.textContent?.trim() || '';
                const lvValue = element.querySelector('[class*="spectrum-Field-field"]')?.textContent?.trim() || '';
                lines.push(`${oi}[labeled_value] "${lvLabel}" value="${lvValue}"`);
                return;
            }
            if (componentOverride === 'inline_alert') {
                const iaHeading = element.querySelector('[class*="spectrum-InLineAlert-heading"]')?.textContent?.trim() || '';
                const iaContent = element.querySelector('[class*="spectrum-InLineAlert-content"]')?.textContent?.trim() || '';
                let line = `${oi}[inline_alert] "${iaHeading}"`;
                if (iaContent) line += ` "${iaContent}"`;
                lines.push(line);
                return;
            }
            if (componentOverride === 'illustrated_message') {
                const imHeading = element.querySelector('h3')?.textContent?.trim() || '';
                const imContent = element.querySelector('section, [class*="description"]')?.textContent?.trim() || '';
                const imIcon = element.querySelector('svg[data-icon]')?.getAttribute('data-icon') || '';
                let line = `${oi}[illustrated_message]`;
                if (imIcon) line += ` icon="${imIcon}"`;
                if (imHeading) line += ` "${imHeading}"`;
                if (imContent) line += ` "${imContent}"`;
                lines.push(line);
                return;
            }
            if (componentOverride === 'avatar') {
                const alt = element.getAttribute('alt') || '';
                lines.push(`${oi}[avatar] "${alt}"`);
                return;
            }
            if (componentOverride === 'image') {
                const alt = element.getAttribute('alt') || '';
                lines.push(`${oi}[image] "${alt}"`);
                return;
            }
            if (componentOverride === 'markdown') {
                // Walk markdown children (p, strong, em, etc.) as text
                const mdText = element.textContent?.trim() || '';
                lines.push(`${oi}[markdown] "${mdText.substring(0, 120)}"`);
                return;
            }
            if (componentOverride === 'meter') {
                const mLabel = element.querySelector('[class*="spectrum-BarLoader-label"]')?.textContent?.trim() || '';
                const mVal = element.querySelector('[class*="spectrum-BarLoader-percentage"]')?.textContent?.trim() || '';
                lines.push(`${oi}[meter] "${mLabel}" ${mVal}`);
                return;
            }
            if (componentOverride === 'progress_circle') {
                const pcLabel = element.getAttribute('aria-label') || '';
                const pcVal = element.getAttribute('aria-valuenow') || '';
                lines.push(`${oi}[progress_circle] "${pcLabel}" ${pcVal}%`);
                return;
            }

            // -- Interactive leaf components (get @ref, collapse internals) --
            const FIELD_COMPONENTS = new Set([
                'search_field', 'number_field', 'text_field', 'text_area',
                'slider', 'range_slider', 'list_view',
            ]);
            if (FIELD_COMPONENTS.has(componentOverride)) {
                const labelEl = (componentOverride === 'slider' || componentOverride === 'range_slider')
                    ? element.querySelector('label[class*="spectrum-Slider-label"]')
                    : element.querySelector('[class*="spectrum-FieldLabel"], label');
                const fLabel = element.getAttribute('aria-label')
                    || labelEl?.textContent?.trim() || '';

                let valueSuffix = '';
                if (componentOverride === 'search_field' || componentOverride === 'number_field'
                    || componentOverride === 'text_field' || componentOverride === 'text_area') {
                    const input = element.querySelector('input, textarea');
                    if (input) valueSuffix = ` value="${input.value || ''}"`;
                }
                if (componentOverride === 'slider') {
                    const out = element.querySelector('[class*="spectrum-Slider-value"]');
                    if (out) valueSuffix = ` value="${out.textContent?.trim()}"`;
                }
                if (componentOverride === 'range_slider') {
                    const out = element.querySelector('[class*="spectrum-Slider-value"]');
                    if (out) valueSuffix = ` value="${out.textContent?.trim()}"`;
                }

                refCounter++;
                const refId = `@e${refCounter}`;
                refs.set(refId, element.querySelector('input, textarea') || element);

                let line = `${oi}${refId} [${componentOverride}] "${fLabel}"`;
                line += valueSuffix;
                lines.push(line);
                return;
            }

            // -- Breadcrumbs: extract ALL items including aria-hidden ones --
            // Spectrum truncates breadcrumb items in narrow viewports (like jsdom)
            // by hiding them with aria-hidden="true" and showing a "…" menu.
            // We want all items visible, so query for all links directly.
            if (componentOverride === 'breadcrumbs') {
                const cLabel = element.getAttribute('aria-label') || '';
                refCounter++;
                const refId = `@e${refCounter}`;
                refs.set(refId, element);

                let line = `${oi}${refId} [breadcrumbs]`;
                if (cLabel) line += ` "${cLabel}"`;
                lines.push(line);

                // Find all breadcrumb links (a elements), including aria-hidden ones
                const links = element.querySelectorAll('a');
                for (const link of links) {
                    const text = link.textContent?.trim();
                    if (!text) continue;
                    refCounter++;
                    const linkRefId = `@e${refCounter}`;
                    refs.set(linkRefId, link);
                    lines.push(`${oi}  ${linkRefId} [link] "${text}"`);
                }
                return;
            }

            // -- Container components (get @ref, walk children) --
            const CONTAINER_OVERRIDES = new Set([
                'form', 'button_group', 'action_group',
                'checkbox_group', 'radio_group', 'action_menu',
            ]);
            if (CONTAINER_OVERRIDES.has(componentOverride)) {
                // For field-groups (checkbox_group, radio_group), get label from FieldLabel span
                const isFieldGroup = componentOverride === 'checkbox_group' || componentOverride === 'radio_group';
                const fieldLabelEl = isFieldGroup
                    ? element.querySelector('[class*="spectrum-FieldLabel"]') : null;
                const cLabel = element.getAttribute('aria-label')
                    || fieldLabelEl?.textContent?.trim() || '';

                refCounter++;
                const refId = `@e${refCounter}`;
                refs.set(refId, element);

                let line = `${oi}${refId} [${componentOverride}]`;
                if (cLabel) line += ` "${cLabel}"`;
                lines.push(line);

                // For field-groups, skip the label span and walk the inner group directly
                if (isFieldGroup) {
                    const innerGroup = element.querySelector('[role="group"], [role="radiogroup"]');
                    if (innerGroup) {
                        for (const child of innerGroup.children) {
                            walk(child, depth + 1, insideInteractive);
                        }
                    }
                } else {
                    for (const child of element.children) {
                        walk(child, depth + 1, insideInteractive);
                    }
                }
                return;
            }
        }

        // --- Date/time field detection ---
        // Spectrum DateField, DatePicker, TimeField, DateRangePicker render as a
        // role="group" containing individual spinbuttons for each date/time segment.
        // Collapse these into a single compact line.
        if (role === 'group') {
            const dateInfo = getDateTimeGroupInfo(element);
            if (dateInfo) {
                const oi = '  '.repeat(depth);
                refCounter++;
                const refId = `@e${refCounter}`;
                refs.set(refId, element);

                let line = `${oi}${refId} [${dateInfo.type}]`;
                if (dateInfo.label) line += ` "${dateInfo.label}"`;
                lines.push(line);

                // If there's a Calendar button, emit it as a child with its own ref
                if (dateInfo.calendarButton) {
                    refCounter++;
                    const calRefId = `@e${refCounter}`;
                    refs.set(calRefId, dateInfo.calendarButton);
                    lines.push(`${oi}  ${calRefId} [button] "Calendar"`);
                }
                return;
            }
        }

        // --- Calendar / RangeCalendar detection ---
        // Spectrum Calendar renders as role="application" containing a heading,
        // prev/next navigation, and a grid of day buttons. Collapse the grid
        // into a compact summary while keeping navigation buttons walkable.
        if (role === 'application') {
            const calInfo = getCalendarInfo(element);
            if (calInfo) {
                const oi = '  '.repeat(depth);
                refCounter++;
                const refId = `@e${refCounter}`;
                refs.set(refId, element);

                let line = `${oi}${refId} [${calInfo.type}] "${calInfo.heading}"`;
                if (calInfo.selected.length > 0) {
                    line += ` selected="${calInfo.selected.join(', ')}"`;
                }
                if (calInfo.today) {
                    line += ` today="${calInfo.today}"`;
                }
                lines.push(line);

                // Walk non-grid, non-heading children normally (prev/next buttons, etc.)
                for (const child of element.children) {
                    if (child === calInfo.gridEl) {
                        // Emit compact grid summary instead of enumerating every day
                        let gridLine = `${oi}  [grid] ${calInfo.dayCount} days`;
                        if (calInfo.firstDate && calInfo.lastDate) {
                            gridLine += ` (${calInfo.firstDate} – ${calInfo.lastDate})`;
                        }
                        lines.push(gridLine);
                    } else if (child.getAttribute('role') === 'heading') {
                        // Skip heading — redundant with the calendar label
                    } else {
                        walk(child, depth + 1, insideInteractive);
                    }
                }
                return;
            }
        }

        // role="none"/"presentation" — decorative wrappers.
        // Show direct text (unless inside an interactive element where the parent
        // already shows the name), then always walk child elements.
        if (role === 'none' || role === 'presentation') {
            const directText = getDirectText(element);
            if (directText && !insideInteractive) {
                const indent = '  '.repeat(depth);
                lines.push(`${indent}"${directText.substring(0, 120)}"`);
            }
            for (const child of element.children) {
                walk(child, depth, insideInteractive);
            }
            return;
        }

        // Skip invisible/structural elements with no useful info
        if (!role && !component && tag === 'div' && !getDirectText(element)) {
            // Still walk children
            for (const child of element.children) {
                walk(child, depth, insideInteractive);
            }
            return;
        }

        const indent = '  '.repeat(depth);
        let ref = '';
        let label = '';

        if ((role && INTERACTIVE_ROLES.has(role)) || componentOverride) {
            refCounter++;
            const refId = `@e${refCounter}`;
            refs.set(refId, element);
            ref = `${refId} `;
        }

        // Display: prefer data-component short name, fall back to ARIA role
        const shortName = component ? component.split('.').pop() : null;

        if (shortName || role) {
            const displayTag = shortName || role;
            const isComponentWithoutRole = !!shortName && !role;
            const name = getAccessibleName(element, role, isComponentWithoutRole);
            label = `[${displayTag}]`;
            if (name && name !== displayTag) label += ` "${name}"`;

            // color_picker: append the color value from the swatch's aria-label
            if (componentOverride === 'color_picker') {
                const swatch = element.querySelector('[aria-roledescription="color swatch"]');
                if (swatch) {
                    const colorDesc = swatch.getAttribute('aria-label') || '';
                    if (colorDesc) label += ` (${colorDesc})`;
                }
            }

            // Role-specific value/state suffixes
            if (role === 'textbox' || role === 'spinbutton') {
                const input = (tag === 'input' || tag === 'textarea') ? element : element.querySelector('input,textarea');
                if (input) {
                    const val = input.value || input.getAttribute('value') || '';
                    label += ` value="${val}"`;
                }
            }
            if (role === 'combobox') {
                const selectedKey = element.getAttribute('data-selectedkey') || element.getAttribute('data-selected_key');
                if (selectedKey) label += ` selected="${selectedKey}"`;
            }
            if (role === 'checkbox' || role === 'switch') {
                const input = (tag === 'input') ? element : element.querySelector('input[type="checkbox"]');
                if (input) label += input.checked ? ' [checked]' : ' [unchecked]';
            }
            if (role === 'table') {
                const tableType = element.getAttribute('data-table-type');
                const rowCount = element.getAttribute('data-row-count');
                const colCount = element.getAttribute('data-column-count');
                if (tableType) label += ` (${tableType})`;
                if (rowCount && colCount) label += ` ${rowCount} rows, ${colCount} columns`;
            }
            if (role === 'figure') {
                const figApi = element.getAttribute('data-figure-api');
                if (figApi === 'classic') label = '[Plot]';
                const figType = element.getAttribute('data-figure-type');
                const traceCount = element.getAttribute('data-trace-count');
                if (figType) label += ` ${figType}`;
                if (traceCount) label += ` | ${traceCount} traces`;
            }
        } else if (tag === 'span' || tag === 'p') {
            const text = getDirectText(element);
            if (text) label = `"${text.substring(0, 120)}"`;
        } else if (tag === 'label') {
            const text = getDirectText(element);
            if (text) label = `[label] "${text}"`;
        } else {
            const text = getDirectText(element);
            if (text) label = `"${text.substring(0, 120)}"`;
        }

        if (label) {
            lines.push(`${indent}${ref}${label}`);
        }

        // Component overrides on interactive elements (color_picker, toggle_button,
        // etc.) are self-contained — don't walk into their Spectrum internals.
        if (componentOverride && (role === 'button' || tag === 'button')) {
            return;
        }

        const isInteractive = role && INTERACTIVE_ROLES.has(role);
        for (const child of element.children) {
            walk(child, label ? depth + 1 : depth, insideInteractive || isInteractive);
        }
    }

    walk(container, 0);

    return {
        text: lines.join('\n'),
        refs,
        interactiveCount: refCounter,
    };
}
