import { readFileSync, readdirSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import type { Pool, PoolClient } from 'pg';
import {
  BOOTSTRAP_STAKEHOLDER_ID,
  DEFAULT_BOOTSTRAP_STAKEHOLDER_EMAIL,
  DEFAULT_BOOTSTRAP_STAKEHOLDER_NAME,
} from './bootstrap-stakeholder.js';
import {
  OAUTH_E2E_FIXTURE_DISCIPLINE,
  OAUTH_E2E_FIXTURE_GITHUB_LOGIN,
  OAUTH_E2E_FIXTURE_STAKEHOLDER_ID,
} from './oauth-e2e-fixture-stakeholder.js';

const MIGRATIONS_DIR = join(dirname(fileURLToPath(import.meta.url)), 'migrations');
const DEFAULT_WORKSPACE_ID = '00000000-0000-0000-0000-00000000d0fa';
const DEFAULT_WORKSPACE_NAME = 'Default Workspace';

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
 * Name/email for the bootstrap stakeholder (role='system'), sourced from
 * `ADMIN_STAKEHOLDER_NAME`/`ADMIN_STAKEHOLDER_EMAIL` when set — see
 * `tools/docker/seed-admin-env.sh`, which populates these from the host's `git config
 * user.name`/`user.email` before `docker compose up`, so the bootstrap stakeholder seeded on
 * first boot reflects a real, known identity instead of the generic placeholder. Falls back to
 * the documented placeholder defaults otherwise (e.g. unit tests, CI).
 */
function resolveBootstrapStakeholderIdentity(env: NodeJS.ProcessEnv): {
  name: string;
  email: string;
} {
  const name = env.ADMIN_STAKEHOLDER_NAME?.trim() || DEFAULT_BOOTSTRAP_STAKEHOLDER_NAME;
  const email = env.ADMIN_STAKEHOLDER_EMAIL?.trim() || DEFAULT_BOOTSTRAP_STAKEHOLDER_EMAIL;
  return { name, email };
}

async function ensureBaselineSeedData(client: PoolClient): Promise<void> {
  const { name: bootstrapStakeholderName, email: bootstrapStakeholderEmail } =
    resolveBootstrapStakeholderIdentity(process.env);

  await client.query(
    `INSERT INTO stakeholders (id, name, email, role, discipline)
     VALUES ($1, 'Bootstrap Stakeholder', 'bootstrap-stakeholder@spool.local', 'system', NULL)
     ON CONFLICT (id) DO NOTHING`,
    [BOOTSTRAP_STAKEHOLDER_ID],
  );

  // Migration 0002 always seeds the bootstrap stakeholder with the placeholder name/email (a
  // later migration's FK needs the row to exist by then). If ADMIN_STAKEHOLDER_NAME/EMAIL are
  // set, adopt them here by updating the row in place — but only while it still holds the
  // untouched placeholder email, so a real admin identity set on first boot is never clobbered
  // by this running again on every subsequent boot.
  if (bootstrapStakeholderEmail !== DEFAULT_BOOTSTRAP_STAKEHOLDER_EMAIL) {
    await client.query(
      `UPDATE stakeholders SET name = $2, email = $3
       WHERE id = $1 AND email = $4`,
      [
        BOOTSTRAP_STAKEHOLDER_ID,
        bootstrapStakeholderName,
        bootstrapStakeholderEmail,
        DEFAULT_BOOTSTRAP_STAKEHOLDER_EMAIL,
      ],
    );
  }

  await client.query(
    `INSERT INTO stakeholders (id, name, email, role, discipline, github_login)
     VALUES ($1, 'OAuth E2E Fixture Stakeholder', 'oauth-e2e-fixture-stakeholder@spool.local', 'stakeholder', $2, $3)
     ON CONFLICT (id) DO NOTHING`,
    [
      OAUTH_E2E_FIXTURE_STAKEHOLDER_ID,
      OAUTH_E2E_FIXTURE_DISCIPLINE,
      OAUTH_E2E_FIXTURE_GITHUB_LOGIN,
    ],
  );

  await client.query(
    `INSERT INTO workspaces (id, name, created_by_stakeholder_id)
     VALUES ($1, $2, $3)
     ON CONFLICT (id) DO NOTHING`,
    [DEFAULT_WORKSPACE_ID, DEFAULT_WORKSPACE_NAME, BOOTSTRAP_STAKEHOLDER_ID],
  );

  await client.query(
    `INSERT INTO workspace_memberships (workspace_id, stakeholder_id)
     SELECT $1, seeded.stakeholder_id
       FROM (
         VALUES ($2::uuid), ($3::uuid)
       ) AS seeded(stakeholder_id)
     ON CONFLICT DO NOTHING`,
    [
      DEFAULT_WORKSPACE_ID,
      BOOTSTRAP_STAKEHOLDER_ID,
      OAUTH_E2E_FIXTURE_STAKEHOLDER_ID,
    ],
  );
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

      await ensureBaselineSeedData(client);
    } finally {
      await client.query('SELECT pg_advisory_unlock($1)', [MIGRATION_LOCK_KEY]);
    }
  } finally {
    client.release();
  }
}
