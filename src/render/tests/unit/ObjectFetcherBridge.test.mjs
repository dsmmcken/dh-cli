import { describe, it, expect, vi } from 'vitest';
import { createObjectFetchManager } from '../../src/ObjectFetcherBridge.mjs';

describe('createObjectFetchManager', () => {
    it('returns object with subscribe method', () => {
        const connection = { getObject: vi.fn() };
        const manager = createObjectFetchManager(connection);
        expect(typeof manager.subscribe).toBe('function');
    });

    it('subscribe immediately fires callback with status ready', () => {
        const connection = { getObject: vi.fn() };
        const manager = createObjectFetchManager(connection);
        const onUpdate = vi.fn();
        manager.subscribe({ type: 'Figure', name: 'my_fig' }, onUpdate);
        expect(onUpdate).toHaveBeenCalledTimes(1);
        expect(onUpdate).toHaveBeenCalledWith(
            expect.objectContaining({ status: 'ready' })
        );
        expect(typeof onUpdate.mock.calls[0][0].fetch).toBe('function');
    });

    it('fetch function returns connection.getObject result', async () => {
        const fakeWidget = { type: 'Table', id: 42 };
        const connection = { getObject: vi.fn().mockResolvedValue(fakeWidget) };
        const manager = createObjectFetchManager(connection);
        const onUpdate = vi.fn();
        const descriptor = { type: 'Table', name: 'my_table' };
        manager.subscribe(descriptor, onUpdate);

        const { fetch } = onUpdate.mock.calls[0][0];
        const result = await fetch();
        expect(result).toBe(fakeWidget);
        expect(connection.getObject).toHaveBeenCalledWith(descriptor);
    });

    it('unsubscribe prevents further fetches', async () => {
        const connection = { getObject: vi.fn().mockResolvedValue({}) };
        const manager = createObjectFetchManager(connection);
        const onUpdate = vi.fn();
        const unsubscribe = manager.subscribe({ type: 'Table', name: 't' }, onUpdate);

        const { fetch } = onUpdate.mock.calls[0][0];
        unsubscribe();

        await expect(fetch()).rejects.toThrow('Subscription cancelled');
    });

    it('handles connection.getObject rejection', async () => {
        const connection = {
            getObject: vi.fn().mockRejectedValue(new Error('network error')),
        };
        const manager = createObjectFetchManager(connection);
        const onUpdate = vi.fn();
        manager.subscribe({ type: 'Table', name: 't' }, onUpdate);

        const { fetch } = onUpdate.mock.calls[0][0];
        await expect(fetch()).rejects.toThrow('network error');
    });

    it('works with different descriptor formats', async () => {
        const fakeObj = { data: 'test' };
        const connection = { getObject: vi.fn().mockResolvedValue(fakeObj) };
        const manager = createObjectFetchManager(connection);
        const onUpdate = vi.fn();

        // String URI descriptor
        const uriDescriptor = 'dh://table/my_table';
        manager.subscribe(uriDescriptor, onUpdate);

        const { fetch } = onUpdate.mock.calls[0][0];
        const result = await fetch();
        expect(result).toBe(fakeObj);
        expect(connection.getObject).toHaveBeenCalledWith(uriDescriptor);
    });
});
