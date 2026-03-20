/**
 * Table data assertion helpers for testing Deephaven tables.
 *
 * These work with the row-object arrays returned by RenderResult.fetchTableData()
 * or WidgetClient.getTableData().
 */

/**
 * Assert that a table has the expected number of rows.
 * @param {Array<object>} rows - Table row data
 * @param {number} expected - Expected row count
 * @param {string} [message] - Optional message
 * @throws {Error} If assertion fails
 */
export function assertRowCount(rows, expected, message) {
    if (rows.length !== expected) {
        throw new AssertionError(
            message || `Expected ${expected} rows, got ${rows.length}`
        );
    }
}

/**
 * Assert that a table has at least the given number of rows.
 * @param {Array<object>} rows - Table row data
 * @param {number} minRows - Minimum row count
 * @param {string} [message] - Optional message
 */
export function assertMinRowCount(rows, minRows, message) {
    if (rows.length < minRows) {
        throw new AssertionError(
            message || `Expected at least ${minRows} rows, got ${rows.length}`
        );
    }
}

/**
 * Assert that a table has specific column names.
 * @param {Array<{name: string, type: string}>} columns - Column definitions
 * @param {string[]} expectedNames - Expected column names
 * @param {string} [message] - Optional message
 */
export function assertColumns(columns, expectedNames, message) {
    const actualNames = columns.map(c => c.name);
    for (const name of expectedNames) {
        if (!actualNames.includes(name)) {
            throw new AssertionError(
                message || `Column "${name}" not found. Available: ${actualNames.join(', ')}`
            );
        }
    }
}

/**
 * Assert that all values in a column match the expected values.
 * @param {Array<object>} rows - Table row data
 * @param {string} columnName - Column to check
 * @param {Array} expectedValues - Expected values in order
 * @param {string} [message] - Optional message
 */
export function assertColumnEquals(rows, columnName, expectedValues, message) {
    if (rows.length !== expectedValues.length) {
        throw new AssertionError(
            message || `Column "${columnName}": expected ${expectedValues.length} values, got ${rows.length} rows`
        );
    }
    for (let i = 0; i < rows.length; i++) {
        const actual = rows[i][columnName];
        const expected = expectedValues[i];
        if (actual !== expected) {
            throw new AssertionError(
                message || `Column "${columnName}" row ${i}: expected ${JSON.stringify(expected)}, got ${JSON.stringify(actual)}`
            );
        }
    }
}

/**
 * Assert that a column contains a specific value in at least one row.
 * @param {Array<object>} rows - Table row data
 * @param {string} columnName - Column to check
 * @param {*} value - Value to find
 * @param {string} [message] - Optional message
 */
export function assertColumnContains(rows, columnName, value, message) {
    const found = rows.some(row => row[columnName] === value);
    if (!found) {
        const actual = rows.map(r => r[columnName]);
        throw new AssertionError(
            message || `Column "${columnName}" does not contain ${JSON.stringify(value)}. Values: ${JSON.stringify(actual.slice(0, 10))}`
        );
    }
}

/**
 * Assert that every value in a column satisfies a predicate.
 * @param {Array<object>} rows - Table row data
 * @param {string} columnName - Column to check
 * @param {function} predicate - Function that returns true for valid values
 * @param {string} [message] - Optional message
 */
export function assertColumnAll(rows, columnName, predicate, message) {
    for (let i = 0; i < rows.length; i++) {
        const value = rows[i][columnName];
        if (!predicate(value, i)) {
            throw new AssertionError(
                message || `Column "${columnName}" row ${i}: value ${JSON.stringify(value)} failed predicate`
            );
        }
    }
}

/**
 * Assert that a table contains a row matching the given partial object.
 * @param {Array<object>} rows - Table row data
 * @param {object} partial - Partial row to match (e.g., { Species: 'setosa', SepalLength: 5.1 })
 * @param {string} [message] - Optional message
 */
export function assertTableHas(rows, partial, message) {
    const found = rows.some(row => {
        for (const [key, value] of Object.entries(partial)) {
            if (row[key] !== value) return false;
        }
        return true;
    });
    if (!found) {
        throw new AssertionError(
            message || `No row matching ${JSON.stringify(partial)} found in ${rows.length} rows`
        );
    }
}

/**
 * Assert that a column's values are sorted.
 * @param {Array<object>} rows - Table row data
 * @param {string} columnName - Column to check
 * @param {'asc'|'desc'} [order='asc'] - Sort order
 * @param {string} [message] - Optional message
 */
export function assertColumnSorted(rows, columnName, order = 'asc', message) {
    for (let i = 1; i < rows.length; i++) {
        const prev = rows[i - 1][columnName];
        const curr = rows[i][columnName];
        const ok = order === 'asc' ? curr >= prev : curr <= prev;
        if (!ok) {
            throw new AssertionError(
                message || `Column "${columnName}" not sorted ${order} at row ${i}: ${JSON.stringify(prev)} → ${JSON.stringify(curr)}`
            );
        }
    }
}

/**
 * Assert that all values in a column are unique.
 * @param {Array<object>} rows - Table row data
 * @param {string} columnName - Column to check
 * @param {string} [message] - Optional message
 */
export function assertColumnUnique(rows, columnName, message) {
    const seen = new Set();
    for (let i = 0; i < rows.length; i++) {
        const value = rows[i][columnName];
        if (seen.has(value)) {
            throw new AssertionError(
                message || `Column "${columnName}" has duplicate value ${JSON.stringify(value)} at row ${i}`
            );
        }
        seen.add(value);
    }
}

/**
 * Assert that a numeric column's values are within a range.
 * @param {Array<object>} rows - Table row data
 * @param {string} columnName - Column to check
 * @param {number} min - Minimum value (inclusive)
 * @param {number} max - Maximum value (inclusive)
 * @param {string} [message] - Optional message
 */
export function assertColumnInRange(rows, columnName, min, max, message) {
    for (let i = 0; i < rows.length; i++) {
        const value = rows[i][columnName];
        if (typeof value !== 'number' || value < min || value > max) {
            throw new AssertionError(
                message || `Column "${columnName}" row ${i}: value ${JSON.stringify(value)} not in range [${min}, ${max}]`
            );
        }
    }
}

class AssertionError extends Error {
    constructor(message) {
        super(message);
        this.name = 'AssertionError';
    }
}
