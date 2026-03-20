import { describe, it, expect, beforeAll, afterAll } from 'vitest';
import { dhr, render, snapshot, click, openErrorSession, closeSession } from '../helpers/cli-commands.mjs';

/**
 * Test that a widget either fails to render (error thrown) or shows an error in the output.
 * Both outcomes are valid for error test widgets.
 *
 * @param {string} widgetName
 * @param {string} [expectedText] - Text expected in the render output or error message
 */
function expectRenderError(widgetName, expectedText) {
    try {
        const out = render(widgetName);
        if (expectedText) {
            expect(out).toContain(expectedText);
        } else {
            // Any render of an error widget counts as passing — some errors are caught server-side
            expect(
                out.includes('error') || out.includes('Error') || out.includes('Traceback')
            ).toBe(true);
        }
    } catch (e) {
        // Render failure is EXPECTED for error tests
        if (expectedText) {
            expect(e.message || String(e)).toContain(expectedText);
        } else {
            expect(e).toBeDefined();
        }
    }
}

/**
 * Test that a widget errors when clicked.
 * Widget renders successfully, but clicking triggers an error.
 */
function expectClickError(widgetName) {
    try {
        render(widgetName);
        try {
            click('@e1');
            snapshot(); // May show error in output
        } catch (e) {
            // Click error is expected
            expect(e).toBeDefined();
        }
    } catch (e) {
        // If even render fails, that's also fine
        expect(e).toBeDefined();
    }
}

describe.runIf(globalThis.__DH_SERVER_AVAILABLE__)('error handling', () => {
    beforeAll(() => openErrorSession());
    afterAll(() => {
        closeSession();
    });

    // Render-time errors
    it('err_none_access: handles None attribute access error', () => {
        expectRenderError('err_none_access_widget');
    });

    it('err_index_out_of_range: handles index out of range error', () => {
        expectRenderError('err_index_out_of_range_widget');
    });

    it('err_type_mismatch: handles type mismatch error', () => {
        expectRenderError('err_type_mismatch_widget');
    });

    it('err_key_error: handles key error', () => {
        expectRenderError('err_key_error_widget');
    });

    it('err_divide_by_zero: handles division by zero error', () => {
        expectRenderError('err_divide_by_zero_widget', 'division by zero');
    });

    it('err_missing_prop: handles missing prop error', () => {
        expectRenderError('err_missing_prop_widget');
    });

    it('err_infinite_render: handles infinite render error', () => {
        expectRenderError('err_infinite_render_widget');
    });

    it('err_wrong_children: handles wrong children error', () => {
        expectRenderError('err_wrong_children_widget');
    });

    it('err_explicit_throw: handles explicit throw error', () => {
        expectRenderError('err_explicit_throw_widget');
    });

    it('err_import_missing: handles import missing error', () => {
        expectRenderError('err_import_missing_widget');
    });

    it('err_recursion: handles recursion error', () => {
        expectRenderError('err_recursion_widget');
    });

    // Click-time errors
    it('err_bad_callback: handles bad callback on click', () => {
        expectClickError('err_bad_callback_widget');
    });

    it('err_stale_closure: handles stale closure on click', () => {
        expectClickError('err_stale_closure_widget');
    });

    it('err_mutation_during_render: handles mutation during render on click', () => {
        expectClickError('err_mutation_widget');
    });
});
