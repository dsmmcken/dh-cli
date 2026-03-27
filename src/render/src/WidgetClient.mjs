/**
 * WidgetClient - Manages connection to a Deephaven widget and its JSON-RPC protocol.
 *
 * Handles:
 * - Connecting to the server and fetching the widget
 * - JSON-RPC bidirectional communication
 * - Document tree management (applying patches)
 * - Exported object tracking
 * - Callable (event handler) invocation
 */
import { JSONRPCServer, JSONRPCClient, JSONRPCServerAndClient } from 'json-rpc-2.0';
import fastJsonPatch from 'fast-json-patch';
// Note: NodeHttp2gRPCTransport causes widget message streams to hang.
// The default fetch-based transport works correctly for all operations.

const { applyPatch } = fastJsonPatch;

const CALLABLE_KEY = '__dhCbid';
const OBJECT_KEY = '__dhObid';
const ELEMENT_KEY = '__dhElemName';

export { CALLABLE_KEY, OBJECT_KEY, ELEMENT_KEY };

export class WidgetClient {
    /**
     * @param {object} dh - The Deephaven JSAPI object
     * @param {string} serverUrl - The server URL
     */
    constructor(dh, serverUrl) {
        this.dh = dh;
        this.serverUrl = serverUrl;
        this.client = null;
        this.connection = null;
        this.widget = null;
        this.jsonClient = null;

        // Document state
        this.document = {};
        this.exportedObjectMap = new Map();
        this._exportedObjectCount = 0;
        this.lastError = null;

        // Event callbacks
        this._onDocumentUpdate = null;
        this._onError = null;
        this._onEvent = null;

        // Promise for waiting for document
        this._documentReady = null;
        this._documentReadyResolve = null;
        this._documentReadyReject = null;
    }

    /**
     * Connect to the Deephaven server.
     */
    async connect() {
        this.client = new this.dh.CoreClient(this.serverUrl);
        await this.client.login({ type: this.dh.CoreClient.LOGIN_TYPE_ANONYMOUS });
        this.connection = await this.client.getAsIdeConnection();
    }

    /**
     * Fetch the variable definition for a named field using the official
     * subscribeToFieldUpdates callback API (matches @deephaven/jsapi-utils
     * fetchVariableDefinition pattern).
     *
     * @param {string} name - Field name
     * @param {number} [timeout=5000] - Timeout in ms
     * @returns {Promise<{title: string, type: string}|null>} The variable definition, or null
     */
    async fetchVariableDefinition(name, timeout = 5000) {
        if (!this.connection) return null;

        return new Promise((resolve) => {
            let removeListener;
            const timeoutId = setTimeout(() => {
                removeListener?.();
                resolve(null);
            }, timeout);

            function handleFieldUpdates(changes) {
                clearTimeout(timeoutId);
                removeListener?.();

                // changes has .created, .updated, .removed arrays of VariableDefinition
                const allFields = [
                    ...(changes.created || []),
                    ...(changes.updated || []),
                ];

                // Find by title (standard VariableDefinition property)
                let definition = allFields.find(def => def.title === name);

                // GWT objects may use name/name_0 instead of title
                if (!definition) {
                    definition = allFields.find(def =>
                        (def.name === name || def.name_0 === name)
                    );
                }

                if (definition) {
                    resolve({
                        title: definition.title || definition.name || definition.name_0,
                        type: definition.type || definition.type_0,
                    });
                } else {
                    resolve(null);
                }
            }

            try {
                // Use the direct callback API: subscribeToFieldUpdates(callback)
                // returns a removeListener function. This is the official pattern
                // from @deephaven/jsapi-utils ConnectionUtils.
                removeListener = this.connection.subscribeToFieldUpdates(handleFieldUpdates);
            } catch (e) {
                // Fall back to event-based API if direct callback isn't supported
                try {
                    const handler = (event) => {
                        handleFieldUpdates(event.detail || event);
                    };
                    this.connection.addEventListener('fieldUpdates', handler);
                    removeListener = () => {
                        this.connection.removeEventListener('fieldUpdates', handler);
                    };
                    this.connection.subscribeToFieldUpdates();
                } catch (e2) {
                    clearTimeout(timeoutId);
                    resolve(null);
                }
            }
        });
    }

    /**
     * Discover all renderable widgets on the server.
     * Returns them sorted by priority: Dashboard > Element.
     *
     * @param {number} [timeout=5000] - Timeout in ms
     * @returns {Promise<Array<{name: string, type: string}>>}
     */
    async discoverWidgets(timeout = 5000) {
        if (!this.connection) return [];

        const fields = await new Promise((resolve) => {
            let removeListener;
            const timeoutId = setTimeout(() => {
                removeListener?.();
                resolve(null);
            }, timeout);

            function handleFieldUpdates(changes) {
                clearTimeout(timeoutId);
                removeListener?.();
                resolve(changes);
            }

            try {
                removeListener = this.connection.subscribeToFieldUpdates(handleFieldUpdates);
            } catch (e) {
                try {
                    const handler = (event) => handleFieldUpdates(event.detail || event);
                    this.connection.addEventListener('fieldUpdates', handler);
                    removeListener = () => this.connection.removeEventListener('fieldUpdates', handler);
                    this.connection.subscribeToFieldUpdates();
                } catch (e2) {
                    clearTimeout(timeoutId);
                    resolve(null);
                }
            }
        });

        if (!fields) return [];

        const allFields = [
            ...(fields.created || []),
            ...(fields.updated || []),
        ];

        const RENDERABLE_TYPES = ['deephaven.ui.Dashboard', 'deephaven.ui.Element'];
        const TYPE_PRIORITY = { 'deephaven.ui.Dashboard': 0, 'deephaven.ui.Element': 1 };

        return allFields
            .filter(f => {
                const type = f.type || f.type_0;
                return RENDERABLE_TYPES.includes(type);
            })
            .map(f => ({
                name: f.title || f.name || f.name_0,
                type: f.type || f.type_0,
            }))
            .sort((a, b) => (TYPE_PRIORITY[a.type] ?? 99) - (TYPE_PRIORITY[b.type] ?? 99));
    }

    /**
     * Open a widget by name and type, set up JSON-RPC, and wait for the initial document.
     * @param {string} name - Widget name
     * @param {string} [type] - Widget type (auto-detected if not specified)
     * @param {number} [timeout=10000] - Timeout in ms to wait for document
     * @returns {Promise<object>} The initial document
     */
    async openWidget(name, type, timeout = 10000) {
        if (!this.connection) {
            throw new Error('Not connected. Call connect() first.');
        }

        // Auto-detect type from server if not specified
        if (!type) {
            const definition = await this.fetchVariableDefinition(name, 3000);
            if (definition?.type) {
                type = definition.type;
            }
        }

        // If still no type, try common types sequentially
        if (!type) {
            const types = ['deephaven.ui.Element', 'deephaven.ui.Dashboard', 'Table', 'Figure'];
            for (const t of types) {
                try {
                    this.widget = await this.connection.getObject({ name, type: t });
                    type = t;
                    break;
                } catch (e) {
                    // Try next type
                }
            }
            if (!type) {
                throw new Error(`Widget "${name}" not found (tried auto-detect and: ${['deephaven.ui.Element', 'deephaven.ui.Dashboard', 'Table', 'Figure'].join(', ')})`);
            }
        } else {
            this.widget = await this.connection.getObject({ name, type });
        }

        // Reset state
        this.document = {};
        this.exportedObjectMap.clear();
        this._exportedObjectCount = 0;

        // Create promise for document ready
        this._documentReady = new Promise((resolve, reject) => {
            this._documentReadyResolve = resolve;
            this._documentReadyReject = reject;
            const timer = setTimeout(() => {
                reject(new Error(`Timeout waiting for document after ${timeout}ms`));
            }, timeout);
            // Store timer so we can clear it
            this._documentReadyTimer = timer;
        });

        // Set up JSON-RPC
        this.jsonClient = new JSONRPCServerAndClient(
            new JSONRPCServer(),
            new JSONRPCClient((request) => {
                this.widget.sendMessage(JSON.stringify(request), []);
            })
        );

        this._setupJsonRpcMethods();
        this._setupWidgetListener();

        // Process initial data
        this._updateExportedObjects(this.widget.exportedObjects);
        const initialData = this.widget.getDataAsString();
        if (initialData.length > 0) {
            await this.jsonClient.receiveAndSend(JSON.parse(initialData));
        }

        // Send setState to trigger initial render
        await this.jsonClient.request('setState', [{}]);

        // Wait for the document
        return this._documentReady;
    }

    /**
     * Call a callable (event handler) on the server.
     * @param {string} callableId - The callable ID (e.g., 'cb0')
     * @param {Array} args - Arguments to pass
     * @returns {Promise<any>} The result
     */
    async callCallable(callableId, args = []) {
        if (!this.jsonClient) {
            throw new Error('No widget open. Call openWidget() first.');
        }
        return this.jsonClient.request('callCallable', [callableId, args]);
    }

    /**
     * Wait for the next document update (re-render).
     * @param {number} [timeout=5000] - Timeout in ms
     * @returns {Promise<object>} The updated document
     */
    waitForUpdate(timeout = 5000) {
        return new Promise((resolve, reject) => {
            const timer = setTimeout(() => {
                reject(new Error(`Timeout waiting for document update after ${timeout}ms`));
            }, timeout);

            const prevCallback = this._onDocumentUpdate;
            this._onDocumentUpdate = (doc) => {
                clearTimeout(timer);
                this._onDocumentUpdate = prevCallback;
                resolve(doc);
            };
        });
    }

    /**
     * Fetch a table from an exported object.
     * @param {number} objectId - The object ID from the document (__dhObid value)
     * @returns {Promise<object>} The fetched table
     */
    async fetchTable(objectId) {
        const exportedObject = this.exportedObjectMap.get(objectId);
        if (!exportedObject) {
            throw new Error(`Exported object ${objectId} not found`);
        }
        const reexported = await exportedObject.reexport();
        return reexported.fetch();
    }

    /**
     * Get table data as an array of row objects.
     * @param {object} table - A fetched DH table
     * @param {number} [startRow=0] - Start row
     * @param {number} [endRow] - End row (defaults to table.size - 1)
     * @returns {Promise<Array<object>>}
     */
    async getTableData(table, startRow = 0, endRow = undefined) {
        if (endRow === undefined) {
            endRow = table.size - 1;
        }
        table.setViewport(startRow, endRow);
        const viewportData = await table.getViewportData();
        const rows = [];
        for (let i = 0; i < viewportData.rows.length; i++) {
            const row = {};
            for (const col of table.columns) {
                row[col.name] = viewportData.rows[i].get(col);
            }
            rows.push(row);
        }
        return rows;
    }

    /**
     * Hydrate a document: replace __dhObid with exported objects,
     * __dhCbid with callable functions.
     * @param {object} doc - The document to hydrate
     * @returns {object} The hydrated document
     */
    hydrateDocument(doc) {
        return this._transformNode(doc, (key, value) => {
            if (value && typeof value === 'object') {
                if (CALLABLE_KEY in value) {
                    const callableId = value[CALLABLE_KEY];
                    return (...args) => this.callCallable(callableId, args);
                }
                if (OBJECT_KEY in value) {
                    const objectId = value[OBJECT_KEY];
                    return this.exportedObjectMap.get(objectId);
                }
            }
            return value;
        });
    }

    /**
     * Subscribe to a ticking table and receive row updates via callback.
     * Returns an unsubscribe function.
     *
     * @param {number} objectId - The exported object ID
     * @param {function} onUpdate - Called with ({ columns, rows }) on each update
     * @param {object} [options]
     * @param {number} [options.startRow=0] - Start row of viewport
     * @param {number} [options.endRow=99] - End row of viewport
     * @returns {Promise<{ unsubscribe: function, table: object }>}
     */
    async subscribeTable(objectId, onUpdate, options = {}) {
        const { startRow = 0, endRow = 99 } = options;
        const table = await this.fetchTable(objectId);
        const columns = table.columns.map(c => ({ name: c.name, type: c.type }));

        const listener = (event) => {
            const viewportData = event.detail;
            const rows = [];
            for (let i = 0; i < viewportData.rows.length; i++) {
                const row = {};
                for (const col of table.columns) {
                    row[col.name] = viewportData.rows[i].get(col);
                }
                rows.push(row);
            }
            onUpdate({ columns, rows });
        };

        table.addEventListener('updated', listener);
        table.setViewport(startRow, endRow);

        return {
            table,
            unsubscribe: () => {
                table.removeEventListener('updated', listener);
                table.close();
            },
        };
    }

    /**
     * Subscribe to a ticking table by name (not from an exported widget object).
     * Useful for subscribing to raw server tables.
     *
     * @param {string} tableName - The table name on the server
     * @param {function} onUpdate - Called with ({ columns, rows }) on each update
     * @param {object} [options]
     * @param {number} [options.startRow=0]
     * @param {number} [options.endRow=99]
     * @returns {Promise<{ unsubscribe: function, table: object }>}
     */
    async subscribeNamedTable(tableName, onUpdate, options = {}) {
        const { startRow = 0, endRow = 99 } = options;

        if (!this.connection) {
            throw new Error('Not connected. Call connect() first.');
        }

        const tableObj = await this.connection.getObject({ name: tableName, type: 'Table' });
        let table;
        if (tableObj.columns) {
            table = tableObj;
        } else if (tableObj.reexport) {
            const reexported = await tableObj.reexport();
            table = await reexported.fetch();
        } else {
            throw new Error(`Could not fetch table "${tableName}"`);
        }

        const columns = table.columns.map(c => ({ name: c.name, type: c.type }));

        const listener = (event) => {
            const viewportData = event.detail;
            const rows = [];
            for (let i = 0; i < viewportData.rows.length; i++) {
                const row = {};
                for (const col of table.columns) {
                    row[col.name] = viewportData.rows[i].get(col);
                }
                rows.push(row);
            }
            onUpdate({ columns, rows });
        };

        table.addEventListener('updated', listener);
        table.setViewport(startRow, endRow);

        return {
            table,
            columns,
            unsubscribe: () => {
                table.removeEventListener('updated', listener);
                table.close();
            },
        };
    }

    /**
     * Wait for N updates on a ticking table subscription.
     * Useful for testing that a table is actually ticking.
     *
     * @param {number} objectId - The exported object ID (or use subscribeNamedTable directly)
     * @param {number} [count=1] - Number of updates to wait for
     * @param {number} [timeout=10000] - Timeout in ms
     * @returns {Promise<Array<{columns, rows}>>} Array of update snapshots
     */
    async waitForTableUpdates(objectId, count = 1, timeout = 10000) {
        const updates = [];
        return new Promise(async (resolve, reject) => {
            const timer = setTimeout(() => {
                sub.unsubscribe();
                if (updates.length > 0) {
                    resolve(updates);
                } else {
                    reject(new Error(`Timeout waiting for ${count} table updates after ${timeout}ms (got ${updates.length})`));
                }
            }, timeout);

            const sub = await this.subscribeTable(objectId, (data) => {
                updates.push(data);
                if (updates.length >= count) {
                    clearTimeout(timer);
                    sub.unsubscribe();
                    resolve(updates);
                }
            });
        });
    }

    /**
     * Close the widget and clean up.
     */
    close() {
        if (this.widget) {
            this.widget.close();
            this.widget = null;
        }
        if (this.jsonClient) {
            this.jsonClient.rejectAllPendingRequests('Widget closed');
            this.jsonClient = null;
        }
        this.document = {};
        this.exportedObjectMap.clear();
    }

    // --- Private methods ---

    _setupJsonRpcMethods() {
        this.jsonClient.addMethod('documentPatched', (params) => {
            const [patch, stateParam] = params;
            const result = applyPatch(this.document, patch, false, true);
            this.document = result.newDocument;

            // Resolve the document ready promise on first document
            if (this._documentReadyResolve) {
                clearTimeout(this._documentReadyTimer);
                this._documentReadyResolve(this.document);
                this._documentReadyResolve = null;
                this._documentReadyReject = null;
            }

            if (this._onDocumentUpdate) {
                this._onDocumentUpdate(this.document);
            }
        });

        this.jsonClient.addMethod('documentError', (params) => {
            const error = JSON.parse(params[0]);
            this.lastError = error;
            if (this._onError) {
                this._onError(error);
            }
            // Reject the document ready promise if it's pending
            if (this._documentReadyReject) {
                clearTimeout(this._documentReadyTimer);
                const msg = error.message || error.detail || JSON.stringify(error);
                this._documentReadyReject(new Error(`Document error: ${msg}`));
                this._documentReadyResolve = null;
                this._documentReadyReject = null;
            }
        });

        this.jsonClient.addMethod('event', (params) => {
            const [name, payload] = params;
            if (this._onEvent) {
                this._onEvent(name, JSON.parse(payload));
            }
        });
    }

    _setupWidgetListener() {
        this.widget.addEventListener('message', (event) => {
            const data = event.detail.getDataAsString();
            const newExportedObjects = event.detail.exportedObjects;
            this._updateExportedObjects(newExportedObjects);
            if (data.length > 0) {
                this.jsonClient.receiveAndSend(JSON.parse(data));
            }
        });
    }

    _updateExportedObjects(newObjects) {
        for (const obj of newObjects) {
            this.exportedObjectMap.set(this._exportedObjectCount, obj);
            this._exportedObjectCount++;
        }
    }

    _transformNode(node, transform) {
        if (node === null || node === undefined) {
            return node;
        }
        if (Array.isArray(node)) {
            return node.map((item, i) => {
                const transformed = transform(String(i), item);
                if (transformed !== item) return transformed;
                if (typeof item === 'object' && item !== null) {
                    return this._transformNode(item, transform);
                }
                return item;
            });
        }
        if (typeof node === 'object') {
            // Check if this node itself should be transformed
            const selfTransformed = transform('', node);
            if (selfTransformed !== node) return selfTransformed;

            const result = {};
            for (const [key, value] of Object.entries(node)) {
                const transformed = transform(key, value);
                if (transformed !== value) {
                    result[key] = transformed;
                } else if (typeof value === 'object' && value !== null) {
                    result[key] = this._transformNode(value, transform);
                } else {
                    result[key] = value;
                }
            }
            return result;
        }
        return node;
    }
}
