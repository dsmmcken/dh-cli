/**
 * DocumentRenderer - Converts a Deephaven document tree into React elements.
 *
 * Takes the raw document (with __dhElemName, __dhCbid, __dhObid markers)
 * and produces a React element tree that can be rendered with react-testing-library.
 */
import React from 'react';
import { DEFAULT_COMPONENT_MAP } from './ComponentMap.mjs';
import { FigureStub, isFigureType } from './FigureStub.mjs';
import { UITableStub } from './UITableStub.mjs';

const { createElement: h, Fragment } = React;

const CALLABLE_KEY = '__dhCbid';
const OBJECT_KEY = '__dhObid';
const ELEMENT_KEY = '__dhElemName';
const HTML_ELEMENT_PREFIX = 'deephaven.ui.html.';

function isTableType(type) {
    return type === 'Table';
}

/**
 * Placeholder for exported objects (tables, figures, etc.) when rendered as children.
 */
function ExportedObjectPlaceholder({ objectId, exportedObject }) {
    const type = exportedObject?.type || 'unknown';
    return h('div', {
        'data-component': 'dh.ExportedObject',
        'data-object-id': String(objectId),
        'data-object-type': type,
        role: type === 'Table' ? 'table' : type === 'Figure' ? 'img' : 'presentation',
    }, h('span', { 'data-testid': `exported-object-${objectId}` }, `[${type} #${objectId}]`));
}

export class DocumentRenderer {
    /**
     * @param {object} options
     * @param {Map<number, object>} options.exportedObjectMap - Map of object IDs to exported objects
     * @param {function} options.callCallable - Function to call a server-side callable
     * @param {object} [options.componentMap] - Custom component map
     */
    constructor({ exportedObjectMap, callCallable, componentMap = DEFAULT_COMPONENT_MAP }) {
        this.exportedObjectMap = exportedObjectMap;
        this.callCallable = callCallable;
        this.componentMap = componentMap;
    }

    /**
     * Render a document tree into React elements.
     * @param {object} doc - The document tree
     * @returns {React.ReactNode}
     */
    render(doc) {
        if (!doc || typeof doc !== 'object') {
            return null;
        }
        return this._renderNode(doc, '0');
    }

    _renderNode(node, key) {
        if (node === null || node === undefined) {
            return null;
        }

        // Primitives
        if (typeof node === 'string' || typeof node === 'number' || typeof node === 'boolean') {
            return node;
        }

        // Arrays
        if (Array.isArray(node)) {
            return h(Fragment, null, ...node.map((item, i) => this._renderNode(item, `${key}-${i}`)));
        }

        // Callable node - replace with a function
        if (CALLABLE_KEY in node) {
            const callableId = node[CALLABLE_KEY];
            return (...args) => this.callCallable(callableId, args);
        }

        // Object node - wrap in placeholder when rendered as a child
        if (OBJECT_KEY in node) {
            const objectId = node[OBJECT_KEY];
            const exportedObject = this.exportedObjectMap.get(objectId);
            if (isFigureType(exportedObject?.type)) {
                return h(FigureStub, { key, objectId, exportedObject });
            }
            if (isTableType(exportedObject?.type)) {
                return h(UITableStub, { key, table: exportedObject });
            }
            return h(ExportedObjectPlaceholder, { key, objectId, exportedObject });
        }

        // Element node - render as a React component
        if (ELEMENT_KEY in node) {
            return this._renderElement(node, key);
        }

        // Plain object - this shouldn't normally happen at the top level,
        // but could be nested props
        return null;
    }

    _renderElement(node, key) {
        const elementName = node[ELEMENT_KEY];
        const props = node.props || {};

        // Handle deephaven.ui.html.* elements — render as native HTML tags.
        // Mirrors the real HTMLElementView.tsx behavior: extract the tag name
        // from the element name prefix and use React.createElement(tag, ...).
        if (elementName.startsWith(HTML_ELEMENT_PREFIX)) {
            const tag = elementName.substring(HTML_ELEMENT_PREFIX.length);
            const processedProps = this._processProps(props, key);
            const { children, ...otherProps } = processedProps;
            return h(tag, { key, ...otherProps }, children);
        }

        // Look up the component
        let Component = this.componentMap[elementName];
        if (!Component) {
            // Create a fallback component for unknown elements
            Component = (props) => {
                const { children, ...rest } = props || {};
                return h('div', {
                    'data-component': elementName,
                    'data-unknown': 'true',
                }, children);
            };
            Component.displayName = `Unknown(${elementName})`;
        }

        // Process props
        const processedProps = this._processProps(props, key);
        processedProps.key = key;
        processedProps.__dhElementName = elementName;

        return h(Component, processedProps);
    }

    _processProps(props, parentKey) {
        const result = {};

        for (const [key, value] of Object.entries(props)) {
            if (key === 'children') {
                // Children are renderable - exported objects need wrapping
                result[key] = this._processValue(value, `${parentKey}-${key}`, true);
            } else {
                result[key] = this._processValue(value, `${parentKey}-${key}`, false);
            }
        }

        return result;
    }

    /**
     * @param {*} value
     * @param {string} key
     * @param {boolean} isRenderable - true if this value will be rendered as a React child
     */
    _processValue(value, key, isRenderable = false) {
        if (value === null || value === undefined) {
            return value;
        }

        if (typeof value === 'string' || typeof value === 'number' || typeof value === 'boolean') {
            return value;
        }

        if (Array.isArray(value)) {
            return value.map((item, i) => this._processValue(item, `${key}-${i}`, isRenderable));
        }

        if (typeof value === 'object') {
            // Callable
            if (CALLABLE_KEY in value) {
                const callableId = value[CALLABLE_KEY];
                return (...args) => this.callCallable(callableId, args);
            }

            // Exported object
            if (OBJECT_KEY in value) {
                const objectId = value[OBJECT_KEY];
                const exportedObject = this.exportedObjectMap.get(objectId);
                if (isRenderable) {
                    if (isFigureType(exportedObject?.type)) {
                        return h(FigureStub, { key, objectId, exportedObject });
                    }
                    if (isTableType(exportedObject?.type)) {
                        return h(UITableStub, { key, table: exportedObject });
                    }
                    return h(ExportedObjectPlaceholder, { key, objectId, exportedObject });
                }
                return exportedObject;
            }

            // Nested element - render it
            if (ELEMENT_KEY in value) {
                return this._renderElement(value, key);
            }

            // Plain object - recurse into it (not renderable)
            const result = {};
            for (const [k, v] of Object.entries(value)) {
                result[k] = this._processValue(v, `${key}-${k}`, false);
            }
            return result;
        }

        return value;
    }
}
