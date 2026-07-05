import type { PoolConfig } from 'pg';

/**
 * Builds a `pg` Pool config from STORE_DB_* environment variables. These names match the
 * `spoolstore` service's Docker Compose environment (compose.yaml) and
 * config/store.env.example, used for host-side integration tests against the compose
 * `postgres` service published on localhost:5433.
 */
export function loadDatabaseConfig(env: NodeJS.ProcessEnv = process.env): PoolConfig {
  const host = env.STORE_DB_HOST ?? 'localhost';
  const port = Number.parseInt(env.STORE_DB_PORT ?? '5433', 10);
  const user = env.STORE_DB_USER ?? 'spool';
  const password = env.STORE_DB_PASSWORD ?? 'spool_dev';
  const database = env.STORE_DB_NAME ?? 'spool';

  return {
    host,
    port,
    user,
    password,
    database,
  } satisfies PoolConfig;
}
