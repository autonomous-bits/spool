import type { Pool } from 'pg';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { BOOTSTRAP_STAKEHOLDER_ID } from '../../src/persistence/bootstrap-stakeholder.js';
import { runMigrations } from '../../src/persistence/migrator.js';
import { setUpTestDatabase, type TestDatabase } from '../support/test-database.js';

describe('store migrations (containerized Postgres)', () => {
  let database: TestDatabase;
  let pool: Pool;

  beforeAll(async () => {
    database = await setUpTestDatabase();
    pool = database.pool;
  });

  afterAll(async () => {
    await database?.close();
  });

  it('creates the stakeholders table with the expected columns', async () => {
    const result = await pool.query<{ column_name: string }>(
      `SELECT column_name FROM information_schema.columns WHERE table_name = 'stakeholders'`,
    );
    const columns = result.rows.map((row) => row.column_name).sort();

    expect(columns).toEqual(
      ['created_at', 'discipline', 'email', 'id', 'name', 'role'].sort(),
    );
  });

  it('creates the chunks table with the expected columns including chunk_type/context_kind', async () => {
    const result = await pool.query<{ column_name: string }>(
      `SELECT column_name FROM information_schema.columns WHERE table_name = 'chunks'`,
    );
    const columns = result.rows.map((row) => row.column_name).sort();

    expect(columns).toEqual(
      [
        'branch_id',
        'chunk_type',
        'content',
        'context_kind',
        'created_at',
        'created_by_stakeholder_id',
        'discipline',
        'id',
        'label',
        'origin_branch_id',
        'status',
        'updated_at',
        'updated_by_stakeholder_id',
      ].sort(),
    );
  });

  it('creates the idx_chunks_draft_mainline unique partial index ratified by IDEA-78', async () => {
    const result = await pool.query<{ indexdef: string }>(
      `SELECT indexdef FROM pg_indexes WHERE indexname = 'idx_chunks_draft_mainline'`,
    );

    expect(result.rows).toHaveLength(1);
    const indexdef = result.rows[0]?.indexdef ?? '';
    expect(indexdef).toContain('UNIQUE INDEX idx_chunks_draft_mainline');
    expect(indexdef).toContain('branch_id IS NULL');
    expect(indexdef).toContain("(status)::text = 'draft'::text");
  });

  it('enforces the chunk_type/context_kind CHECK constraints', async () => {
    const constraints = await pool.query<{ conname: string }>(
      `SELECT conname FROM pg_constraint WHERE conrelid = 'chunks'::regclass AND contype = 'c'`,
    );
    const names = constraints.rows.map((row) => row.conname);

    expect(names.some((name) => name.includes('chunk_type'))).toBe(true);
    expect(names.some((name) => name.includes('context_kind'))).toBe(true);
  });

  it('seeds exactly one bootstrap stakeholder with the fixed documented UUID', async () => {
    const result = await pool.query<{ id: string }>(
      'SELECT id FROM stakeholders WHERE id = $1',
      [BOOTSTRAP_STAKEHOLDER_ID],
    );

    expect(result.rows).toHaveLength(1);
    expect(result.rows[0]?.id).toBe(BOOTSTRAP_STAKEHOLDER_ID);
  });

  it('is idempotent: re-running migrations does not duplicate the seed or fail', async () => {
    await runMigrations(pool);
    await runMigrations(pool);

    const stakeholderRows = await pool.query<{ count: string }>(
      'SELECT COUNT(*)::text AS count FROM stakeholders WHERE id = $1',
      [BOOTSTRAP_STAKEHOLDER_ID],
    );
    expect(stakeholderRows.rows[0]?.count).toBe('1');

    const migrationRows = await pool.query<{ count: string }>(
      'SELECT COUNT(*)::text AS count FROM schema_migrations',
    );
    expect(migrationRows.rows[0]?.count).toBe('4');
  });

  it('creates the edges table with the expected columns', async () => {
    const result = await pool.query<{ column_name: string }>(
      `SELECT column_name FROM information_schema.columns WHERE table_name = 'edges'`,
    );
    const columns = result.rows.map((row) => row.column_name).sort();

    expect(columns).toEqual(
      [
        'branch_id',
        'created_at',
        'created_by_stakeholder_id',
        'discipline',
        'from_chunk_label',
        'id',
        'origin_branch_id',
        'status',
        'superseded_by_edge_id',
        'to_chunk_label',
        'type',
        'updated_at',
        'updated_by_stakeholder_id',
      ].sort(),
    );
  });

  it('creates the idx_edges_branch_lookup, idx_edges_mainline, and idx_edges_branch_active indexes per the authoritative schema', async () => {
    const result = await pool.query<{ indexname: string; indexdef: string }>(
      `SELECT indexname, indexdef FROM pg_indexes WHERE tablename = 'edges'`,
    );
    const byName = new Map(result.rows.map((row) => [row.indexname, row.indexdef]));

    expect(byName.has('idx_edges_branch_lookup')).toBe(true);
    expect(byName.get('idx_edges_branch_lookup')).toContain(
      '(branch_id, from_chunk_label, to_chunk_label)',
    );

    expect(byName.has('idx_edges_mainline')).toBe(true);
    const mainlineDef = byName.get('idx_edges_mainline') ?? '';
    expect(mainlineDef).toContain('UNIQUE INDEX idx_edges_mainline');
    expect(mainlineDef).toContain('branch_id IS NULL');
    expect(mainlineDef).toContain("(status)::text = 'active'::text");

    expect(byName.has('idx_edges_branch_active')).toBe(true);
    const branchActiveDef = byName.get('idx_edges_branch_active') ?? '';
    expect(branchActiveDef).toContain('UNIQUE INDEX idx_edges_branch_active');
    expect(branchActiveDef).toContain("(status)::text = 'active'::text");
  });
});
