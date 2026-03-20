/**
 * Node.js ESM loader hook that handles CSS/SCSS imports by returning empty modules.
 *
 * Usage: node --import ./src/css-loader.mjs your-script.mjs
 * Or:    node --loader ./src/css-loader.mjs your-script.mjs
 *
 * Required because @deephaven/components and @adobe/react-spectrum import .css files
 * which Node.js cannot handle natively.
 */

import { register } from 'node:module';
import { pathToFileURL } from 'node:url';

register('./css-loader-hooks.mjs', import.meta.url);
