import swc from 'unplugin-swc';
import { defineProject } from 'vitest/config';

export default defineProject({
  plugins: [
    swc.vite({
      module: { type: 'es6' },
      jsc: {
        parser: { syntax: 'typescript', decorators: true },
        transform: {
          legacyDecorator: true,
          decoratorMetadata: true,
          useDefineForClassFields: false,
        },
        target: 'es2024',
        keepClassNames: true,
      },
      sourceMaps: true,
    }),
  ],
  oxc: false,
  resolve: {
    tsconfigPaths: true,
    extensionAlias: {
      '.js': ['.ts', '.js'],
      '.mjs': ['.mts', '.mjs'],
    },
    dedupe: ['@nestjs/common', '@nestjs/core', 'reflect-metadata', 'rxjs'],
  },
  test: {
    name: 'store',
    environment: 'node',
    setupFiles: ['./src/test/setup.ts'],
    include: ['src/**/*.{spec,test}.ts', 'test/**/*.{spec,test,e2e-spec}.ts'],
    pool: 'forks',
    isolate: true,
    globals: false,
    testTimeout: 10_000,
    hookTimeout: 10_000,
  },
});
