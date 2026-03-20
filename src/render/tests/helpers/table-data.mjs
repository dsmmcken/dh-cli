/**
 * Mock table data fixtures for tableAssertions tests.
 */

export const SAMPLE_COLUMNS = [
    { name: 'Name', type: 'String' },
    { name: 'Age', type: 'int' },
    { name: 'Score', type: 'double' },
];

export const SAMPLE_ROWS = [
    { Name: 'Alice', Age: 30, Score: 95.5 },
    { Name: 'Bob', Age: 25, Score: 88.0 },
    { Name: 'Charlie', Age: 35, Score: 72.3 },
    { Name: 'Diana', Age: 28, Score: 91.7 },
];

export const SORTED_ROWS_ASC = [
    { Name: 'Alice', Age: 25, Score: 72.3 },
    { Name: 'Bob', Age: 28, Score: 88.0 },
    { Name: 'Charlie', Age: 30, Score: 91.7 },
    { Name: 'Diana', Age: 35, Score: 95.5 },
];

export const SORTED_ROWS_DESC = [
    { Name: 'Diana', Age: 35, Score: 95.5 },
    { Name: 'Charlie', Age: 30, Score: 91.7 },
    { Name: 'Bob', Age: 28, Score: 88.0 },
    { Name: 'Alice', Age: 25, Score: 72.3 },
];

export const DUPLICATE_ROWS = [
    { Name: 'Alice', Age: 30, Score: 95.5 },
    { Name: 'Bob', Age: 25, Score: 88.0 },
    { Name: 'Alice', Age: 30, Score: 95.5 },
];

export const EMPTY_ROWS = [];
