import { describe, it, expect, beforeAll, afterAll } from 'vitest';
import { loadProviders, TestProviderStack } from '../../src/TestProviderStack.mjs';
import { createObjectFetchManager } from '../../src/ObjectFetcherBridge.mjs';
import { loadGoldenLayout, createTestLayout } from '../../src/GoldenLayoutSetup.mjs';
import { renderAndWait } from '../helpers/render-utils.mjs';

let React, h;

beforeAll(async () => {
    await loadProviders();
    await loadGoldenLayout();
    React = (await import('react')).default;
    h = React.createElement;
}, 30000);

describe('loadProviders', () => {
    it('succeeds without error', () => {
        // loadProviders completed in beforeAll without throwing
        expect(true).toBe(true);
    });
});

describe('TestProviderStack', () => {
    it('renders children', async () => {
        const child = h('div', { 'data-testid': 'child' });
        const element = h(TestProviderStack, null, child);
        const { container, cleanup } = await renderAndWait(element);
        try {
            const found = container.querySelector('[data-testid="child"]');
            expect(found).not.toBeNull();
        } finally {
            cleanup();
        }
    });

    it('renders children with text content', async () => {
        const child = h('span', null, 'Hello from provider stack');
        const element = h(TestProviderStack, null, child);
        const { container, cleanup } = await renderAndWait(element);
        try {
            expect(container.textContent).toContain('Hello from provider stack');
        } finally {
            cleanup();
        }
    });

    it('accepts dh prop', async () => {
        const fakeDh = { CoreClient: class {} };
        const child = h('div', { 'data-testid': 'dh-child' });
        const element = h(TestProviderStack, { dh: fakeDh }, child);
        const { container, cleanup } = await renderAndWait(element);
        try {
            const found = container.querySelector('[data-testid="dh-child"]');
            expect(found).not.toBeNull();
        } finally {
            cleanup();
        }
    });

    it('accepts objectFetchManager prop', async () => {
        const fakeConnection = {
            getObject: async () => ({}),
        };
        const ofm = createObjectFetchManager(fakeConnection);
        const child = h('div', { 'data-testid': 'ofm-child' });
        const element = h(TestProviderStack, { objectFetchManager: ofm }, child);
        const { container, cleanup } = await renderAndWait(element);
        try {
            const found = container.querySelector('[data-testid="ofm-child"]');
            expect(found).not.toBeNull();
        } finally {
            cleanup();
        }
    });

    it('accepts layoutManager prop', async () => {
        const win = globalThis.__TEST_DOM__.window;
        const layoutResult = createTestLayout(win);
        try {
            const child = h('div', { 'data-testid': 'layout-child' });
            const element = h(TestProviderStack, { layoutManager: layoutResult.layout }, child);
            const { container, cleanup } = await renderAndWait(element);
            try {
                const found = container.querySelector('[data-testid="layout-child"]');
                expect(found).not.toBeNull();
            } finally {
                cleanup();
            }
        } finally {
            layoutResult.destroy();
        }
    });

    it('renders real DH Text component', async () => {
        const { Text } = await import('@deephaven/components');
        const child = h(Text, null, 'DH Text Content');
        const element = h(TestProviderStack, null, child);
        const { container, cleanup } = await renderAndWait(element);
        try {
            expect(container.textContent).toContain('DH Text Content');
        } finally {
            cleanup();
        }
    });

    it('renders real DH Button component', async () => {
        const { Button } = await import('@deephaven/components');
        const child = h(Button, { variant: 'primary' }, 'Click Me');
        const element = h(TestProviderStack, null, child);
        const { container, cleanup } = await renderAndWait(element);
        try {
            expect(container.textContent).toContain('Click Me');
        } finally {
            cleanup();
        }
    });
});
