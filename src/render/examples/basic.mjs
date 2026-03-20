/**
 * Basic example: render a Deephaven UI widget and inspect the result.
 *
 * Prerequisites:
 *   dh serve test_dashboard.py --no-browser --port 10000
 */
import { renderWidget, createTestClient } from '../src/index.mjs';

const SERVER_URL = 'http://localhost:10000';

async function main() {
    console.log('=== DH Render Test: Basic Example ===\n');

    // Method 1: Quick one-shot render
    console.log('--- One-shot render ---');
    const result = await renderWidget(SERVER_URL, 'my_widget');

    console.log('Document:');
    console.log(JSON.stringify(result.document, null, 2));

    console.log('\nRendered HTML:');
    console.log(result.html);

    console.log('\nComponent summary:');
    const summary = result.getSummary();
    console.log(JSON.stringify(summary, null, 2));

    // Query the DOM
    console.log('\nPanels found:', result.findByRole('region').length);
    console.log('Tables found:', result.findByRole('table').length);

    // Find by component name
    const panels = result.findByComponent('deephaven.ui.components.Panel');
    for (const panel of panels) {
        console.log(`Panel: "${panel.getAttribute('data-title')}"`);
    }

    // Fetch table data from exported object
    console.log('\n--- Table Data ---');
    const tableData = await result.fetchTableData(0);
    console.log('Columns:', tableData.columns.map(c => `${c.name}(${c.type})`).join(', '));
    console.log('Rows:');
    for (const row of tableData.rows) {
        console.log(`  ${JSON.stringify(row)}`);
    }

    // Clean up
    result.unmount();
    console.log('\nDone!');
    process.exit(0);
}

main().catch(e => {
    console.error('Error:', e);
    process.exit(1);
});
