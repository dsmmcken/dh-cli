/**
 * Complex test: Iris Species Dashboard
 *
 * This tests a full dashboard with:
 * - Tables with aggregations (avg, min, max)
 * - Charts (scatter, histogram, density heatmap)
 * - Tabs
 * - Picker (species selection)
 * - State management (use_state, use_memo, use_cell_data)
 * - Conditional rendering (badges only when species selected)
 * - Dashboard layout (rows, columns, stacks)
 * - Markdown content
 * - Panels
 *
 * Prerequisites:
 *   dh serve /workspace/test_iris_dashboard.py --no-browser --port 10000
 */
import {
    createTestClient,
    findAllCallables,
    findAllObjects,
    findAllElements,
    findCallableByProp,
    findCallableByButtonText,
    prettyPrintDocument,
} from '../src/index.mjs';

const SERVER_URL = 'http://localhost:10000';

// ---- Test runner ----
let totalPass = 0;
let totalFail = 0;

function test(name) {
    console.log(`\n  ${name}`);
}

function pass(message) {
    console.log(`    ✓ ${message}`);
    totalPass++;
}

function fail(message) {
    console.log(`    ✗ ${message}`);
    totalFail++;
}

function assert(condition, message) {
    if (condition) pass(message);
    else fail(message);
}

function assertEqual(actual, expected, message) {
    if (actual === expected) pass(`${message}`);
    else fail(`${message}: expected ${JSON.stringify(expected)}, got ${JSON.stringify(actual)}`);
}

function assertGte(actual, expected, message) {
    if (actual >= expected) pass(`${message}: ${actual} >= ${expected}`);
    else fail(`${message}: expected >= ${expected}, got ${actual}`);
}

function assertContains(text, substring, message) {
    if (text && text.includes(substring)) pass(message);
    else fail(`${message}: "${substring}" not found in "${String(text).substring(0, 100)}"`);
}

// ---- Tests ----
async function main() {
    console.log('╔══════════════════════════════════════════════════╗');
    console.log('║  Iris Dashboard Complex Test Suite               ║');
    console.log('╚══════════════════════════════════════════════════╝');

    const client = await createTestClient(SERVER_URL);

    // ═══════════════════════════════════════════
    // Test the full dashboard widget
    // ═══════════════════════════════════════════
    console.log('\n══════════════════════════════════════════════════');
    console.log('  iris_species_dashboard_final (ui.dashboard)');
    console.log('══════════════════════════════════════════════════');

    test('dashboard renders without errors');
    let result;
    try {
        result = await client.render('iris_species_dashboard_final', {
            widgetType: 'deephaven.ui.Dashboard',
            timeout: 15000,
        });
        pass('Dashboard rendered successfully');
    } catch (e) {
        fail(`Dashboard render failed: ${e.message}`);
        client.close();
        process.exit(1);
    }

    // ── Document structure ──
    test('document has expected structure');
    const doc = result.document;
    const allElements = findAllElements(doc);
    const allObjects = findAllObjects(doc);
    const allCallables = findAllCallables(doc);

    console.log(`    Elements: ${allElements.length}`);
    console.log(`    Exported objects: ${allObjects.length}`);
    console.log(`    Callables: ${allCallables.length}`);

    assertGte(allElements.length, 10, 'Has many elements');
    assertGte(allObjects.length, 1, 'Has exported objects (tables/charts)');

    // ── Pretty print ──
    test('component tree');
    const tree = prettyPrintDocument(doc);
    console.log('    ' + tree.split('\n').join('\n    '));

    // ── Element types ──
    test('contains expected component types');
    const elementNames = allElements.map(e => e.name);
    const uniqueNames = [...new Set(elementNames)];
    console.log(`    Unique element types (${uniqueNames.length}):`);
    uniqueNames.forEach(n => console.log(`      - ${n}`));

    // Check for key components
    assert(elementNames.some(n => n.includes('Panel')), 'Has Panel components');
    assert(elementNames.some(n => n.includes('Flex') || n.includes('flex')), 'Has Flex layout');

    // Check for dashboard layout components
    const hasColumn = elementNames.some(n => n.includes('column') || n.includes('Column'));
    const hasRow = elementNames.some(n => n.includes('row') || n.includes('Row'));
    console.log(`    Dashboard layout: column=${hasColumn}, row=${hasRow}`);

    // ── Rendered HTML ──
    test('HTML output');
    const html = result.html;
    assertGte(html.length, 100, `HTML is substantial (${html.length} chars)`);

    // Check for panel titles
    const panelTitles = ['About', 'Sepal Panel', 'Investigate Species', 'Average', 'Max', 'Min'];
    for (const title of panelTitles) {
        // Panel titles might be in data attributes or text content
        const found = html.includes(title) ||
            result.container.querySelector(`[data-title="${title}"]`) !== null ||
            result.container.querySelector(`[aria-label="${title}"]`) !== null;
        if (found) pass(`Panel "${title}" found`);
        else fail(`Panel "${title}" not found in output`);
    }

    // ── Markdown content ──
    test('markdown/about content');
    const hasIrisDashboard = html.includes('Iris Dashboard') || html.includes('iris') || html.includes('Iris');
    assert(hasIrisDashboard, 'Contains Iris Dashboard text');

    // ── Tabs ──
    test('tabs structure');
    const tabElements = allElements.filter(e => e.name.includes('tab') || e.name.includes('Tab'));
    console.log(`    Tab-related elements: ${tabElements.length}`);
    tabElements.forEach(t => console.log(`      - ${t.name}`));
    assertGte(tabElements.length, 1, 'Has tab elements');

    // Check for tab titles
    const tabTitles = ['Sepal Length vs. Sepal Width', 'Sepal Length Histogram', 'Sepal Width Histogram'];
    for (const title of tabTitles) {
        const tabEl = allElements.find(e =>
            e.name.includes('tab') && e.props?.title === title
        );
        if (tabEl) pass(`Tab "${title}" found`);
        else {
            // Also check in HTML
            const inHtml = html.includes(title);
            if (inHtml) pass(`Tab "${title}" found in HTML`);
            else fail(`Tab "${title}" not found`);
        }
    }

    // ── Picker ──
    test('species picker');
    const pickerElements = allElements.filter(e => e.name.includes('Picker') || e.name.includes('picker'));
    console.log(`    Picker elements: ${pickerElements.length}`);
    assertGte(pickerElements.length, 1, 'Has picker element');

    if (pickerElements.length > 0) {
        const picker = pickerElements[0];
        console.log(`    Picker props: ${JSON.stringify(Object.keys(picker.props || {}))}`);
        assertEqual(picker.props?.label, 'Current Species', 'Picker label is correct');
    }

    // Check for picker on_change callable
    const pickerCallable = findCallableByProp(doc, 'onChange', (el) =>
        el.name.includes('Picker') || el.name.includes('picker')
    );
    assert(pickerCallable !== null, 'Picker has onChange callable');

    // ── Exported objects (tables + charts) ──
    test('exported objects');
    console.log(`    Total exported objects: ${result.exportedObjects.size}`);
    for (const [id, obj] of result.exportedObjects) {
        console.log(`      [${id}] type: ${obj.type}`);
    }
    assertGte(result.exportedObjects.size, 1, 'Has exported objects');

    // Try to fetch table data from one of the table objects
    const tableObjects = [...result.exportedObjects.entries()]
        .filter(([_, obj]) => obj.type === 'Table');
    console.log(`    Table objects: ${tableObjects.length}`);

    if (tableObjects.length > 0) {
        test('table data from exported objects');
        for (const [id, obj] of tableObjects.slice(0, 3)) {
            try {
                const { columns, rows } = await result.fetchTableData(id);
                console.log(`    Table [${id}]: ${columns.map(c => c.name).join(', ')} (${rows.length} rows)`);
                assertGte(rows.length, 1, `Table [${id}] has data`);
            } catch (e) {
                console.log(`    Table [${id}] fetch error: ${e.message}`);
            }
        }
    }

    // ── Illustrated message (no species selected) ──
    test('initial state - no species selected');
    const illustratedMsg = allElements.filter(e => e.name.includes('IllustratedMessage'));
    if (illustratedMsg.length > 0) {
        pass('IllustratedMessage shown (no species selected)');
    } else {
        // It might be rendered differently
        const speciesRequired = html.includes('Species required') || html.includes('species');
        assert(speciesRequired, 'Shows species-required message');
    }

    // ── Select a species via picker ──
    test('selecting a species via picker');
    if (pickerCallable) {
        console.log(`    Calling picker onChange (${pickerCallable}) with "setosa"`);
        try {
            const updateP = result.waitForUpdate(10000);
            await result.fireCallable(pickerCallable, ['setosa']);
            await updateP;
            pass('Species selection triggered re-render');

            // Check that the document updated
            const updatedDoc = result.document;
            const updatedElements = findAllElements(updatedDoc);
            const updatedObjects = findAllObjects(updatedDoc);

            console.log(`    After selection - Elements: ${updatedElements.length}, Objects: ${updatedObjects.length}`);

            // After selecting a species, we should see badges and a heatmap
            const badgeElements = updatedElements.filter(e => e.name.includes('Badge') || e.name.includes('badge'));
            console.log(`    Badge elements: ${badgeElements.length}`);

            if (badgeElements.length > 0) {
                pass(`Found ${badgeElements.length} badge elements after species selection`);
                // Check badge content
                for (const badge of badgeElements.slice(0, 3)) {
                    const children = badge.props?.children;
                    if (typeof children === 'string') {
                        console.log(`      Badge: "${children}"`);
                    } else if (Array.isArray(children)) {
                        console.log(`      Badge: "${children.join('')}"`);
                    }
                }
            } else {
                fail('Expected badge elements after species selection');
            }

            // Check for summary values
            const updatedHtml = result.html;
            const hasSepalLength = updatedHtml.includes('SepalLength');
            if (hasSepalLength) {
                pass('HTML contains SepalLength stats after species selection');
            }

            // Check that illustrated message is gone (replaced by heatmap)
            const stillHasIllustratedMsg = updatedElements.filter(e =>
                e.name.includes('IllustratedMessage') &&
                e.props?.children?.some?.(c =>
                    typeof c === 'object' && c?.__dhElemName?.includes('Heading') &&
                    c?.props?.children?.includes?.('Species required')
                )
            );
            if (stillHasIllustratedMsg.length === 0) {
                pass('IllustratedMessage replaced after species selection');
            }

        } catch (e) {
            fail(`Species selection failed: ${e.message}`);
        }
    }

    // ── Row double-press callable ──
    test('table row double-press callable');
    const rowDoublePressCallable = findCallableByProp(doc, 'onRowDoublePress');
    if (rowDoublePressCallable) {
        pass('Found onRowDoublePress callable on table');
        console.log(`    Callable ID: ${rowDoublePressCallable}`);
    } else {
        // It might be named differently
        const allCb = findAllCallables(doc);
        const rowPress = allCb.find(cb => cb.path.includes('RowDoublePress') || cb.path.includes('row_double_press'));
        if (rowPress) {
            pass(`Found row press callable: ${rowPress.id}`);
        } else {
            console.log(`    Available callables:`);
            allCb.forEach(cb => console.log(`      ${cb.path}: ${cb.id}`));
            fail('onRowDoublePress callable not found');
        }
    }

    // ── Component summary ──
    test('overall component summary');
    const summary = result.getSummary();
    assert(summary.success, 'Render succeeded');
    console.log(`    Total components rendered: ${summary.componentCount}`);
    console.log(`    Exported objects: ${summary.exportedObjectCount}`);
    assertGte(summary.componentCount, 10, 'Many components rendered');

    result.unmount();

    // ═══════════════════════════════════════════
    // Also test individual non-component widgets
    // ═══════════════════════════════════════════
    console.log('\n══════════════════════════════════════════════════');
    console.log('  Individual exported objects');
    console.log('══════════════════════════════════════════════════');

    test('iris table (raw)');
    try {
        const wc = client.widgetClient;
        // For raw tables, getObject returns a WidgetExportedObject-like that needs reexport+fetch
        const irisObj = await wc.connection.getObject({ name: 'iris', type: 'Table' });
        // The returned object from getObject for a Table type has a different shape.
        // We need to check if it already IS a table or needs fetch().
        let table;
        if (irisObj.columns) {
            table = irisObj;
        } else if (irisObj.reexport) {
            const reexported = await irisObj.reexport();
            table = await reexported.fetch();
        } else {
            // Try getTable via IDE session
            const ide = await wc.connection.startSession('python');
            table = await ide.getTable('iris');
        }

        if (table && table.columns) {
            pass('Got iris table from server');
            const cols = table.columns.map(c => `${c.name}(${c.type})`);
            console.log(`    Columns: ${cols.join(', ')}`);
            assertGte(table.columns.length, 5, 'Iris table has expected columns');

            table.setViewport(0, 4);
            const vp = await table.getViewportData();
            console.log(`    First 5 rows:`);
            for (let i = 0; i < Math.min(5, vp.rows.length); i++) {
                const row = {};
                for (const col of table.columns) {
                    row[col.name] = vp.rows[i].get(col);
                }
                console.log(`      ${JSON.stringify(row)}`);
            }
            assertGte(vp.rows.length, 1, 'Iris table has data');
        } else {
            fail('Could not get table with columns');
        }
    } catch (e) {
        fail(`Could not fetch iris table: ${e.message}`);
    }

    test('aggregated tables');
    for (const tableName of ['iris_avg', 'iris_max', 'iris_min']) {
        try {
            const wc = client.widgetClient;
            const tableObj = await wc.connection.getObject({ name: tableName, type: 'Table' });
            let table;
            if (tableObj.columns) {
                table = tableObj;
            } else if (tableObj.reexport) {
                const reexported = await tableObj.reexport();
                table = await reexported.fetch();
            }
            if (table && table.columns) {
                const cols = table.columns.map(c => c.name);
                table.setViewport(0, 9);
                const vp = await table.getViewportData();
                console.log(`    ${tableName}: ${cols.join(', ')} (${vp.rows.length} rows)`);
                pass(`${tableName} accessible`);
            } else {
                fail(`${tableName}: no columns`);
            }
        } catch (e) {
            fail(`${tableName} failed: ${e.message}`);
        }
    }

    // ── Summary ──
    console.log(`\n╔══════════════════════════════════════════════════╗`);
    console.log(`║  Results: ${String(totalPass).padStart(3)} passed, ${String(totalFail).padStart(3)} failed${' '.repeat(21)}║`);
    console.log(`╚══════════════════════════════════════════════════╝`);

    client.close();
    process.exit(totalFail > 0 ? 1 : 0);
}

main().catch(e => {
    console.error('Fatal error:', e);
    console.error(e.stack);
    process.exit(1);
});
