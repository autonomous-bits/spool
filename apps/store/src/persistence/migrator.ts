import { readFileSync, readdirSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import type { Pool } from 'pg';

const MIGRATIONS_DIR = join(dirname(fileURLToPath(import.meta.url)), 'migrations');

/**
 * A single advisory lock key for Spool's store migration runner, so concurrent app boots
 * (e.g. multiple container replicas) don't race applying the same migration twice.
 */
const MIGRATION_LOCK_KEY = 72_820_931;

function listMigrationFiles(): string[] {
  return readdirSync(MIGRATIONS_DIR)
    .filter((file) => file.endsWith('.sql'))
    .sort();
}

/**
 * Applies every pending .sql migration file in `migrations/`, in filename order, tracked in a
 * `schema_migrations` table. Idempotent: safe to call on every app boot and every test run
 * against the same database.
 */
export async function runMigrations(pool: Pool): Promise<void> {
  const client = await pool.connect();
  try {
    await client.query('SELECT pg_advisory_lock($1)', [MIGRATION_LOCK_KEY]);
    try {
      await client.query(`
        CREATE TABLE IF NOT EXISTS schema_migrations (
          filename VARCHAR(255) PRIMARY KEY,
          applied_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
        )
      `);

      const appliedResult = await client.query<{ filename: string }>(
        'SELECT filename FROM schema_migrations',
      );
      const applied = new Set(appliedResult.rows.map((row) => row.filename));

      for (const filename of listMigrationFiles()) {
        if (applied.has(filename)) {
          continue;
        }

        const sql = readFileSync(join(MIGRATIONS_DIR, filename), 'utf8');
        await client.query('BEGIN');
        try {
          await client.query(sql);
          await client.query('INSERT INTO schema_migrations (filename) VALUES ($1)', [filename]);
          await client.query('COMMIT');
        } catch (error) {
          await client.query('ROLLBACK');
          throw error;
        }
      }
    } finally {
      await client.query('SELECT pg_advisory_unlock($1)', [MIGRATION_LOCK_KEY]);
    }
  } finally {
    client.release();
  }
}
