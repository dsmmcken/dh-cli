/**
 * JsApiLoader - Loads the Deephaven JSAPI for use in Node.js.
 *
 * Uses @deephaven/jsapi-nodejs to download and load dh-core.js and dh-internal.js
 * natively as ESM modules (no jsdom needed for the JSAPI itself).
 * Creates a separate jsdom environment for React rendering.
 */

// Polyfill navigator for Node.js < 21 (e.g. Node 20 in the Firecracker VM).
// The Deephaven JSAPI and its dependencies reference `navigator` at load time.
// Node.js 21+ provides a built-in navigator global; older versions do not.
if (typeof globalThis.navigator === 'undefined') {
    const os = await import('node:os');
    globalThis.navigator = {
        userAgent: `Node.js/${process.versions.node}`,
        platform: process.platform,
        language: 'en',
        languages: ['en'],
        onLine: true,
        hardwareConcurrency: os.cpus().length,
    };
}

import { JSDOM } from 'jsdom';
import { loadDhModules } from '@deephaven/jsapi-nodejs';
import { existsSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';

// Inside the Firecracker VM, use a persistent cache directory on ext4 so
// downloaded JSAPI files survive snapshot restore (tmpfs is re-mounted fresh).
const VM_JSAPI_CACHE = '/opt/render/jsapi-cache';
const DEFAULT_STORAGE_DIR = existsSync(VM_JSAPI_CACHE)
    ? VM_JSAPI_CACHE
    : join(tmpdir(), 'dh-render-jsapi');

export class JsApiLoader {
    /**
     * @param {string} serverUrl - The Deephaven server URL (e.g., 'http://localhost:10000')
     */
    constructor(serverUrl) {
        this.serverUrl = serverUrl.replace(/\/$/, '');
        this.dom = null;
        this.dh = null;
    }

    /**
     * Load only the JSAPI modules (no jsdom creation).
     * Can be called concurrently with other setup that doesn't need JSAPI.
     * @returns {Promise<object>} The dh JSAPI object
     */
    async loadJSAPI() {
        const storageDir = DEFAULT_STORAGE_DIR;
        this.dh = await loadDhModules({
            serverUrl: new URL(this.serverUrl),
            storageDir,
            targetModuleType: 'esm',
        });

        if (!this.dh || !this.dh.CoreClient) {
            throw new Error('Failed to load Deephaven JSAPI - dh.CoreClient not found');
        }

        return this.dh;
    }

    /**
     * Create the jsdom environment for React rendering.
     * Can be called independently of loadJSAPI().
     * @returns {JSDOM}
     */
    createDom() {
        this.dom = new JSDOM('<!DOCTYPE html><html><body></body></html>', {
            url: this.serverUrl,
            pretendToBeVisual: true,
        });
        return this.dom;
    }

    /**
     * Load the JSAPI from the server using @deephaven/jsapi-nodejs,
     * and create a jsdom environment for React rendering.
     * @returns {Promise<{dh: object, window: object, dom: JSDOM}>}
     */
    async load() {
        await this.loadJSAPI();
        this.createDom();
        return { dh: this.dh, window: this.dom.window, dom: this.dom };
    }

    /**
     * Clean up the jsdom environment.
     *
     * Note: we intentionally do NOT call dom.window.close() because
     * MathJax (bundled in @deephaven/js-plugin-ui) captures the first
     * jsdom window's DOMParser at module load time. Closing that window
     * invalidates the DOMParser, causing "Can't find handler for document"
     * errors when markdown components render in subsequent tests.
     */
    close() {
        if (this.dom) {
            this.dom = null;
            this.dh = null;
        }
    }
}
