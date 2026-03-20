/**
 * Example: TAP output + table assertions + snapshots
 *
 * Demonstrates all the new testing features together.
 * Output is TAP-compatible for CI consumption.
 *
 * Prerequisites:
 *   dh serve /workspace/test_iris_dashboard.py --no-browser --port 10000
 *
 * Run:
 *   node examples/test-with-tap.mjs
 */
import {
    createTestClient,
    findAllElements,
    findAllObjects,
    findCallableByProp,
    prettyPrintDocument,
    TapReporter,
    assertRowCount,
    assertMinRowCount,
    assertColumns,
    assertColumnContains,
    assertColumnAll,
    assertTableHas,
    assertColumnUnique,
    assertColumnInRange,
} from '../src/index.mjs';

const SERVER_URL = 'http://localhost:10000';
const tap = new TapReporter();

async function main() {
    // ── Connect ──
    let client;
    try {
        client = await createTestClient(SERVER_URL);
        tap.pass('connected to server');
    } catch (e) {
        tap.fail('connected to server', e);
        tap.done();
    }

    // ── Render dashboard ──
    let result;
    try {
        result = await client.render('iris_species_dashboard_final', {
            widgetType: 'deephaven.ui.Dashboard',
            timeout: 15000,
        });
        tap.pass('dashboard rendered');
    } catch (e) {
        tap.fail('dashboard rendered', e);
        client.close();
        tap.done();
    }

    // ── Document structure assertions ──
    const doc = result.document;
    const allElements = findAllElements(doc);
    const allObjects = findAllObjects(doc);

    tap.test('has >= 10 elements', () => {
        if (allElements.length < 10) throw new Error(`Only ${allElements.length} elements`);
    });

    tap.test('has exported objects', () => {
        if (allObjects.length < 1) throw new Error('No exported objects');
    });

    tap.test('has Panel components', () => {
        if (!allElements.some(e => e.name.includes('Panel'))) throw new Error('No Panel found');
    });

    tap.test('has Picker component', () => {
        if (!allElements.some(e => e.name.includes('Picker'))) throw new Error('No Picker found');
    });

    tap.test('has Tabs component', () => {
        if (!allElements.some(e => e.name.includes('Tabs'))) throw new Error('No Tabs found');
    });

    // ── Table assertions on exported objects ──
    const tableObjects = [...result.exportedObjects.entries()]
        .filter(([_, obj]) => obj.type === 'Table');

    tap.test(`has table objects (found ${tableObjects.length})`, () => {
        if (tableObjects.length < 1) throw new Error('No table objects');
    });

    // Test one of the table exports
    if (tableObjects.length > 0) {
        const [tableId] = tableObjects[0];
        await tap.testAsync('can fetch table data', async () => {
            const { columns, rows } = await result.fetchTableData(tableId);
            assertMinRowCount(rows, 1, 'Table has at least 1 row');
            assertColumns(columns, ['Species'], 'Table has Species column');
        });
    }

    // ── Raw table assertions ──
    await tap.testAsync('iris_avg table: correct structure', async () => {
        const wc = client.widgetClient;
        const tableObj = await wc.connection.getObject({ name: 'iris_avg', type: 'Table' });
        let table;
        if (tableObj.columns) {
            table = tableObj;
        } else if (tableObj.reexport) {
            const reexported = await tableObj.reexport();
            table = await reexported.fetch();
        }

        const columns = table.columns.map(c => ({ name: c.name, type: c.type }));
        assertColumns(columns, ['Species', 'SepalLength', 'SepalWidth', 'PetalLength', 'PetalWidth']);

        table.setViewport(0, 9);
        const vp = await table.getViewportData();
        const rows = [];
        for (let i = 0; i < vp.rows.length; i++) {
            const row = {};
            for (const col of table.columns) {
                row[col.name] = vp.rows[i].get(col);
            }
            rows.push(row);
        }

        assertRowCount(rows, 3, '3 species in averages');
        assertColumnContains(rows, 'Species', 'setosa');
        assertColumnContains(rows, 'Species', 'versicolor');
        assertColumnContains(rows, 'Species', 'virginica');
        assertColumnUnique(rows, 'Species');
        assertColumnAll(rows, 'SepalLength', v => typeof v === 'number' && v > 0, 'SepalLength is positive number');
        assertColumnInRange(rows, 'SepalLength', 1, 10, 'SepalLength avg in reasonable range');
    });

    await tap.testAsync('iris table: has data', async () => {
        const wc = client.widgetClient;
        const tableObj = await wc.connection.getObject({ name: 'iris', type: 'Table' });
        let table;
        if (tableObj.columns) {
            table = tableObj;
        } else if (tableObj.reexport) {
            const reexported = await tableObj.reexport();
            table = await reexported.fetch();
        }

        const columns = table.columns.map(c => ({ name: c.name, type: c.type }));
        assertColumns(columns, ['Species', 'SepalLength', 'SepalWidth']);

        table.setViewport(0, 4);
        const vp = await table.getViewportData();
        const rows = [];
        for (let i = 0; i < vp.rows.length; i++) {
            const row = {};
            for (const col of table.columns) {
                row[col.name] = vp.rows[i].get(col);
            }
            rows.push(row);
        }

        assertMinRowCount(rows, 5, 'Iris has at least 5 rows in viewport');
    });

    // ── Picker interaction ──
    const pickerCallable = findCallableByProp(doc, 'onChange', (el) =>
        el.name.includes('Picker')
    );

    tap.test('picker has onChange callable', () => {
        if (!pickerCallable) throw new Error('No picker onChange callable');
    });

    if (pickerCallable) {
        await tap.testAsync('selecting species triggers re-render', async () => {
            const updateP = result.waitForUpdate(10000);
            await result.fireCallable(pickerCallable, ['setosa']);
            await updateP;
        });

        tap.test('badges appear after species selection', () => {
            const updatedElements = findAllElements(result.document);
            const badges = updatedElements.filter(e => e.name.includes('Badge'));
            if (badges.length < 1) throw new Error('No badge elements after selection');
        });
    }

    // ── Cleanup ──
    result.unmount();
    client.close();
    tap.done();
}

main().catch(e => {
    tap.fail(`fatal error: ${e.message}`, e);
    tap.done();
});
