/**
 * diagnoseWidget - High-level agent-friendly API for quickly debugging components.
 *
 * Given a server URL and widget name, produces a structured diagnostic report:
 * - Did it connect?
 * - Did the widget render?
 * - What components are in the tree?
 * - Are there errors?
 * - What exported objects (tables/figures) exist?
 * - Can table data be fetched?
 *
 * Designed to be the primary entry point for AI agents that need to quickly
 * determine whether a component is working.
 *
 * Usage:
 *   import { diagnoseWidget } from 'dh-render-test';
 *   const report = await diagnoseWidget('http://localhost:10000', 'my_widget');
 *   console.log(JSON.stringify(report, null, 2));
 */
import { JsApiLoader } from './JsApiLoader.mjs';
import { WidgetClient } from './WidgetClient.mjs';
import { DocumentRenderer } from './DocumentRenderer.mjs';
import { DEFAULT_COMPONENT_MAP } from './ComponentMap.mjs';
import {
    findAllElements,
    findAllObjects,
    findAllCallables,
    prettyPrintDocument,
} from './helpers.mjs';

/**
 * Diagnose a widget: connect, render, inspect, and produce a structured report.
 *
 * @param {string} serverUrl - The Deephaven server URL
 * @param {string} widgetName - Name of the widget to diagnose
 * @param {object} [options]
 * @param {string} [options.widgetType] - Widget type (auto-detected if not specified)
 * @param {number} [options.timeout=15000] - Timeout in ms
 * @param {boolean} [options.fetchTables=true] - Whether to fetch table data
 * @param {number} [options.maxTableRows=5] - Max rows to fetch per table
 * @returns {Promise<DiagnosticReport>}
 */
export async function diagnoseWidget(serverUrl, widgetName, options = {}) {
    const {
        widgetType,
        timeout = 15000,
        fetchTables = true,
        maxTableRows = 5,
    } = options;

    const report = {
        widget: widgetName,
        widgetType: widgetType || '(auto-detect)',
        server: serverUrl,
        timestamp: new Date().toISOString(),
        status: 'unknown',
        connection: { success: false, error: null, timeMs: 0 },
        render: { success: false, error: null, timeMs: 0 },
        document: {
            elementCount: 0,
            exportedObjectCount: 0,
            callableCount: 0,
            componentTypes: [],
            tree: null,
        },
        tables: [],
        errors: [],
    };

    let loader, widgetClient;

    // Phase 1: Connect
    const connStart = Date.now();
    try {
        loader = new JsApiLoader(serverUrl);
        const { dh } = await loader.load();
        widgetClient = new WidgetClient(dh, serverUrl);
        await widgetClient.connect();
        report.connection.success = true;
        report.connection.timeMs = Date.now() - connStart;
    } catch (e) {
        const msg = _errorMessage(e);
        report.connection.error = msg;
        report.connection.timeMs = Date.now() - connStart;
        report.status = 'connection_failed';
        report.errors.push({ phase: 'connection', message: msg });
        _cleanup(loader, widgetClient);
        return report;
    }

    // Phase 2: Render
    const renderStart = Date.now();
    let doc;
    try {
        doc = await widgetClient.openWidget(widgetName, widgetType, timeout);
        report.render.success = true;
        report.render.timeMs = Date.now() - renderStart;
    } catch (e) {
        const msg = _errorMessage(e);
        report.render.error = msg;
        report.render.timeMs = Date.now() - renderStart;
        report.status = 'render_failed';
        report.errors.push({ phase: 'render', message: msg });
        // Check for server-side error
        if (widgetClient.lastError) {
            report.errors.push({
                phase: 'server',
                message: widgetClient.lastError.message || JSON.stringify(widgetClient.lastError),
                traceback: widgetClient.lastError.traceback,
            });
        }
        _cleanup(loader, widgetClient);
        return report;
    }

    // Phase 3: Inspect document
    try {
        const elements = findAllElements(doc);
        const objects = findAllObjects(doc);
        const callables = findAllCallables(doc);

        report.document.elementCount = elements.length;
        report.document.exportedObjectCount = objects.length;
        report.document.callableCount = callables.length;
        report.document.componentTypes = [...new Set(elements.map(e => e.name))];
        report.document.tree = prettyPrintDocument(doc);

        // Check for server error after render
        if (widgetClient.lastError) {
            report.errors.push({
                phase: 'server',
                message: widgetClient.lastError.message || JSON.stringify(widgetClient.lastError),
                traceback: widgetClient.lastError.traceback,
            });
        }
    } catch (e) {
        report.errors.push({ phase: 'inspect', message: _errorMessage(e) });
    }

    // Phase 4: Fetch table data
    if (fetchTables) {
        const tableObjects = [...widgetClient.exportedObjectMap.entries()]
            .filter(([_, obj]) => obj.type === 'Table');

        for (const [id, obj] of tableObjects) {
            const tableInfo = { objectId: id, type: 'Table', columns: [], rowCount: null, sampleRows: [], error: null };
            try {
                const table = await widgetClient.fetchTable(id);
                tableInfo.columns = table.columns.map(c => ({ name: c.name, type: c.type }));
                tableInfo.rowCount = table.size;

                const endRow = Math.min(maxTableRows - 1, table.size - 1);
                if (endRow >= 0) {
                    const rows = await widgetClient.getTableData(table, 0, endRow);
                    tableInfo.sampleRows = rows;
                }
            } catch (e) {
                tableInfo.error = e.message;
            }
            report.tables.push(tableInfo);
        }
    }

    // Final status
    report.status = report.errors.length > 0 ? 'rendered_with_errors' : 'ok';

    _cleanup(loader, widgetClient);
    return report;
}

/**
 * List all available widgets/objects on a Deephaven server.
 *
 * @param {string} serverUrl - The Deephaven server URL
 * @returns {Promise<Array<{name: string, type: string}>>}
 */
export async function listWidgets(serverUrl) {
    const loader = new JsApiLoader(serverUrl);
    const { dh } = await loader.load();

    const widgetClient = new WidgetClient(dh, serverUrl);
    await widgetClient.connect();
    const connection = widgetClient.connection;

    // Subscribe to field updates using the direct callback API
    // (matches @deephaven/jsapi-utils pattern)
    const fields = await new Promise((resolve) => {
        let removeListener;
        const timeoutId = setTimeout(() => {
            removeListener?.();
            resolve(null);
        }, 5000);

        function handleFieldUpdates(changes) {
            clearTimeout(timeoutId);
            removeListener?.();
            resolve(changes);
        }

        try {
            removeListener = connection.subscribeToFieldUpdates(handleFieldUpdates);
        } catch (e) {
            // Fall back to event-based API
            try {
                const handler = (event) => {
                    handleFieldUpdates(event.detail || event);
                };
                connection.addEventListener('fieldUpdates', handler);
                removeListener = () => {
                    connection.removeEventListener('fieldUpdates', handler);
                };
                connection.subscribeToFieldUpdates();
            } catch (e2) {
                clearTimeout(timeoutId);
                resolve(null);
            }
        }
    });

    const result = [];

    if (fields) {
        const allFields = [
            ...(fields.created || []),
            ...(fields.updated || []),
        ];
        for (const field of allFields) {
            // Standard VariableDefinition uses .title and .type
            // GWT objects may use .name/.name_0 and .type/.type_0
            const name = field.title || field.name || field.name_0;
            const type = field.type || field.type_0;
            if (name && type) {
                result.push({ name, type });
            }
        }
    }

    if (result.length === 0) {
        result.push({ name: '(field discovery not available)', type: 'info' });
    }

    widgetClient.close();
    loader.close();
    return result;
}

function _errorMessage(e) {
    if (typeof e === 'string') return e;
    if (e instanceof Error) return e.message;
    if (e?.message) return e.message;
    if (e?.detailMessage) return e.detailMessage;
    return String(e);
}

function _cleanup(loader, widgetClient) {
    try { widgetClient?.close(); } catch (e) { /* ignore */ }
    try { loader?.close(); } catch (e) { /* ignore */ }
}
