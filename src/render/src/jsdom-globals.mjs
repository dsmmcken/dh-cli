/**
 * Installs jsdom globals on globalThis so Deephaven/Spectrum components work in Node.js.
 *
 * Must be called before importing any @deephaven/* or @adobe/react-spectrum packages.
 *
 * @param {import('jsdom').JSDOM} dom - The JSDOM instance
 * @returns {{ restore: () => void }} - Call restore() to undo the global installation
 */
export function installJsdomGlobals(dom) {
    const w = dom.window;
    const saved = {};

    const globalNames = [
        'window', 'document',
        'HTMLElement', 'HTMLInputElement', 'HTMLTextAreaElement', 'HTMLSelectElement',
        'HTMLAnchorElement', 'HTMLButtonElement', 'HTMLDivElement', 'HTMLSpanElement',
        'HTMLFormElement', 'HTMLLabelElement', 'HTMLTableElement', 'HTMLTableRowElement',
        'SVGElement',
        'Node', 'Text', 'Element', 'DocumentFragment',
        'Event', 'CustomEvent', 'MouseEvent', 'KeyboardEvent', 'InputEvent', 'FocusEvent',
        'MutationObserver', 'ResizeObserver', 'IntersectionObserver',
        'getComputedStyle', 'requestAnimationFrame', 'cancelAnimationFrame',
        'Range', 'NodeFilter', 'TreeWalker', 'DOMParser', 'XMLSerializer',
        'navigator',
        'localStorage', 'sessionStorage',
        'CSSStyleDeclaration', 'CSSStyleSheet', 'StyleSheet',
        'Image', 'HTMLImageElement',
        'URL', 'URLSearchParams',
        'AbortController', 'AbortSignal',
        'fetch', 'Request', 'Response', 'Headers',
    ];

    for (const name of globalNames) {
        try {
            saved[name] = globalThis[name];
            if (w[name] !== undefined) {
                globalThis[name] = w[name];
            }
        } catch (e) {
            // Some globals may be read-only
        }
    }

    // matchMedia polyfill (jsdom doesn't implement it)
    if (!w.matchMedia) {
        const matchMediaFn = (query) => ({
            matches: false,
            media: query,
            onchange: null,
            addListener: () => {},
            removeListener: () => {},
            addEventListener: () => {},
            removeEventListener: () => {},
            dispatchEvent: () => false,
        });
        saved.matchMedia = globalThis.matchMedia;
        globalThis.matchMedia = matchMediaFn;
    }

    // ResizeObserver polyfill (jsdom doesn't implement it).
    // Must fire callbacks with non-zero dimensions so that components gated on
    // container size (e.g. DH ListViewWrapper checks contentRect.height > 0)
    // actually render their children.
    saved.ResizeObserver = globalThis.ResizeObserver;
    const ResizeObserverPolyfill = class ResizeObserver {
        constructor(callback) { this._callback = callback; }
        observe(element) {
            // Fire asynchronously (microtask) with default dimensions.
            Promise.resolve().then(() => {
                if (this._callback) {
                    this._callback([{
                        target: element,
                        contentRect: { x: 0, y: 0, width: 800, height: 600,
                            top: 0, left: 0, bottom: 600, right: 800 },
                        borderBoxSize: [{ inlineSize: 800, blockSize: 600 }],
                        contentBoxSize: [{ inlineSize: 800, blockSize: 600 }],
                    }], this);
                }
            });
        }
        unobserve() {}
        disconnect() { this._callback = null; }
    };
    globalThis.ResizeObserver = ResizeObserverPolyfill;
    w.ResizeObserver = ResizeObserverPolyfill;

    // IntersectionObserver polyfill (jsdom doesn't implement it)
    if (!globalThis.IntersectionObserver) {
        saved.IntersectionObserver = globalThis.IntersectionObserver;
        globalThis.IntersectionObserver = class IntersectionObserver {
            constructor() {}
            observe() {}
            unobserve() {}
            disconnect() {}
        };
    }
    if (!w.IntersectionObserver) {
        w.IntersectionObserver = globalThis.IntersectionObserver;
    }

    // DOMRect polyfill (jsdom doesn't implement it, needed by Spectrum ListView/Virtualizer)
    if (!globalThis.DOMRect) {
        saved.DOMRect = globalThis.DOMRect;
        globalThis.DOMRect = class DOMRect {
            constructor(x = 0, y = 0, width = 0, height = 0) {
                this.x = x;
                this.y = y;
                this.width = width;
                this.height = height;
                this.top = y;
                this.right = x + width;
                this.bottom = y + height;
                this.left = x;
            }
            toJSON() {
                return { x: this.x, y: this.y, width: this.width, height: this.height, top: this.top, right: this.right, bottom: this.bottom, left: this.left };
            }
        };
    }
    if (!w.DOMRect) {
        w.DOMRect = globalThis.DOMRect;
    }

    // CSS namespace polyfill (jsdom doesn't implement it, needed by @react-spectrum/tabs)
    if (!globalThis.CSS) {
        saved.CSS = globalThis.CSS;
        globalThis.CSS = {
            supports: () => false,
            escape: (s) => s.replace(/([^\w-])/g, '\\$1'),
        };
    }

    // getAnimations polyfill (jsdom doesn't implement it, needed by Spectrum LoadingSpinner)
    if (!w.document.getAnimations) {
        w.document.getAnimations = () => [];
    }
    if (!w.Element.prototype.getAnimations) {
        w.Element.prototype.getAnimations = () => [];
    }

    return {
        restore() {
            for (const [name, value] of Object.entries(saved)) {
                try {
                    if (value === undefined) delete globalThis[name];
                    else globalThis[name] = value;
                } catch (e) { /* read-only */ }
            }
        },
    };
}
