#!/usr/bin/env node
/**
 * CLI tool for quick widget diagnosis.
 *
 * Usage:
 *   npx dh-diagnose http://localhost:10000 my_widget
 *   npx dh-diagnose http://localhost:10000 my_dashboard --type deephaven.ui.Dashboard
 *   npx dh-diagnose http://localhost:10000 --list
 */
import { diagnoseWidget, listWidgets } from '../src/index.mjs';

const args = process.argv.slice(2);

if (args.length === 0 || args.includes('--help') || args.includes('-h')) {
    console.log(`Usage:
  dh-diagnose <server-url> <widget-name> [options]
  dh-diagnose <server-url> --list

Options:
  --type <type>     Widget type (default: deephaven.ui.Element)
  --no-tables       Skip table data fetching
  --timeout <ms>    Timeout in ms (default: 15000)
  --list            List all available widgets on the server
  --help            Show this help message

Examples:
  dh-diagnose http://localhost:10000 my_widget
  dh-diagnose http://localhost:10000 my_dashboard --type deephaven.ui.Dashboard
  dh-diagnose http://localhost:10000 --list`);
    process.exit(0);
}

const serverUrl = args[0];

if (args.includes('--list')) {
    try {
        const widgets = await listWidgets(serverUrl);
        console.log(JSON.stringify(widgets, null, 2));
    } catch (e) {
        console.error(`Error: ${e.message || e}`);
        process.exit(1);
    }
    process.exit(0);
}

const widgetName = args[1];
if (!widgetName) {
    console.error('Error: widget name required. Use --list to discover available widgets.');
    process.exit(1);
}

const typeIdx = args.indexOf('--type');
const widgetType = typeIdx >= 0 ? args[typeIdx + 1] : 'deephaven.ui.Element';
const fetchTables = !args.includes('--no-tables');
const timeoutIdx = args.indexOf('--timeout');
const timeout = timeoutIdx >= 0 ? parseInt(args[timeoutIdx + 1], 10) : 15000;

try {
    const report = await diagnoseWidget(serverUrl, widgetName, {
        widgetType,
        fetchTables,
        timeout,
    });
    console.log(JSON.stringify(report, null, 2));
    process.exit(report.status === 'ok' ? 0 : 1);
} catch (e) {
    console.error(`Fatal error: ${e.message || e}`);
    process.exit(1);
}
