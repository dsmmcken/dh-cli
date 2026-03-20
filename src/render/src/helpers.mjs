/**
 * Helper utilities for working with Deephaven UI documents and rendered output.
 */

const CALLABLE_KEY = '__dhCbid';
const OBJECT_KEY = '__dhObid';
const ELEMENT_KEY = '__dhElemName';

/**
 * Find all callables in a document tree.
 * @param {object} doc - The document
 * @returns {Array<{path: string, id: string, parentElement: string}>}
 */
export function findAllCallables(doc) {
    const results = [];
    _walk(doc, '', (path, value, parentElement) => {
        if (value && typeof value === 'object' && CALLABLE_KEY in value) {
            results.push({ path, id: value[CALLABLE_KEY], parentElement });
        }
    });
    return results;
}

/**
 * Find all exported object references in a document tree.
 * @param {object} doc - The document
 * @returns {Array<{path: string, id: number, parentElement: string}>}
 */
export function findAllObjects(doc) {
    const results = [];
    _walk(doc, '', (path, value, parentElement) => {
        if (value && typeof value === 'object' && OBJECT_KEY in value) {
            results.push({ path, id: value[OBJECT_KEY], parentElement });
        }
    });
    return results;
}

/**
 * Find all element nodes in a document tree.
 * @param {object} doc - The document
 * @returns {Array<{path: string, name: string, props: object}>}
 */
export function findAllElements(doc) {
    const results = [];
    _walk(doc, '', (path, value) => {
        if (value && typeof value === 'object' && ELEMENT_KEY in value) {
            results.push({ path, name: value[ELEMENT_KEY], props: value.props });
        }
    });
    return results;
}

/**
 * Find a callable by its property name (e.g., 'onPress') within an element that matches a predicate.
 * @param {object} doc - The document
 * @param {string} propName - The property name (e.g., 'onPress', 'onChange')
 * @param {function} [elementPredicate] - Optional predicate to match the parent element
 * @returns {string|null} The callable ID, or null if not found
 */
export function findCallableByProp(doc, propName, elementPredicate = null) {
    const elements = findAllElements(doc);
    for (const el of elements) {
        if (elementPredicate && !elementPredicate(el)) continue;
        const prop = el.props?.[propName];
        if (prop && typeof prop === 'object' && CALLABLE_KEY in prop) {
            return prop[CALLABLE_KEY];
        }
    }
    return null;
}

/**
 * Find a callable by the text content of its parent button/element.
 * Searches for buttons with matching children text.
 * @param {object} doc - The document
 * @param {string} text - The button text to match
 * @returns {string|null} The callable ID
 */
export function findCallableByButtonText(doc, text) {
    return findCallableByProp(doc, 'onPress', (el) => {
        const children = el.props?.children;
        if (typeof children === 'string') return children === text;
        if (Array.isArray(children)) return children.some(c => c === text);
        return false;
    });
}

/**
 * Find a callable by element name and property.
 * @param {object} doc - The document
 * @param {string} elementName - The element name to match
 * @param {string} propName - The property name (default: 'onPress')
 * @returns {string|null}
 */
export function findCallableByElement(doc, elementName, propName = 'onPress') {
    return findCallableByProp(doc, propName, (el) => el.name === elementName);
}

/**
 * Get a value at a dot-separated path in the document.
 * @param {object} doc - The document
 * @param {string} path - Dot-separated path (e.g., 'props.children.0.props.title')
 * @returns {*}
 */
export function getAtPath(doc, path) {
    const parts = path.split('.');
    let current = doc;
    for (const part of parts) {
        if (current == null) return undefined;
        current = current[part];
    }
    return current;
}

/**
 * Pretty-print a document tree for debugging.
 * @param {object} doc - The document
 * @param {number} [indent=0]
 * @returns {string}
 */
export function prettyPrintDocument(doc, indent = 0) {
    if (!doc || typeof doc !== 'object') return String(doc);

    const prefix = '  '.repeat(indent);
    const lines = [];

    if (ELEMENT_KEY in doc) {
        const name = doc[ELEMENT_KEY];
        const props = doc.props || {};
        const { children, ...otherProps } = props;

        const propStr = Object.entries(otherProps)
            .filter(([k, v]) => typeof v !== 'object' || (v && CALLABLE_KEY in v))
            .map(([k, v]) => {
                if (v && typeof v === 'object' && CALLABLE_KEY in v) return `${k}={${v[CALLABLE_KEY]}}`;
                return `${k}=${JSON.stringify(v)}`;
            })
            .join(' ');

        lines.push(`${prefix}<${name.split('.').pop()}${propStr ? ' ' + propStr : ''}>`);

        if (children !== undefined) {
            if (Array.isArray(children)) {
                for (const child of children) {
                    lines.push(prettyPrintDocument(child, indent + 1));
                }
            } else if (typeof children === 'string') {
                lines.push(`${prefix}  ${children}`);
            } else if (typeof children === 'object') {
                lines.push(prettyPrintDocument(children, indent + 1));
            }
        }

        lines.push(`${prefix}</${name.split('.').pop()}>`);
    } else if (OBJECT_KEY in doc) {
        lines.push(`${prefix}[Object #${doc[OBJECT_KEY]}]`);
    } else if (CALLABLE_KEY in doc) {
        lines.push(`${prefix}[Callable ${doc[CALLABLE_KEY]}]`);
    }

    return lines.join('\n');
}

// Internal walk helper
function _walk(node, path, callback, parentElement = null) {
    if (!node || typeof node !== 'object') return;

    callback(path, node, parentElement);

    const currentElement = (ELEMENT_KEY in node) ? node[ELEMENT_KEY] : parentElement;

    if (Array.isArray(node)) {
        node.forEach((item, i) => _walk(item, `${path}[${i}]`, callback, currentElement));
    } else {
        for (const [key, value] of Object.entries(node)) {
            if (typeof value === 'object' && value !== null) {
                _walk(value, path ? `${path}.${key}` : key, callback, currentElement);
            }
        }
    }
}
