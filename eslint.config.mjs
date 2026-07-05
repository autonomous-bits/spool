// @ts-check
import js from '@eslint/js';
import { defineConfig } from 'eslint/config';
import tseslint from 'typescript-eslint';
import eslintConfigPrettier from 'eslint-config-prettier/flat';

export default defineConfig({
  ignores: ['**/dist/**', '**/node_modules/**', '.github/extensions/**'],
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
});
