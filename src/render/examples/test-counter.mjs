/**
 * Clean test example: test a counter widget with assertions.
 *
 * Prerequisites:
 *   dh serve /workspace/test_interactive.py --no-browser --port 10000
 */
import { createTestClient, findCallableByButtonText, prettyPrintDocument } from '../src/index.mjs';

const SERVER_URL = 'http://localhost:10000';

// Simple assertion helpers
let passCount = 0;
let failCount = 0;

function assert(condition, message) {
    if (condition) {
        console.log(`  ✓ ${message}`);
        passCount++;
    } else {
        console.log(`  ✗ ${message}`);
        failCount++;
    }
}

function assertEqual(actual, expected, message) {
    assert(actual === expected, `${message}: expected "${expected}", got "${actual}"`);
}

function assertContains(text, substring, message) {
    assert(text.includes(substring), `${message}: "${substring}" in "${text.substring(0, 100)}"`);
}

async function main() {
    console.log('DH Render Test Suite\n');

    const client = await createTestClient(SERVER_URL);

    // Test 1: Initial render
    console.log('Test: Counter widget initial render');
    const result = await client.render('counter_widget');

    console.log('\nComponent tree:');
    console.log(prettyPrintDocument(result.document));

    assert(result.html.length > 0, 'Widget renders HTML');
    assertContains(result.html, 'Count: 0', 'Initial count is 0');
    assertContains(result.html, 'Message: Hello', 'Initial message is Hello');
    assertEqual(result.findByRole('button').length, 2, 'Two buttons rendered');
    assertEqual(result.findByText('Increment').length, 1, 'Increment button exists');
    assertEqual(result.findByText('Reset').length, 1, 'Reset button exists');

    // Test 2: Increment
    console.log('\nTest: Increment button');
    let cbId = findCallableByButtonText(result.document, 'Increment');
    assert(cbId !== null, 'Increment callable found');

    let updatePromise = result.waitForUpdate();
    await result.fireCallable(cbId, []);
    await updatePromise;
    assertContains(result.html, 'Count: 1', 'Count incremented to 1');

    // Test 3: Multiple increments
    console.log('\nTest: Multiple increments');
    for (let i = 2; i <= 5; i++) {
        cbId = findCallableByButtonText(result.document, 'Increment');
        updatePromise = result.waitForUpdate();
        await result.fireCallable(cbId, []);
        await updatePromise;
    }
    assertContains(result.html, 'Count: 5', 'Count incremented to 5');

    // Test 4: Reset
    console.log('\nTest: Reset button');
    cbId = findCallableByButtonText(result.document, 'Reset');
    assert(cbId !== null, 'Reset callable found');
    updatePromise = result.waitForUpdate();
    await result.fireCallable(cbId, []);
    await updatePromise;
    assertContains(result.html, 'Count: 0', 'Count reset to 0');

    // Summary
    console.log(`\n${'='.repeat(40)}`);
    console.log(`Results: ${passCount} passed, ${failCount} failed`);
    console.log(`${'='.repeat(40)}`);

    client.close();
    process.exit(failCount > 0 ? 1 : 0);
}

main().catch(e => {
    console.error('Test error:', e);
    process.exit(1);
});
