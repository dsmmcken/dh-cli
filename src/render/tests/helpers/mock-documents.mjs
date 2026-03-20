/**
 * Standard document tree fixtures for unit tests.
 */

export const SIMPLE_TEXT_DOC = {
    __dhElemName: 'deephaven.ui.components.Text',
    props: { children: ['Hello'] },
};

export const BUTTON_DOC = {
    __dhElemName: 'deephaven.ui.components.Panel',
    props: {
        children: [{
            __dhElemName: 'deephaven.ui.components.Button',
            props: {
                children: ['Click Me'],
                variant: 'primary',
                onPress: { __dhCbid: 'cb0' },
            },
        }],
    },
};

export const TABLE_DOC = {
    __dhElemName: 'deephaven.ui.components.Panel',
    props: {
        children: [{
            __dhElemName: 'deephaven.ui.elements.UITable',
            props: {
                table: { __dhObid: 0 },
            },
        }],
    },
};

export const NESTED_DOC = {
    __dhElemName: 'deephaven.ui.components.Flex',
    props: {
        children: [
            {
                __dhElemName: 'deephaven.ui.components.Panel',
                props: {
                    children: [
                        {
                            __dhElemName: 'deephaven.ui.components.Button',
                            props: {
                                children: ['Nested Button'],
                                onPress: { __dhCbid: 'cb1' },
                            },
                        },
                        {
                            __dhElemName: 'deephaven.ui.components.Text',
                            props: { children: ['Nested Text'] },
                        },
                    ],
                },
            },
        ],
    },
};

export const CALLABLE_DOC = {
    __dhElemName: 'deephaven.ui.components.Panel',
    props: {
        children: [
            {
                __dhElemName: 'deephaven.ui.components.Text',
                props: { children: ['Counter: 0'] },
            },
            {
                __dhElemName: 'deephaven.ui.components.Button',
                props: {
                    children: ['Increment'],
                    onPress: { __dhCbid: 'cb0' },
                },
            },
            {
                __dhElemName: 'deephaven.ui.components.Button',
                props: {
                    children: ['Decrement'],
                    onPress: { __dhCbid: 'cb1' },
                },
            },
            {
                __dhElemName: 'deephaven.ui.components.TextField',
                props: {
                    label: 'Name',
                    value: '',
                    onChange: { __dhCbid: 'cb2' },
                },
            },
        ],
    },
};

export const MULTI_OBJECT_DOC = {
    __dhElemName: 'deephaven.ui.components.Panel',
    props: {
        children: [
            {
                __dhElemName: 'deephaven.ui.elements.UITable',
                props: { table: { __dhObid: 0 } },
            },
            {
                __dhElemName: 'deephaven.ui.elements.UITable',
                props: { table: { __dhObid: 1 } },
            },
        ],
    },
};

export const HTML_DOC = {
    __dhElemName: 'deephaven.ui.html.div',
    props: {
        children: [
            {
                __dhElemName: 'deephaven.ui.html.h1',
                props: { children: ['Title'] },
            },
            {
                __dhElemName: 'deephaven.ui.html.p',
                props: { children: ['Paragraph text'] },
            },
        ],
    },
};

export const EMPTY_DOC = {
    __dhElemName: 'deephaven.ui.components.Panel',
    props: { children: [] },
};

export const TABLE_WITH_CONFIG_DOC = {
    __dhElemName: 'deephaven.ui.components.Panel',
    props: {
        children: [{
            __dhElemName: 'deephaven.ui.elements.UITable',
            props: {
                table: { __dhObid: 0 },
                reverse: true,
                density: 'compact',
                frontColumns: ['Timestamp', 'Species'],
                onRowDoublePress: { __dhCbid: 'cb0' },
            },
        }],
    },
};

/**
 * Create a mock exported table object with reexport/fetch/viewport API.
 * @param {{ columns: Array<{name: string, type: string}>, rows: Array<Object> }} opts
 */
export function createMockExportedTable({ columns, rows }) {
    return {
        type: 'Table',
        reexport: () => Promise.resolve({
            fetch: () => Promise.resolve({
                columns: columns.map(c => ({ name: c.name, type: c.type })),
                size: rows.length,
                setViewport: () => {},
                getViewportData: () => Promise.resolve({
                    rows: rows.map(r => ({
                        get: (col) => r[col.name],
                    })),
                }),
            }),
        }),
    };
}

/** Document containing a figure exported object as a child. */
export const FIGURE_DOC = {
    __dhElemName: 'deephaven.ui.components.Panel',
    props: {
        title: 'Scatter Panel',
        children: { __dhObid: 0 },
    },
};

/** Document with mixed figures and tables. */
export const FIGURE_AND_TABLE_DOC = {
    __dhElemName: 'deephaven.ui.components.Panel',
    props: {
        children: [
            { __dhObid: 0 },
            { __dhObid: 1 },
            {
                __dhElemName: 'deephaven.ui.elements.UITable',
                props: { table: { __dhObid: 2 } },
            },
        ],
    },
};

/**
 * Create a mock exported figure object matching the real
 * reexport → fetch → getDataAsString chain.
 *
 * @param {object} payload - The NEW_FIGURE payload JSON
 * @returns {object} Mock exported figure object
 */
export function createMockExportedFigure(payload) {
    return {
        type: 'deephaven.plot.express.DeephavenFigure',
        reexport: () => Promise.resolve({
            fetch: () => Promise.resolve({
                getDataAsString: () => JSON.stringify(payload),
                exportedObjects: (payload.new_references || []).map(() => ({
                    type: 'Table',
                    reexport: () => Promise.resolve({ fetch: () => Promise.resolve({}) }),
                })),
                close: () => {},
            }),
        }),
    };
}

/**
 * Create a mock JSON-RPC documentPatched response for a given document.
 */
export function createDocumentPatchedResponse(doc) {
    return JSON.stringify({
        jsonrpc: '2.0',
        method: 'documentPatched',
        params: [[{ op: 'replace', path: '', value: doc }], null],
    });
}
