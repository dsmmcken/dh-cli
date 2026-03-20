import { describe, it, expect } from 'vitest';
import {
    findAllCallables, findAllObjects, findAllElements,
    findCallableByProp, findCallableByButtonText, findCallableByElement,
    getAtPath, prettyPrintDocument,
} from '../../src/helpers.mjs';
import {
    SIMPLE_TEXT_DOC, BUTTON_DOC, TABLE_DOC, NESTED_DOC,
    CALLABLE_DOC, MULTI_OBJECT_DOC, EMPTY_DOC,
} from '../helpers/mock-documents.mjs';

describe('findAllCallables', () => {
    it('finds callables in a simple doc with one button', () => {
        const result = findAllCallables(BUTTON_DOC);
        expect(result.length).toBe(1);
        expect(result[0].id).toBe('cb0');
    });

    it('finds multiple callables in a nested doc', () => {
        const result = findAllCallables(CALLABLE_DOC);
        expect(result.length).toBe(3);
        const ids = result.map(r => r.id);
        expect(ids).toContain('cb0');
        expect(ids).toContain('cb1');
        expect(ids).toContain('cb2');
    });

    it('returns empty array for doc with no callables', () => {
        const result = findAllCallables(SIMPLE_TEXT_DOC);
        expect(result).toEqual([]);
    });
});

describe('findAllObjects', () => {
    it('finds objects in a doc with tables', () => {
        const result = findAllObjects(MULTI_OBJECT_DOC);
        expect(result.length).toBe(2);
        expect(result[0].id).toBe(0);
        expect(result[1].id).toBe(1);
    });

    it('returns empty array for doc with no objects', () => {
        const result = findAllObjects(SIMPLE_TEXT_DOC);
        expect(result).toEqual([]);
    });
});

describe('findAllElements', () => {
    it('finds all elements in a nested tree', () => {
        const result = findAllElements(NESTED_DOC);
        const names = result.map(r => r.name);
        expect(names).toContain('deephaven.ui.components.Flex');
        expect(names).toContain('deephaven.ui.components.Panel');
        expect(names).toContain('deephaven.ui.components.Button');
        expect(names).toContain('deephaven.ui.components.Text');
        expect(result.length).toBe(4);
    });
});

describe('findCallableByProp', () => {
    it('finds callable by prop name', () => {
        const id = findCallableByProp(CALLABLE_DOC, 'onChange');
        expect(id).toBe('cb2');
    });

    it('returns null when prop not found', () => {
        const id = findCallableByProp(CALLABLE_DOC, 'onSubmit');
        expect(id).toBeNull();
    });
});

describe('findCallableByButtonText', () => {
    it('finds callable by button text', () => {
        const id = findCallableByButtonText(CALLABLE_DOC, 'Increment');
        expect(id).toBe('cb0');
    });

    it('returns null for non-existent button text', () => {
        const id = findCallableByButtonText(CALLABLE_DOC, 'NonExistent');
        expect(id).toBeNull();
    });
});

describe('findCallableByElement', () => {
    it('finds callable by element name', () => {
        const id = findCallableByElement(CALLABLE_DOC, 'deephaven.ui.components.Button');
        // Should find the first button's onPress (cb0)
        expect(id).toBe('cb0');
    });
});

describe('getAtPath', () => {
    it('gets nested value by dot-separated path', () => {
        const value = getAtPath(BUTTON_DOC, 'props.children');
        expect(Array.isArray(value)).toBe(true);
        expect(value.length).toBe(1);
    });

    it('returns undefined for invalid path', () => {
        const value = getAtPath(BUTTON_DOC, 'props.nonexistent.deep');
        expect(value).toBeUndefined();
    });

    it('handles null intermediate values', () => {
        const doc = { a: { b: null } };
        const value = getAtPath(doc, 'a.b.c');
        expect(value).toBeUndefined();
    });
});

describe('prettyPrintDocument', () => {
    it('formats a simple element', () => {
        const output = prettyPrintDocument(SIMPLE_TEXT_DOC);
        expect(output).toContain('<Text>');
        expect(output).toContain('Hello');
        expect(output).toContain('</Text>');
    });

    it('formats a nested tree', () => {
        const output = prettyPrintDocument(NESTED_DOC);
        expect(output).toContain('<Flex>');
        expect(output).toContain('<Panel>');
        expect(output).toContain('<Button');
        expect(output).toContain('Nested Button');
        expect(output).toContain('</Flex>');
    });

    it('handles callables and objects in output', () => {
        const doc = {
            __dhElemName: 'deephaven.ui.components.Button',
            props: {
                children: ['Click'],
                onPress: { __dhCbid: 'cb5' },
            },
        };
        const output = prettyPrintDocument(doc);
        expect(output).toContain('onPress={cb5}');
    });

    it('handles primitive input', () => {
        expect(prettyPrintDocument('hello')).toBe('hello');
        expect(prettyPrintDocument(42)).toBe('42');
        expect(prettyPrintDocument(null)).toBe('null');
    });
});
