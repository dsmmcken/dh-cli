/**
 * Mock factories for widget, dh, and connection objects.
 * Extracted from test_real_components.mjs patterns.
 */
import { createDocumentPatchedResponse } from './mock-documents.mjs';

/**
 * Create a mock widget object that returns a given document.
 */
export function createMockWidget(document, options = {}) {
    const jsonRpcResponse = createDocumentPatchedResponse(document);
    return {
        type: options.type || 'deephaven.ui.Element',
        exportedObjects: options.exportedObjects || [],
        getDataAsString: () => jsonRpcResponse,
        sendMessage: options.sendMessage || (() => {}),
        addEventListener: options.addEventListener || (() => () => {}),
        close: options.close || (() => {}),
    };
}

/**
 * Create a mock dh (JSAPI) object.
 */
export function createMockDh() {
    return { CoreClient: function() {} };
}

/**
 * Create a mock connection object.
 */
export function createMockConnection(overrides = {}) {
    return {
        getObject: overrides.getObject || (async (desc) => ({ type: 'Widget', name: desc.name })),
        subscribeToFieldUpdates: overrides.subscribeToFieldUpdates || (() => () => {}),
        ...overrides,
    };
}
