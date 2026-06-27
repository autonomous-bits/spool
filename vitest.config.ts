import { defineConfig } from 'vitest/config';

export default defineConfig({
  test: {
    projects: ['apps/store', 'apps/mcp'],
    passWithNoTests: true,
    reporters: process.env.CI
      ? [
          'github-actions',
          ['junit', { outputFile: './test-results/junit.xml' }],
          'default',
        ]
      : ['default'],
    coverage: {
      provider: 'v8',
      enabled: false,
      include: ['apps/*/src/**/*.ts'],
      exclude: [
        '**/node_modules/**',
        '**/dist/**',
        '**/*.spec.ts',
        '**/*.test.ts',
        '**/main.ts',
        '**/*.module.ts',
      ],
      reporter: ['text', 'lcov', 'json-summary', 'html'],
      reportsDirectory: './coverage',
      thresholds: {
        lines: 80,
        functions: 80,
        branches: 75,
        statements: 80,
      },
    },
  },
});
