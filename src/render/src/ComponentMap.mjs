/**
 * ComponentMap - Maps Deephaven element names to React components.
 *
 * For server-side testing, we don't need the full Deephaven UI component library.
 * Instead, we create lightweight stub components that:
 * - Render the correct HTML structure
 * - Pass through props
 * - Support event handlers
 * - Display text content
 *
 * Users can override/extend the component map with custom components.
 */
import React from 'react';
import { UITableStub } from './UITableStub.mjs';

const { createElement: h, Fragment } = React;

/**
 * Create a simple stub component that renders as a div with data attributes.
 * @param {string} displayName - The component display name
 * @param {string} [tag='div'] - The HTML tag to use
 */
function createStubComponent(displayName, tag = 'div') {
    const Component = (props) => {
        const { children, __dhElementName, ...restProps } = props || {};

        // Extract event handlers and data props
        const htmlProps = {};
        const dataProps = {};

        for (const [key, value] of Object.entries(restProps)) {
            if (key.startsWith('on') && typeof value === 'function') {
                // Keep event handlers
                htmlProps[key] = value;
            } else if (typeof value === 'string' || typeof value === 'number' || typeof value === 'boolean') {
                dataProps[`data-${key.toLowerCase()}`] = String(value);
            }
        }

        return h(tag, {
            'data-component': __dhElementName || displayName,
            ...dataProps,
            ...htmlProps,
            role: getRole(displayName),
        }, children);
    };

    Component.displayName = displayName;
    return Component;
}

/**
 * Map Deephaven element names to ARIA roles for better testability.
 */
function getRole(name) {
    if (name.includes('Button')) return 'button';
    if (name.includes('TextField') || name.includes('TextInput')) return 'textbox';
    if (name.includes('Checkbox')) return 'checkbox';
    if (name.includes('Slider')) return 'slider';
    if (name.includes('Tab')) return 'tab';
    if (name.includes('Table') || name.includes('UITable')) return 'table';
    if (name.includes('Panel')) return 'region';
    if (name.includes('Dialog') || name.includes('Modal')) return 'dialog';
    if (name.includes('Menu')) return 'menu';
    if (name.includes('List')) return 'list';
    if (name.includes('Item')) return 'listitem';
    if (name.includes('Heading')) return 'heading';
    if (name.includes('Link')) return 'link';
    if (name.includes('Image')) return 'img';
    if (name.includes('Switch') || name.includes('Toggle')) return 'switch';
    if (name.includes('Radio')) return 'radio';
    if (name.includes('Select') || name.includes('Picker') || name.includes('ComboBox')) return 'combobox';
    return undefined;
}

/**
 * Panel component - wraps children with a title
 */
function Panel(props) {
    const { title, children, __dhElementName, ...rest } = props || {};

    const dataProps = {};
    for (const [key, value] of Object.entries(rest)) {
        if (typeof value === 'string' || typeof value === 'number' || typeof value === 'boolean') {
            dataProps[`data-${key.toLowerCase()}`] = String(value);
        }
    }

    return h('section', {
        'data-component': __dhElementName || 'deephaven.ui.components.Panel',
        'data-title': title,
        role: 'region',
        'aria-label': title,
        ...dataProps,
    },
        title ? h('h2', null, title) : null,
        children
    );
}
Panel.displayName = 'Panel';

/**
 * Button component
 */
function Button(props) {
    const { children, onPress, onClick, isDisabled, variant, __dhElementName, ...rest } = props || {};

    return h('button', {
        'data-component': __dhElementName || 'deephaven.ui.components.Button',
        onClick: onPress || onClick,
        disabled: isDisabled,
        'data-variant': variant,
        role: 'button',
    }, children);
}
Button.displayName = 'Button';

/**
 * TextField component - handles both DOM events and server-side callables.
 * DH widgets wire onChange as a callable (__dhCbid), which the DocumentRenderer
 * converts to a function. This component calls that function with the input value.
 */
function TextField(props) {
    const { label, value, defaultValue, onChange, onValueChange, isDisabled, __dhElementName, ...rest } = props || {};

    const handleChange = (e) => {
        const val = e.target.value;
        // onChange/onValueChange may be server-side callables (functions that send JSON-RPC)
        if (typeof onChange === 'function') onChange(val);
        if (typeof onValueChange === 'function') onValueChange(val);
    };

    return h('div', { 'data-component': __dhElementName || 'deephaven.ui.components.TextField' },
        label ? h('label', null, label) : null,
        h('input', {
            type: 'text',
            value: value,
            defaultValue: defaultValue,
            onChange: handleChange,
            disabled: isDisabled,
            role: 'textbox',
            'aria-label': label,
        })
    );
}
TextField.displayName = 'TextField';

/**
 * NumberField component
 */
function NumberField(props) {
    const { label, value, defaultValue, onChange, onValueChange, isDisabled, minValue, maxValue, step, __dhElementName, ...rest } = props || {};

    const handleChange = (e) => {
        const val = Number(e.target.value);
        if (typeof onChange === 'function') onChange(val);
        if (typeof onValueChange === 'function') onValueChange(val);
    };

    return h('div', { 'data-component': __dhElementName || 'deephaven.ui.components.NumberField' },
        label ? h('label', null, label) : null,
        h('input', {
            type: 'number',
            value: value,
            defaultValue: defaultValue,
            min: minValue,
            max: maxValue,
            step: step,
            onChange: handleChange,
            disabled: isDisabled,
            role: 'spinbutton',
            'aria-label': label,
        })
    );
}
NumberField.displayName = 'NumberField';

/**
 * TextArea component
 */
function TextArea(props) {
    const { label, value, defaultValue, onChange, onValueChange, isDisabled, __dhElementName, ...rest } = props || {};

    const handleChange = (e) => {
        const val = e.target.value;
        if (typeof onChange === 'function') onChange(val);
        if (typeof onValueChange === 'function') onValueChange(val);
    };

    return h('div', { 'data-component': __dhElementName || 'deephaven.ui.components.TextArea' },
        label ? h('label', null, label) : null,
        h('textarea', {
            value: value,
            defaultValue: defaultValue,
            onChange: handleChange,
            disabled: isDisabled,
            role: 'textbox',
            'aria-label': label,
        })
    );
}
TextArea.displayName = 'TextArea';

/**
 * Text component
 */
function Text(props) {
    const { children, __dhElementName, ...rest } = props || {};
    return h('span', { 'data-component': __dhElementName || 'deephaven.ui.components.Text' }, children);
}
Text.displayName = 'Text';

/**
 * Flex layout
 */
function Flex(props) {
    const { children, direction, gap, alignItems, justifyContent, __dhElementName, ...rest } = props || {};
    return h('div', {
        'data-component': __dhElementName || 'deephaven.ui.components.Flex',
        'data-direction': direction,
        role: 'group',
    }, children);
}
Flex.displayName = 'Flex';

/**
 * Default component map - maps DH element names to React components.
 */
export const DEFAULT_COMPONENT_MAP = {
    // Layout
    'deephaven.ui.components.Panel': Panel,
    'deephaven.ui.components.Flex': Flex,
    'deephaven.ui.components.Column': createStubComponent('Column'),
    'deephaven.ui.components.Row': createStubComponent('Row'),
    'deephaven.ui.components.Stack': createStubComponent('Stack'),
    'deephaven.ui.components.View': createStubComponent('View'),
    'deephaven.ui.components.Grid': createStubComponent('Grid'),

    // Data
    'deephaven.ui.elements.UITable': UITableStub,

    // Input
    'deephaven.ui.components.Button': Button,
    'deephaven.ui.components.ActionButton': Button,
    'deephaven.ui.components.TextField': TextField,
    'deephaven.ui.components.TextArea': TextArea,
    'deephaven.ui.components.NumberField': NumberField,
    'deephaven.ui.components.Checkbox': createStubComponent('Checkbox'),
    'deephaven.ui.components.Switch': createStubComponent('Switch'),
    'deephaven.ui.components.Slider': createStubComponent('Slider'),
    'deephaven.ui.components.RangeSlider': createStubComponent('RangeSlider'),
    'deephaven.ui.components.RadioGroup': createStubComponent('RadioGroup'),
    'deephaven.ui.components.Radio': createStubComponent('Radio'),
    'deephaven.ui.components.Picker': createStubComponent('Picker'),
    'deephaven.ui.components.ComboBox': createStubComponent('ComboBox'),
    'deephaven.ui.components.DatePicker': createStubComponent('DatePicker'),
    'deephaven.ui.components.TimePicker': createStubComponent('TimePicker'),

    // Display
    'deephaven.ui.components.Text': Text,
    'deephaven.ui.components.Heading': createStubComponent('Heading', 'h3'),
    'deephaven.ui.components.Content': createStubComponent('Content'),
    'deephaven.ui.components.IllustratedMessage': createStubComponent('IllustratedMessage'),
    'deephaven.ui.components.Badge': createStubComponent('Badge'),
    'deephaven.ui.components.ProgressBar': createStubComponent('ProgressBar'),
    'deephaven.ui.components.ProgressCircle': createStubComponent('ProgressCircle'),
    'deephaven.ui.components.StatusLight': createStubComponent('StatusLight'),

    // Navigation
    'deephaven.ui.components.TabList': createStubComponent('TabList'),
    'deephaven.ui.components.Tabs': createStubComponent('Tabs'),
    'deephaven.ui.components.Tab': createStubComponent('Tab'),
    'deephaven.ui.components.TabPanels': createStubComponent('TabPanels'),
    'deephaven.ui.components.Link': createStubComponent('Link', 'a'),
    'deephaven.ui.components.ActionMenu': createStubComponent('ActionMenu'),
    'deephaven.ui.components.ActionGroup': createStubComponent('ActionGroup'),

    // Overlay
    'deephaven.ui.components.Dialog': createStubComponent('Dialog'),
    'deephaven.ui.components.DialogTrigger': createStubComponent('DialogTrigger'),
    'deephaven.ui.components.ContextualHelp': createStubComponent('ContextualHelp'),
    'deephaven.ui.components.Tooltip': createStubComponent('Tooltip'),
    'deephaven.ui.components.TooltipTrigger': createStubComponent('TooltipTrigger'),

    // Collections
    'deephaven.ui.components.ListView': createStubComponent('ListView'),
    'deephaven.ui.components.Item': createStubComponent('Item'),

    // Forms
    'deephaven.ui.components.Form': createStubComponent('Form', 'form'),
};

/**
 * Create a merged component map with user overrides.
 * @param {object} overrides - Component overrides
 * @returns {object} Merged component map
 */
export function createComponentMap(overrides = {}) {
    return { ...DEFAULT_COMPONENT_MAP, ...overrides };
}
