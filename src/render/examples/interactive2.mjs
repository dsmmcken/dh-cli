/**
 * Interactive example v2: properly tracks callable IDs across re-renders
 */
import { createTestClient } from '../src/index.mjs';

const SERVER_URL = 'http://localhost:10000';

function findCallableId(doc, path) {
    const parts = path.split('.');
    let current = doc;
    for (const part of parts) {
        if (!current || typeof current !== 'object') return null;
        current = current[part];
    }
    return current?.__dhCbid || null;
}

function getIncrementCallableId(doc) {
    // Navigate to the onPress of the "Increment" button (3rd child of Flex)
    return findCallableId(doc, 'props.children.props.children.2.props.onPress');
}

function getResetCallableId(doc) {
    return findCallableId(doc, 'props.children.props.children.3.props.onPress');
}

async function main() {
    console.log('=== Interactive Test v2 ===\n');

    const client = await createTestClient(SERVER_URL);
    const result = await client.render('counter_widget');

    const getText = () => {
        const heading = result.querySelector('[role="heading"]');
        return heading?.textContent || 'not found';
    };

    console.log(`Initial: ${getText()}`);

    // Click increment 3 times, reading fresh callable IDs each time
    for (let i = 0; i < 3; i++) {
        const cbId = getIncrementCallableId(result.document);
        console.log(`  Increment callable: ${cbId}`);
        const updatePromise = result.waitForUpdate();
        await result.fireCallable(cbId, []);
        await updatePromise;
        console.log(`After increment ${i + 1}: ${getText()}`);
    }

    // Click reset
    const resetId = getResetCallableId(result.document);
    console.log(`  Reset callable: ${resetId}`);
    const updatePromise = result.waitForUpdate();
    await result.fireCallable(resetId, []);
    await updatePromise;
    console.log(`After reset: ${getText()}`);

    client.close();
    process.exit(0);
}

main().catch(e => {
    console.error('Error:', e);
    process.exit(1);
});
