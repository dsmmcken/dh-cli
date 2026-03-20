#!/usr/bin/env node
/**
 * One-shot warmup script for vm prepare.
 * Loads all modules + JSAPI to populate NODE_COMPILE_CACHE and jsapi-cache.
 * Hard-exits after completion — does NOT wait for event loop drain.
 */
import '../src/css-loader.mjs';
const t0 = Date.now();

// Hard kill after 30s no matter what
setTimeout(() => process.exit(0), 30000).unref();

try {
    const { JsApiLoader } = await import('../src/index.mjs');
    const loader = new JsApiLoader('http://127.0.0.1:10000');
    await loader.load();
    loader.close();
    process.stderr.write(`[warmup] modules + JSAPI cached in ${Date.now() - t0}ms\n`);
} catch (e) {
    process.stderr.write(`[warmup] error: ${e.message}\n`);
}
process.exit(0);
