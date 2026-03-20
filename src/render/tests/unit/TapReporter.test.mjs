import { describe, it, expect } from 'vitest';
import { TapReporter } from '../../src/TapReporter.mjs';

function createMockOutput() {
    const lines = [];
    return {
        write: (line) => lines.push(line),
        lines,
        text: () => lines.join(''),
    };
}

describe('pass', () => {
    it('outputs TAP version header on first test', () => {
        const out = createMockOutput();
        const tap = new TapReporter({ output: out });
        tap.pass('first test');
        expect(out.lines[0]).toBe('TAP version 13\n');
    });

    it('outputs "ok N - description"', () => {
        const out = createMockOutput();
        const tap = new TapReporter({ output: out });
        tap.pass('my passing test');
        expect(out.lines[1]).toBe('ok 1 - my passing test\n');
    });
});

describe('fail', () => {
    it('outputs "not ok N - description"', () => {
        const out = createMockOutput();
        const tap = new TapReporter({ output: out });
        tap.fail('my failing test');
        expect(out.lines[1]).toBe('not ok 1 - my failing test\n');
    });

    it('includes error message in YAML block', () => {
        const out = createMockOutput();
        const tap = new TapReporter({ output: out });
        tap.fail('broken test', new Error('something went wrong'));
        const text = out.text();
        expect(text).toContain('  ---\n');
        expect(text).toContain('  message: something went wrong\n');
        expect(text).toContain('  ...\n');
    });
});

describe('skip', () => {
    it('outputs "ok N - description # SKIP"', () => {
        const out = createMockOutput();
        const tap = new TapReporter({ output: out });
        tap.skip('skipped test');
        expect(out.lines[1]).toContain('# SKIP');
        expect(out.lines[1]).toContain('ok 1 - skipped test');
    });

    it('includes reason when given', () => {
        const out = createMockOutput();
        const tap = new TapReporter({ output: out });
        tap.skip('skipped test', 'not implemented');
        expect(out.lines[1]).toContain('# SKIP not implemented');
    });
});

describe('test', () => {
    it('records pass when fn succeeds', () => {
        const out = createMockOutput();
        const tap = new TapReporter({ output: out });
        tap.test('sync pass', () => { /* no throw */ });
        expect(tap.counts.passed).toBe(1);
        expect(tap.counts.failed).toBe(0);
    });

    it('records fail when fn throws', () => {
        const out = createMockOutput();
        const tap = new TapReporter({ output: out });
        tap.test('sync fail', () => { throw new Error('boom'); });
        expect(tap.counts.failed).toBe(1);
        expect(tap.counts.passed).toBe(0);
    });
});

describe('testAsync', () => {
    it('records pass for resolved async fn', async () => {
        const out = createMockOutput();
        const tap = new TapReporter({ output: out });
        await tap.testAsync('async pass', async () => { /* resolves */ });
        expect(tap.counts.passed).toBe(1);
        expect(tap.counts.failed).toBe(0);
    });

    it('records fail for rejected async fn', async () => {
        const out = createMockOutput();
        const tap = new TapReporter({ output: out });
        await tap.testAsync('async fail', async () => { throw new Error('async boom'); });
        expect(tap.counts.failed).toBe(1);
        expect(tap.counts.passed).toBe(0);
    });
});

describe('done', () => {
    it('outputs plan line "1..N"', () => {
        const out = createMockOutput();
        const tap = new TapReporter({ output: out });
        tap.pass('test one');
        tap.pass('test two');
        tap.done({ exit: false });
        const text = out.text();
        expect(text).toContain('1..2\n');
    });

    it('outputs summary with counts', () => {
        const out = createMockOutput();
        const tap = new TapReporter({ output: out });
        tap.pass('passes');
        tap.fail('fails', new Error('oops'));
        tap.skip('skips');
        tap.done({ exit: false });
        const text = out.text();
        expect(text).toContain('# tests 3');
        expect(text).toContain('# pass  1');
        expect(text).toContain('# fail  1');
        expect(text).toContain('# skip  1');
    });

    it('returns summary object', () => {
        const out = createMockOutput();
        const tap = new TapReporter({ output: out });
        tap.pass('a');
        tap.fail('b');
        const summary = tap.done({ exit: false });
        expect(summary).toEqual({
            total: 2,
            passed: 1,
            failed: 1,
            skipped: 0,
        });
    });
});

describe('counts', () => {
    it('returns current counts without finalizing', () => {
        const out = createMockOutput();
        const tap = new TapReporter({ output: out });
        tap.pass('one');
        tap.pass('two');
        tap.fail('three');
        const c = tap.counts;
        expect(c).toEqual({ total: 3, passed: 2, failed: 1, skipped: 0 });
        // Verify done was not called (no plan line yet)
        const text = out.text();
        expect(text).not.toContain('1..');
    });
});
