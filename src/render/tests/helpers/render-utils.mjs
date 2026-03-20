/**
 * Shared rendering utilities for unit tests that need React + jsdom.
 * Extracted from test_real_components.mjs.
 */

let _React, _ReactDOM;

async function getReact() {
    if (!_React) {
        _React = (await import('react')).default;
        _ReactDOM = await import('react-dom/client');
    }
    return { React: _React, ReactDOM: _ReactDOM, act: _React.act };
}

/**
 * Simple render + wait.
 * @param {Element} element - React element to render
 * @param {number} ms - Wait time in ms
 */
export async function renderAndWait(element, ms = 50) {
    const { ReactDOM, act } = await getReact();
    const dom = globalThis.__TEST_DOM__;
    const container = dom.window.document.createElement('div');
    dom.window.document.body.appendChild(container);
    const root = ReactDOM.createRoot(container);
    await act(async () => {
        root.render(element);
        await new Promise(resolve => setTimeout(resolve, ms));
    });
    return {
        container,
        root,
        cleanup: () => {
            _React.act(() => { root.unmount(); });
            container.remove();
        },
    };
}

/**
 * Render with act() and wait for async effects.
 * @param {Element} element - React element to render
 * @param {object} options
 * @param {number} options.waitMs - Delay between flushes
 * @param {number} options.maxWaitMs - Maximum wait time
 * @param {function} options.checkFn - Predicate to check if rendering is done
 */
export async function renderWithAct(element, { waitMs = 100, maxWaitMs = 3000, checkFn } = {}) {
    const { ReactDOM, act } = await getReact();
    const dom = globalThis.__TEST_DOM__;
    const container = dom.window.document.createElement('div');
    dom.window.document.body.appendChild(container);
    const root = ReactDOM.createRoot(container);

    await act(async () => {
        root.render(element);
    });

    const body = dom.window.document.body;
    const start = Date.now();
    while (Date.now() - start < maxWaitMs) {
        await act(async () => {
            await new Promise(resolve => setTimeout(resolve, waitMs));
        });
        if (checkFn && checkFn(body)) break;
    }

    return {
        container,
        body,
        root,
        cleanup: () => {
            _React.act(() => { root.unmount(); });
            container.remove();
        },
    };
}
