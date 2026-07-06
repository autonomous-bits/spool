import { execFile } from 'node:child_process';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { promisify } from 'node:util';
import { Pool } from 'pg';
import { loadDatabaseConfig } from '../../src/persistence/database-config.js';
import { runMigrations } from '../../src/persistence/migrator.js';

export interface TestDatabase {
  pool: Pool;
  close(): Promise<void>;
}

const execFileAsync = promisify(execFile);
const REPO_ROOT = resolve(dirname(fileURLToPath(import.meta.url)), '../../../..');
const DATABASE_READY_TIMEOUT_MS = 30_000;
const DATABASE_RETRY_INTERVAL_MS = 1_000;

let databaseBootstrapPromise: Promise<void> | undefined;

async function delay(milliseconds: number): Promise<void> {
  await new Promise((resolveDelay) => {
    setTimeout(resolveDelay, milliseconds);
  });
}

async function assertDatabaseReachable(): Promise<void> {
  const pool = new Pool({
    ...loadDatabaseConfig(),
    max: 1,
    connectionTimeoutMillis: DATABASE_RETRY_INTERVAL_MS,
  });

  try {
    const client = await pool.connect();
    client.release();
  } finally {
    await pool.end();
  }
}

async function ensureContainerizedPostgres(): Promise<void> {
  if (databaseBootstrapPromise !== undefined) {
    await databaseBootstrapPromise;
    return;
  }

  databaseBootstrapPromise = (async () => {
    let composeStartupError: unknown;

    try {
      await assertDatabaseReachable();
      return;
    } catch {
      try {
        await execFileAsync('docker', ['compose', 'up', '-d', 'postgres'], {
          cwd: REPO_ROOT,
        });
      } catch (error) {
        composeStartupError = error;
      }
    }

    const deadline = Date.now() + DATABASE_READY_TIMEOUT_MS;
    let lastError: unknown;

    while (Date.now() < deadline) {
      try {
        await assertDatabaseReachable();
        return;
      } catch (error) {
        lastError = error;
        await delay(DATABASE_RETRY_INTERVAL_MS);
      }
    }

    if (composeStartupError !== undefined) {
      throw composeStartupError instanceof Error
        ? composeStartupError
        : new Error(JSON.stringify(composeStartupError));
    }

    throw lastError instanceof Error ? lastError : new Error(String(lastError));
  })();

  try {
    await databaseBootstrapPromise;
  } catch (error) {
    databaseBootstrapPromise = undefined;
    throw error;
  }
}

/**
 * Builds a Pool against the containerized Postgres used for host-side integration tests
 * (compose's `postgres` service, published on localhost:5433 per config/store.env.example),
 * applies migrations, and returns the pool plus a close() to release it.
 */
export async function setUpTestDatabase(): Promise<TestDatabase> {
  await ensureContainerizedPostgres();
  const pool = new Pool(loadDatabaseConfig());
  await runMigrations(pool);

  return {
    pool,
    close: () => pool.end(),
  };
}
