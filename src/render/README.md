# dh-render

Headless rendering engine for Deephaven UI widgets. Renders components with React 18 in jsdom — no browser needed.

This is the `render/` subdirectory of [dh-cli](https://github.com/dsmmcken/dh-cli). It powers the `dh render` command.

## Usage via dh-cli

```bash
# Render and snapshot a widget
dh render test_button.py

# Click, then see the result
dh render test_button.py click "Primary"

# Multiple actions
dh render test_form.py fill "Name" "Alice" click "Submit"

# Diagnose
dh render diagnose test_button.py

# Against a running server
dh render test_button.py --url http://localhost:10000 snapshot
```

## Standalone Usage

For development and testing without the Go CLI:

```bash
cd render
npm install

# Interactive CLI (daemon-based)
node bin/dh-render.mjs open http://localhost:10000
node bin/dh-render.mjs render button_widget
node bin/dh-render.mjs click "Primary"
node bin/dh-render.mjs snapshot
node bin/dh-render.mjs close

# One-shot diagnosis
node bin/dh-diagnose.mjs http://localhost:10000 button_widget

# One-shot with action pipeline (used by Go integration)
node --import ./src/css-loader.mjs bin/oneshot.mjs --url http://localhost:10000 --widget button_widget snapshot
```

## Programmatic API

```js
import { renderWidget, createTestClient } from './src/index.mjs';

// Quick one-shot
const result = await renderWidget('http://localhost:10000', 'my_widget');
console.log(result.html);

// Reusable client
const client = await createTestClient('http://localhost:10000');
const result = await client.render('my_widget');
result.querySelector('button').click();
await result.flush();
client.close();
```

## Testing

```bash
npm test                      # Unit + CLI tests (no server)
npm run test:snapshots        # Snapshot tests (auto-starts servers)
npm run test:integration      # Integration tests
npm run test:all              # Everything
```

## How It Works

1. Downloads Deephaven JSAPI from the server via `@deephaven/jsapi-nodejs`
2. Connects and authenticates via `dh.CoreClient`
3. Renders widgets using the real `WidgetHandler` pipeline from `@deephaven/js-plugin-ui`
4. Full GoldenLayout portal support and real Spectrum components
5. Accessibility tree snapshots with `@eN` refs for targeting interactive elements
6. DOM interactions (click, fill, select) fire React synthetic events routed to server-side callables
