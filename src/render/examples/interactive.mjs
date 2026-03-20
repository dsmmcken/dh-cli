/**
 * Interactive example: test a counter component with button clicks.
 *
 * Prerequisites:
 *   dh serve /workspace/test_interactive.py --no-browser --port 10000
 */
import { createTestClient } from '../src/index.mjs';

const SERVER_URL = 'http://localhost:10000';

async function main() {
    console.log('=== DH Render Test: Interactive Example ===\n');

    const client = await createTestClient(SERVER_URL);
    const result = await client.render('counter_widget');

    console.log('--- Initial Render ---');
    console.log('HTML:', result.html);
    console.log();

    // Find the document structure
    const doc = result.document;
    console.log('Document:', JSON.stringify(doc, null, 2));

    // Find all callables in the document
    function findCallables(obj, path = '') {
        const callables = [];
        if (!obj || typeof obj !== 'object') return callables;
        if ('__dhCbid' in obj) {
            callables.push({ path, id: obj.__dhCbid });
            return callables;
        }
        for (const [key, value] of Object.entries(obj)) {
            callables.push(...findCallables(value, `${path}.${key}`));
        }
        return callables;
    }

    const callables = findCallables(doc);
    console.log('\nCallables found:');
    for (const cb of callables) {
        console.log(`  ${cb.path}: ${cb.id}`);
    }

    // Find buttons and their callables
    const buttons = result.findByRole('button');
    console.log('\nButtons:', buttons.length);
    for (const btn of buttons) {
        console.log(`  "${btn.textContent}"`);
    }

    // Click "Increment" button - fire the callable
    const incrementCb = callables.find(cb => cb.path.includes('children.2') || cb.path.includes('on_press'));
    if (incrementCb) {
        console.log(`\n--- Clicking Increment (${incrementCb.id}) ---`);
        const updatePromise = result.waitForUpdate();
        await result.fireCallable(incrementCb.id, []);
        await updatePromise;
        console.log('After increment HTML:', result.html);
    }

    // Click again
    if (incrementCb) {
        console.log('\n--- Clicking Increment again ---');
        const updatePromise = result.waitForUpdate();
        await result.fireCallable(incrementCb.id, []);
        await updatePromise;
        console.log('After 2nd increment HTML:', result.html);
    }

    // Click "Reset"
    const resetCb = callables.find(cb => cb.path.includes('children.3') || cb.id !== incrementCb?.id);
    if (resetCb && resetCb.id !== incrementCb?.id) {
        console.log(`\n--- Clicking Reset (${resetCb.id}) ---`);
        const updatePromise = result.waitForUpdate();
        await result.fireCallable(resetCb.id, []);
        await updatePromise;
        console.log('After reset HTML:', result.html);
    }

    console.log('\n--- Final Summary ---');
    console.log(JSON.stringify(result.getSummary(), null, 2));

    client.close();
    console.log('\nDone!');
    process.exit(0);
}

main().catch(e => {
    console.error('Error:', e);
    process.exit(1);
});
