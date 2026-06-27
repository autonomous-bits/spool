import { defineProject } from 'vitest/config';

export default defineProject({
  resolve: {
    tsconfigPaths: true,
  },
  test: {
    name: 'mcp',
    environment: 'node',
    include: ['src/**/*.{spec,test}.ts'],
    pool: 'forks',
    isolate: true,
    globals: false,
  },
});
