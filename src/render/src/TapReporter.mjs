/**
 * TapReporter - Produces TAP (Test Anything Protocol) output for CI consumption.
 *
 * TAP is understood by most CI systems, test aggregators, and can be parsed
 * by tools like tap-parser, tap-mocha-reporter, etc.
 *
 * Usage:
 *   const tap = new TapReporter();
 *   tap.pass('renders without errors');
 *   tap.fail('has 5 columns', new Error('expected 5, got 3'));
 *   tap.skip('ticking table support');
 *   tap.done(); // prints TAP summary and exits
 */

export class TapReporter {
    constructor(options = {}) {
        this._count = 0;
        this._passed = 0;
        this._failed = 0;
        this._skipped = 0;
        this._results = [];
        this._output = options.output || process.stdout;
        this._started = false;
    }

    /**
     * Print the TAP version header. Called automatically on first test.
     */
    _ensureStarted() {
        if (!this._started) {
            this._started = true;
            this._write('TAP version 13');
        }
    }

    /**
     * Record a passing test.
     * @param {string} description - Test description
     */
    pass(description) {
        this._ensureStarted();
        this._count++;
        this._passed++;
        this._write(`ok ${this._count} - ${description}`);
        this._results.push({ ok: true, description });
    }

    /**
     * Record a failing test.
     * @param {string} description - Test description
     * @param {Error|string} [error] - Error or message
     */
    fail(description, error) {
        this._ensureStarted();
        this._count++;
        this._failed++;
        this._write(`not ok ${this._count} - ${description}`);
        if (error) {
            const msg = error instanceof Error ? error.message : String(error);
            this._write('  ---');
            this._write(`  message: ${msg}`);
            if (error instanceof Error && error.stack) {
                const stackLines = error.stack.split('\n').slice(1, 4);
                this._write('  stack: |');
                for (const line of stackLines) {
                    this._write(`    ${line.trim()}`);
                }
            }
            this._write('  ...');
        }
        this._results.push({ ok: false, description, error });
    }

    /**
     * Record a skipped test.
     * @param {string} description - Test description
     * @param {string} [reason] - Reason for skipping
     */
    skip(description, reason) {
        this._ensureStarted();
        this._count++;
        this._skipped++;
        const directive = reason ? `# SKIP ${reason}` : '# SKIP';
        this._write(`ok ${this._count} - ${description} ${directive}`);
        this._results.push({ ok: true, description, skipped: true });
    }

    /**
     * Run an assertion and record pass/fail.
     * @param {string} description - Test description
     * @param {function} fn - Function that throws on failure
     */
    test(description, fn) {
        try {
            fn();
            this.pass(description);
        } catch (e) {
            this.fail(description, e);
        }
    }

    /**
     * Run an async assertion and record pass/fail.
     * @param {string} description - Test description
     * @param {function} fn - Async function that throws on failure
     */
    async testAsync(description, fn) {
        try {
            await fn();
            this.pass(description);
        } catch (e) {
            this.fail(description, e);
        }
    }

    /**
     * Print the plan and summary, optionally exit.
     * @param {object} [options]
     * @param {boolean} [options.exit=false] - Whether to call process.exit
     * @returns {{ total: number, passed: number, failed: number, skipped: number }}
     */
    done(options = {}) {
        this._ensureStarted();
        this._write(`1..${this._count}`);
        this._write('');
        this._write(`# tests ${this._count}`);
        this._write(`# pass  ${this._passed}`);
        if (this._failed > 0) this._write(`# fail  ${this._failed}`);
        if (this._skipped > 0) this._write(`# skip  ${this._skipped}`);

        const summary = {
            total: this._count,
            passed: this._passed,
            failed: this._failed,
            skipped: this._skipped,
        };

        if (options.exit !== false) {
            process.exit(this._failed > 0 ? 1 : 0);
        }

        return summary;
    }

    /**
     * Get current counts without finalizing.
     */
    get counts() {
        return {
            total: this._count,
            passed: this._passed,
            failed: this._failed,
            skipped: this._skipped,
        };
    }

    _write(line) {
        this._output.write(line + '\n');
    }
}
