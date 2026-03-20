import { describe, it, expect } from 'vitest';
import {
    assertRowCount, assertMinRowCount, assertColumns, assertColumnEquals,
    assertColumnContains, assertColumnAll, assertTableHas, assertColumnSorted,
    assertColumnUnique, assertColumnInRange,
} from '../../src/tableAssertions.mjs';
import {
    SAMPLE_COLUMNS, SAMPLE_ROWS, SORTED_ROWS_ASC, SORTED_ROWS_DESC,
    DUPLICATE_ROWS, EMPTY_ROWS,
} from '../helpers/table-data.mjs';

describe('assertRowCount', () => {
    it('passes with correct count', () => {
        expect(() => assertRowCount(SAMPLE_ROWS, 4)).not.toThrow();
    });

    it('throws with wrong count', () => {
        expect(() => assertRowCount(SAMPLE_ROWS, 5)).toThrow(/Expected 5 rows/);
    });

    it('works with empty array', () => {
        expect(() => assertRowCount(EMPTY_ROWS, 0)).not.toThrow();
    });
});

describe('assertMinRowCount', () => {
    it('passes with enough rows', () => {
        expect(() => assertMinRowCount(SAMPLE_ROWS, 3)).not.toThrow();
    });

    it('throws with too few rows', () => {
        expect(() => assertMinRowCount(SAMPLE_ROWS, 10)).toThrow(/Expected at least 10 rows/);
    });
});

describe('assertColumns', () => {
    it('passes when all columns present', () => {
        expect(() => assertColumns(SAMPLE_COLUMNS, ['Name', 'Age'])).not.toThrow();
    });

    it('throws when column missing', () => {
        expect(() => assertColumns(SAMPLE_COLUMNS, ['Name', 'Missing'])).toThrow(/Column "Missing" not found/);
    });
});

describe('assertColumnEquals', () => {
    it('passes with matching values', () => {
        expect(() =>
            assertColumnEquals(SAMPLE_ROWS, 'Name', ['Alice', 'Bob', 'Charlie', 'Diana'])
        ).not.toThrow();
    });

    it('throws on mismatch', () => {
        expect(() =>
            assertColumnEquals(SAMPLE_ROWS, 'Name', ['Alice', 'Bob', 'Charlie', 'Eve'])
        ).toThrow(/Column "Name" row 3/);
    });

    it('throws on length mismatch', () => {
        expect(() =>
            assertColumnEquals(SAMPLE_ROWS, 'Name', ['Alice', 'Bob'])
        ).toThrow(/expected 2 values, got 4/);
    });
});

describe('assertColumnContains', () => {
    it('passes when value present', () => {
        expect(() => assertColumnContains(SAMPLE_ROWS, 'Name', 'Bob')).not.toThrow();
    });

    it('throws when value absent', () => {
        expect(() => assertColumnContains(SAMPLE_ROWS, 'Name', 'Eve')).toThrow(/does not contain/);
    });
});

describe('assertColumnAll', () => {
    it('passes when all match predicate', () => {
        expect(() =>
            assertColumnAll(SAMPLE_ROWS, 'Age', (v) => v > 20)
        ).not.toThrow();
    });

    it('throws when one fails predicate', () => {
        expect(() =>
            assertColumnAll(SAMPLE_ROWS, 'Age', (v) => v > 30)
        ).toThrow(/failed predicate/);
    });
});

describe('assertTableHas', () => {
    it('passes with matching partial row', () => {
        expect(() =>
            assertTableHas(SAMPLE_ROWS, { Name: 'Bob', Age: 25 })
        ).not.toThrow();
    });

    it('throws when no match', () => {
        expect(() =>
            assertTableHas(SAMPLE_ROWS, { Name: 'Bob', Age: 99 })
        ).toThrow(/No row matching/);
    });
});

describe('assertColumnSorted', () => {
    it('passes for asc sorted', () => {
        expect(() => assertColumnSorted(SORTED_ROWS_ASC, 'Age', 'asc')).not.toThrow();
    });

    it('passes for desc sorted', () => {
        expect(() => assertColumnSorted(SORTED_ROWS_DESC, 'Age', 'desc')).not.toThrow();
    });

    it('throws for unsorted', () => {
        expect(() => assertColumnSorted(SAMPLE_ROWS, 'Age', 'asc')).toThrow(/not sorted asc/);
    });
});

describe('assertColumnUnique', () => {
    it('passes for unique values', () => {
        expect(() => assertColumnUnique(SAMPLE_ROWS, 'Name')).not.toThrow();
    });

    it('throws for duplicates', () => {
        expect(() => assertColumnUnique(DUPLICATE_ROWS, 'Name')).toThrow(/duplicate value/);
    });
});

describe('assertColumnInRange', () => {
    it('passes when all in range', () => {
        expect(() => assertColumnInRange(SAMPLE_ROWS, 'Age', 20, 40)).not.toThrow();
    });

    it('throws when out of range', () => {
        expect(() => assertColumnInRange(SAMPLE_ROWS, 'Age', 26, 34)).toThrow(/not in range/);
    });

    it('throws for non-numbers', () => {
        expect(() => assertColumnInRange(SAMPLE_ROWS, 'Name', 0, 100)).toThrow(/not in range/);
    });
});
