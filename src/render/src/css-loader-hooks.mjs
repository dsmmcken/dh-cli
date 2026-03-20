/**
 * ESM loader hooks for running Deephaven packages in Node.js:
 * - CSS/SCSS/asset imports → empty modules
 * - redux-thunk CJS/ESM interop fix
 * - lodash → lodash-es redirect
 *
 * Used by css-loader.mjs via node:module register().
 *
 * Note: heavy packages (@deephaven/console, iris-grid, chart, etc.) are stubbed
 * in the js-plugin-ui CJS bundle by patch-bundle.mjs, not here.
 */

const STYLE_EXTENSIONS = ['.css', '.scss', '.sass', '.less'];
const ASSET_EXTENSIONS = ['.svg', '.png', '.jpg', '.gif', '.woff', '.woff2', '.ttf', '.eot'];
const ALL_EXTENSIONS = [...STYLE_EXTENSIONS, ...ASSET_EXTENSIONS];

/**
 * Resolve hook.
 */
export function resolve(specifier, context, nextResolve) {
    // Style/asset extensions → short-circuit
    if (ALL_EXTENSIONS.some(ext => specifier.endsWith(ext))) {
        return {
            shortCircuit: true,
            url: new URL(specifier, context.parentURL || 'file:///').href,
        };
    }

    // Handle ?inline / ?raw query params (vite-style CSS imports)
    if (specifier.includes('?inline') || specifier.includes('?raw')) {
        const cleanSpecifier = specifier.split('?')[0];
        if (ALL_EXTENSIONS.some(ext => cleanSpecifier.endsWith(ext))) {
            return {
                shortCircuit: true,
                url: new URL(specifier, context.parentURL || 'file:///').href,
            };
        }
    }

    // Redirect bare 'redux-thunk' to its ESM entry to avoid double-wrapped default
    if (specifier === 'redux-thunk') {
        return nextResolve('redux-thunk/es/index.js', context);
    }

    // Redirect lodash → lodash-es for ESM named export compatibility
    if (specifier === 'lodash') {
        return nextResolve('lodash-es', context);
    }

    return nextResolve(specifier, context);
}

/**
 * Load hook: return empty module for CSS/SCSS/assets.
 */
export function load(url, context, nextLoad) {
    const cleanUrl = url.split('?')[0];

    // CSS/SCSS/assets → empty module
    if (ALL_EXTENSIONS.some(ext => cleanUrl.endsWith(ext))) {
        return {
            shortCircuit: true,
            format: 'module',
            source: 'export default "";',
        };
    }

    return nextLoad(url, context);
}
