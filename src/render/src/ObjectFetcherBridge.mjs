/**
 * ObjectFetcherBridge — implements the ObjectFetchManager interface expected by
 * useObjectFetch() / useWidget() from @deephaven/jsapi-bootstrap.
 *
 * Bridges our existing IdeConnection to the React context system so that
 * WidgetHandler can fetch widgets using its standard useWidget() hook.
 *
 * Interface:
 *   ObjectFetchManager = {
 *     subscribe(descriptor, onUpdate) → unsubscribe
 *   }
 *
 * onUpdate receives:
 *   { status: 'loading' }
 *   { status: 'ready', fetch: () => Promise<widget> }
 *   { status: 'error', error: Error }
 */

/**
 * Create an ObjectFetchManager backed by a Deephaven IdeConnection.
 *
 * @param {object} connection - dh.IdeConnection instance
 * @returns {object} ObjectFetchManager
 */
export function createObjectFetchManager(connection) {
    return {
        /**
         * Subscribe to fetch state for a widget descriptor.
         * Immediately fires 'ready' with a fetch function since our connection
         * is already established.
         *
         * @param {object} descriptor - { type, name } or { type, id } or string URI
         * @param {function} onUpdate - Callback receiving { status, fetch?, error? }
         * @returns {function} Unsubscribe function
         */
        subscribe(descriptor, onUpdate) {
            let cancelled = false;

            // Exported objects from the document tree (WidgetExportedObject) have
            // their own fetch()/reexport() methods. useExportedObject handles them
            // via fetchReexportedObject — we must NOT resolve them here or we'll
            // double-consume the descriptor. Keep them in 'loading' so useWidget
            // returns null and the fetchReexportedObject path takes over.
            if (descriptor != null && typeof descriptor === 'object' &&
                typeof descriptor.fetch === 'function' && typeof descriptor.type === 'string') {
                onUpdate({ status: 'loading' });
                return () => { cancelled = true; };
            }

            // Named objects and URI objects — signal ready immediately
            onUpdate({
                status: 'ready',
                fetch: async () => {
                    if (cancelled) {
                        throw new Error('Subscription cancelled');
                    }
                    return connection.getObject(descriptor);
                },
            });

            return () => {
                cancelled = true;
            };
        },
    };
}
