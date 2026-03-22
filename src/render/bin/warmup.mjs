#!/usr/bin/env node
/**
 * One-shot warmup script for vm prepare.
 * Loads all render-path modules to populate NODE_COMPILE_CACHE and jsapi-cache.
 * Does NOT open server connections — avoids competing with JVM warmup.
 * Hard-exits after completion — does NOT wait for event loop drain.
 */
import '../src/css-loader.mjs';
const t0 = Date.now();

// Hard kill after 30s no matter what
setTimeout(() => process.exit(0), 30000).unref();

try {
    // Import index.mjs — pulls in all static deps (JsApiLoader, React,
    // WidgetClient, jsdom-globals, TestProviderStack, GoldenLayout, etc.)
    const { JsApiLoader } = await import('../src/index.mjs');

    // Load JSAPI from the DH server (downloads + caches dh-core.js, dh-internal.js)
    const loader = new JsApiLoader('http://127.0.0.1:10000');
    await loader.load();
    loader.close();

    const t1 = Date.now();
    process.stderr.write(`[warmup] modules + JSAPI cached in ${t1 - t0}ms\n`);

    // Import heavy dynamic modules that createTestClient loads at runtime.
    // This populates V8 compile cache without opening WebSocket connections
    // or requiring jsdom globals (which would compete with JVM warmup).
    await Promise.all([
        import('react-dom/client').catch(() => {}),
        import('@deephaven/js-plugin-ui').catch(() => {}),
    ]);

    process.stderr.write(`[warmup] dynamic modules cached in ${Date.now() - t0}ms\n`);
} catch (e) {
    process.stderr.write(`[warmup] error: ${e.message}\n`);
}
process.exit(0);
