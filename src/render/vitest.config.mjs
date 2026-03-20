import { defineConfig } from 'vitest/config';

export default defineConfig({
  test: {
    globals: true,
    environment: 'node',
    pool: 'forks',
    poolOptions: {
      forks: {
        execArgv: ['--import', './src/css-loader.mjs'],
      },
    },
    projects: [
      {
        test: {
          name: 'unit',
          include: ['tests/unit/**/*.test.mjs'],
          setupFiles: ['tests/setup/unit-setup.mjs'],
        },
      },
      {
        test: {
          name: 'cli',
          include: ['tests/cli/**/*.test.mjs'],
          setupFiles: ['tests/setup/cli-setup.mjs'],
          testTimeout: 30000,
        },
      },
      {
        test: {
          name: 'integration',
          // Component interaction tests and error handling tests
          include: [
            'tests/integration/components/**/*.test.mjs',
            'tests/integration/errors/**/*.test.mjs',
          ],
          setupFiles: ['tests/setup/integration-setup.mjs'],
          globalSetup: ['tests/setup/integration-global-setup.mjs'],
          testTimeout: 60000,
          hookTimeout: 30000,
          // Integration tests share a CLI daemon — must run sequentially
          poolOptions: {
            forks: {
              singleFork: true,
            },
          },
        },
      },
      {
        test: {
          name: 'snapshots',
          include: ['tests/snapshots/**/*.test.mjs'],
          setupFiles: ['tests/setup/snapshot-setup.mjs'],
          testTimeout: 120000,
          hookTimeout: 30000,
          // No singleFork — test files run in parallel across forks,
          // each test starts/stops its own DH server
        },
      },
    ],
  },
});
