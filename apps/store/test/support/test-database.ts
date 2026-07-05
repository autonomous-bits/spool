import { Pool } from 'pg';
import { loadDatabaseConfig } from '../../src/persistence/database-config.js';
import { runMigrations } from '../../src/persistence/migrator.js';

export interface TestDatabase {
  pool: Pool;
  close(): Promise<void>;
}

/**
 * Builds a Pool against the containerized Postgres used for host-side integration tests
 * (compose's `postgres` service, published on localhost:5433 per config/store.env.example),
 * applies migrations, and returns the pool plus a close() to release it.
 */
export async function setUpTestDatabase(): Promise<TestDatabase> {
  const pool = new Pool(loadDatabaseConfig());
  await runMigrations(pool);

  return {
    pool,
    close: () => pool.end(),
  };
}
