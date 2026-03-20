// Debug: list all available objects on the server
import { JsApiLoader } from '../src/JsApiLoader.mjs';
import { WidgetClient } from '../src/WidgetClient.mjs';

const SERVER_URL = 'http://localhost:10000';

async function main() {
    const loader = new JsApiLoader(SERVER_URL);
    const { dh } = await loader.load();

    const client = new dh.CoreClient(SERVER_URL);
    await client.login({ type: dh.CoreClient.LOGIN_TYPE_ANONYMOUS });
    const connection = await client.getAsIdeConnection();

    console.log('Connected. Waiting for field updates...');

    // Subscribe and wait for field updates
    const fields = await new Promise((resolve) => {
        connection.addEventListener('fieldUpdates', (event) => {
            resolve(event);
        });
        connection.subscribeToFieldUpdates();
        setTimeout(() => resolve(null), 5000);
    });

    if (fields && fields.detail) {
        console.log('\nField updates:');
        const detail = fields.detail;
        if (detail.created) {
            console.log('Created fields:');
            for (const field of detail.created) {
                console.log(`  ${field.name} (${field.type})`);
            }
        }
        if (detail.updated) {
            console.log('Updated fields:');
            for (const field of detail.updated) {
                console.log(`  ${field.name} (${field.type})`);
            }
        }
    } else {
        console.log('No field updates received. Trying knownFields...');
    }

    // Also try to get the known fields directly
    console.log('\nKnown fields from connection:');
    try {
        const knownFields = connection.knownFields;
        if (knownFields) {
            // It might be a Java-style set/map
            const entries = [];
            if (knownFields.forEach) {
                knownFields.forEach((v) => entries.push(v));
            } else if (knownFields.hashCodeMap || knownFields.stringMap) {
                // GWT Map structure
                const stringMap = knownFields.stringMap;
                if (stringMap) {
                    for (const key of Object.keys(stringMap)) {
                        entries.push(stringMap[key]);
                    }
                }
            }
            for (const entry of entries) {
                if (entry.name_0 !== undefined) {
                    console.log(`  ${entry.name_0} (${entry.type_0})`);
                } else {
                    console.log(`  ${JSON.stringify(entry).substring(0, 200)}`);
                }
            }
        }
    } catch (e) {
        console.log('Could not read knownFields:', e.message);
    }

    // Try specific widget types
    console.log('\nTrying to get iris_species_dashboard_final:');
    const types = ['deephaven.ui.Element', 'deephaven.ui.Dashboard'];
    for (const type of types) {
        try {
            const widget = await connection.getObject({ name: 'iris_species_dashboard_final', type });
            console.log(`  SUCCESS with type "${type}"!`);
            console.log(`  Widget type: ${widget.type}`);
            console.log(`  Data: ${JSON.stringify(widget.getDataAsString()).substring(0, 200)}`);
            widget.close();
        } catch (e) {
            console.log(`  FAILED with type "${type}": ${e.message || e}`);
        }
    }

    // Try other known objects
    console.log('\nTrying other objects:');
    const names = ['iris', 'ui_iris', 'sepal_panel', 'about_panel', 'scatter_by_species',
                   'sepal_tabs', 'iris_avg', 'iris_max', 'iris_min', 'species_table',
                   'sepal_flex_tabs', 'about_markdown'];
    for (const name of names) {
        for (const type of ['Table', 'Figure', 'deephaven.ui.Element']) {
            try {
                const obj = await connection.getObject({ name, type });
                console.log(`  ${name} (${type}): type=${obj.type}`);
                obj.close();
                break;
            } catch (e) {
                // skip
            }
        }
    }

    process.exit(0);
}

main().catch(e => {
    console.error('Error:', e);
    process.exit(1);
});
