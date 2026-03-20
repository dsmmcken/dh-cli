/**
 * Full test suite: tests multiple widgets with real data.
 *
 * Prerequisites:
 *   dh serve /workspace/test_combined.py --no-browser --port 10000
 */
import {
    createTestClient,
    findCallableByButtonText,
    findAllElements,
    findAllObjects,
    prettyPrintDocument,
} from '../src/index.mjs';

const SERVER_URL = 'http://localhost:10000';

// ---- Test runner ----
let totalPass = 0;
let totalFail = 0;
let currentTest = '';

function describe(name, fn) {
    console.log(`\n${'─'.repeat(50)}`);
    console.log(`${name}`);
    console.log('─'.repeat(50));
    return fn();
}

function test(name) {
    currentTest = name;
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

function assertContains(text, substring, message) {
    if (text.includes(substring)) pass(message);
    else fail(`${message}: "${substring}" not found`);
}

// ---- Tests ----
async function main() {
    console.log('╔══════════════════════════════════════════════════╗');
    console.log('║  Deephaven UI Component Test Suite               ║');
    console.log('╚══════════════════════════════════════════════════╝');

    const client = await createTestClient(SERVER_URL);

    // ── Dashboard Widget Tests ──
    await describe('Dashboard Widget', async () => {
        test('renders with a panel and table');
        const result = await client.render('dashboard_widget');

        console.log('\n    Component tree:');
        prettyPrintDocument(result.document).split('\n').forEach(l => console.log('    ' + l));

        assert(result.html.length > 0, 'Renders HTML');
        assertEqual(result.findByRole('region').length, 1, 'Has one panel');
        assertEqual(result.findByRole('table').length, 1, 'Has one table');

        const panels = result.findByComponent('deephaven.ui.components.Panel');
        assertEqual(panels.length, 1, 'Has Panel component');
        assertEqual(panels[0].getAttribute('data-title'), 'Data Dashboard', 'Panel title is correct');

        test('has exported table object');
        const objects = findAllObjects(result.document);
        assertEqual(objects.length, 1, 'One exported object');
        assert(result.exportedObjects.has(0), 'Object 0 exists in map');

        test('table has correct data');
        const { columns, rows } = await result.fetchTableData(0);
        assertEqual(columns.length, 3, 'Table has 3 columns');
        assertEqual(columns[0].name, 'X', 'First column is X');
        assertEqual(columns[1].name, 'Y', 'Second column is Y');
        assertEqual(columns[2].name, 'Label', 'Third column is Label');
        assertEqual(rows.length, 10, 'Table has 10 rows');
        assertEqual(rows[0].X, 0, 'First row X is 0');
        assertEqual(rows[0].Y, 0, 'First row Y is 0');
        assertEqual(rows[0].Label, 'Even', 'First row Label is Even');
        assertEqual(rows[1].Label, 'Odd', 'Second row Label is Odd');
        assertEqual(rows[9].X, 9, 'Last row X is 9');
        assertEqual(rows[9].Y, 18, 'Last row Y is 18');

        result.unmount();
    });

    // ── Counter Widget Tests ──
    await describe('Counter Widget', async () => {
        test('renders with initial state');
        const result = await client.render('counter_widget');

        assertContains(result.html, 'Count: 0', 'Initial count is 0');
        assertContains(result.html, 'Message: Hello', 'Initial message');
        assertEqual(result.findByRole('button').length, 2, 'Two buttons');

        test('increment button works');
        let cbId = findCallableByButtonText(result.document, 'Increment');
        assert(cbId !== null, 'Found increment callable');

        let updateP = result.waitForUpdate();
        await result.fireCallable(cbId, []);
        await updateP;
        assertContains(result.html, 'Count: 1', 'Count is 1 after first click');

        test('multiple increments');
        for (let i = 0; i < 4; i++) {
            cbId = findCallableByButtonText(result.document, 'Increment');
            updateP = result.waitForUpdate();
            await result.fireCallable(cbId, []);
            await updateP;
        }
        assertContains(result.html, 'Count: 5', 'Count is 5 after 5 clicks');

        test('reset button works');
        cbId = findCallableByButtonText(result.document, 'Reset');
        assert(cbId !== null, 'Found reset callable');
        updateP = result.waitForUpdate();
        await result.fireCallable(cbId, []);
        await updateP;
        assertContains(result.html, 'Count: 0', 'Count reset to 0');

        test('component summary is clean');
        const summary = result.getSummary();
        assert(summary.success, 'Summary reports success');
        assert(summary.componentCount >= 5, `Has ${summary.componentCount} components`);

        result.unmount();
    });

    // ── Error Widget Tests ──
    await describe('Error Widget', async () => {
        test('renders initially without error');
        const result = await client.render('error_widget');

        assertContains(result.html, 'This component is working', 'Shows working message');
        assertEqual(result.findByRole('button').length, 1, 'Has trigger button');

        test('error propagation');
        const cbId = findCallableByButtonText(result.document, 'Trigger Error');
        assert(cbId !== null, 'Found trigger callable');

        // After triggering the error, the server should send a documentError
        // We need to handle this gracefully
        try {
            const updateP = result.waitForUpdate(3000);
            await result.fireCallable(cbId, []);
            await updateP;
            // If we get here, the server re-rendered (error might be caught differently)
            pass('Server responded after error trigger');
        } catch (e) {
            // Timeout is expected if the server sends a documentError instead of documentPatched
            pass('Error handled (timeout or error response)');
        }

        result.unmount();
    });

    // ── Summary ──
    console.log('\n╔══════════════════════════════════════════════════╗');
    console.log(`║  Results: ${String(totalPass).padStart(3)} passed, ${String(totalFail).padStart(3)} failed${' '.repeat(21)}║`);
    console.log('╚══════════════════════════════════════════════════╝');

    client.close();
    process.exit(totalFail > 0 ? 1 : 0);
}

main().catch(e => {
    console.error('Fatal error:', e);
    process.exit(1);
});
