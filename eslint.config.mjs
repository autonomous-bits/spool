// @ts-check
import js from '@eslint/js';
import { defineConfig } from 'eslint/config';
import tseslint from 'typescript-eslint';
import eslintConfigPrettier from 'eslint-config-prettier/flat';

export default defineConfig([
  { ignores: ['**/dist/**', '**/node_modules/**', '.github/extensions/**'] },
  {
    files: ['**/*.{js,mjs,cjs,ts,mts,cts}'],
    extends: [
      js.configs.recommended,
      tseslint.configs.strictTypeChecked,
      tseslint.configs.stylisticTypeChecked,
      eslintConfigPrettier,
    ],
    languageOptions: {
      parserOptions: {
        project: [
          './tsconfig.eslint.json',
          './apps/store/tsconfig.eslint.json',
          './apps/mcp/tsconfig.eslint.json',
        ],
        tsconfigRootDir: import.meta.dirname,
      },
    },
    rules: {
      '@typescript-eslint/consistent-type-imports': 'error',
    },
  },
  {
    // Plain Node scripts (not application code): tools/, Docker-only test fixtures, and this
    // config file itself. These run directly under `node`, so they need Node's ambient globals
    // that TypeScript's own `no-undef` carve-out (for `.ts`/`.mts`/`.cts` files only) doesn't
    // cover for `.mjs`/`.cjs`.
    files: ['**/*.mjs', '**/*.cjs'],
    languageOptions: {
      globals: {
        process: 'readonly',
        console: 'readonly',
        fetch: 'readonly',
        URL: 'readonly',
        Buffer: 'readonly',
        globalThis: 'readonly',
      },
    },
  },
  {
    // supertest's `Response.body` (and Nest's `HttpService`/axios-style JSON bodies used in
    // tests) are inherently typed `any` with no way to parameterize them per-request. Every
    // `response.body.foo` access therefore trips the `no-unsafe-*` family even though the values
    // are runtime-checked by the assertions themselves. Relaxing these rules for test files only
    // reflects that inherent limitation rather than a real type-safety gap in application code.
    files: ['**/*.spec.ts', '**/*.e2e-spec.ts'],
    rules: {
      '@typescript-eslint/no-unsafe-member-access': 'off',
      '@typescript-eslint/no-unsafe-assignment': 'off',
      '@typescript-eslint/no-unsafe-argument': 'off',
      '@typescript-eslint/no-unsafe-return': 'off',
      '@typescript-eslint/no-unsafe-call': 'off',
    },
  },
]);
