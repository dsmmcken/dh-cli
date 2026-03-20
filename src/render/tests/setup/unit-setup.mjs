/**
 * Setup file for unit tests.
 * Installs jsdom globals so React and DH components can be imported.
 */
import { JSDOM } from 'jsdom';
import { installJsdomGlobals } from '../../src/jsdom-globals.mjs';

const dom = new JSDOM('<!DOCTYPE html><html><body><div id="root"></div></body></html>', {
    pretendToBeVisual: true,
    url: 'http://localhost',
});

const globals = installJsdomGlobals(dom);

// React act() requires this flag
globalThis.IS_REACT_ACT_ENVIRONMENT = true;

// Expose dom for tests that need direct DOM access
globalThis.__TEST_DOM__ = dom;

// Cleanup after all tests
import { afterAll } from 'vitest';
afterAll(() => {
    globals.restore();
});
