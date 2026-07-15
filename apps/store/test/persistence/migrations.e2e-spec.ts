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
    await database.close();
  });

  it('creates the stakeholders table with the expected columns', async () => {
    const result = await pool.query<{ column_name: string }>(
      `SELECT column_name FROM information_schema.columns WHERE table_name = 'stakeholders'`,
    );
    const columns = result.rows.map((row) => row.column_name).sort();

    expect(columns).toEqual(
      ['created_at', 'discipline', 'email', 'github_login', 'id', 'name', 'role'].sort(),
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
        'workspace_id',
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
    expect(migrationRows.rows[0]?.count).toBe('19');
  });

  it('creates the suggestions table with the expected columns', async () => {
    const result = await pool.query<{ column_name: string }>(
      `SELECT column_name FROM information_schema.columns WHERE table_name = 'suggestions'`,
    );
    const columns = result.rows.map((row) => row.column_name).sort();

    expect(columns).toEqual(
      [
        'content',
        'created_at',
        'decided_at',
        'decided_by_stakeholder_id',
        'discipline',
        'from_chunk_label',
        'id',
        'label',
        'relationship_type',
        'status',
        'submitted_by_actor_kind',
        'submitted_by_stakeholder_id',
        'to_chunk_label',
        'updated_at',
        'workspace_id',
      ].sort(),
    );
  });

  it('creates the suggestion_state_logs table with the expected columns', async () => {
    const result = await pool.query<{ column_name: string }>(
      `SELECT column_name FROM information_schema.columns WHERE table_name = 'suggestion_state_logs'`,
    );
    const columns = result.rows.map((row) => row.column_name).sort();

    expect(columns).toEqual(
      [
        'created_at',
        'id',
        'new_status',
        'old_status',
        'suggestion_id',
        'updated_by_stakeholder_id',
      ].sort(),
    );
  });

  it('enforces the check_suggestion_type CHECK constraint and idx_suggestions_unique index', async () => {
    const constraints = await pool.query<{ conname: string }>(
      `SELECT conname FROM pg_constraint WHERE conrelid = 'suggestions'::regclass AND contype = 'c'`,
    );
    const names = constraints.rows.map((row) => row.conname);
    expect(names).toContain('check_suggestion_type');

    const indexes = await pool.query<{ indexname: string }>(
      `SELECT indexname FROM pg_indexes WHERE tablename = 'suggestions'`,
    );
    expect(indexes.rows.map((row) => row.indexname)).toContain('idx_suggestions_unique');
  });

  it('adds the branches_origin_suggestion_id_fkey FK constraint targeting suggestions(id)', async () => {
    const result = await pool.query<{ confrelid: string }>(
      `SELECT confrelid::regclass::text AS confrelid
         FROM pg_constraint
        WHERE conname = 'branches_origin_suggestion_id_fkey'`,
    );

    expect(result.rows).toHaveLength(1);
    expect(result.rows[0]?.confrelid).toBe('suggestions');
  });

  it('creates the branches table with the expected columns', async () => {
    const result = await pool.query<{ column_name: string }>(
      `SELECT column_name FROM information_schema.columns WHERE table_name = 'branches'`,
    );
    const columns = result.rows.map((row) => row.column_name).sort();

    expect(columns).toEqual(
      [
        'created_at',
        'created_by_stakeholder_id',
        'discipline',
        'diverged_at',
        'id',
        'merged_at',
        'merged_by_stakeholder_id',
        'name',
        'origin_suggestion_id',
        'status',
        'submitted_at',
        'updated_at',
        'verified_at',
        'workspace_id',
      ].sort(),
    );
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
        'workspace_id',
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
      '(workspace_id, branch_id, from_chunk_label, to_chunk_label)',
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

  it('creates the artifacts table with the expected columns', async () => {
    const result = await pool.query<{ column_name: string }>(
      `SELECT column_name FROM information_schema.columns WHERE table_name = 'artifacts'`,
    );
    const columns = result.rows.map((row) => row.column_name).sort();

    expect(columns).toEqual(
      ['created_at', 'created_by_stakeholder_id', 'id', 'mime_type', 'uri', 'workspace_id'].sort(),
    );
  });

  it('creates the chunk_artifacts table with the expected columns', async () => {
    const result = await pool.query<{ column_name: string }>(
      `SELECT column_name FROM information_schema.columns WHERE table_name = 'chunk_artifacts'`,
    );
    const columns = result.rows.map((row) => row.column_name).sort();

    expect(columns).toEqual(
      [
        'artifact_id',
        'branch_id',
        'chunk_label',
        'created_at',
        'created_by_stakeholder_id',
        'id',
        'origin_branch_id',
        'status',
        'updated_at',
        'updated_by_stakeholder_id',
        'workspace_id',
      ].sort(),
    );
  });

  it('creates idx_chunk_artifacts_branch_lookup and idx_chunk_artifacts_mainline (IDEA-64)', async () => {
    const result = await pool.query<{ indexname: string; indexdef: string }>(
      `SELECT indexname, indexdef FROM pg_indexes WHERE tablename = 'chunk_artifacts'`,
    );
    const byName = new Map(result.rows.map((row) => [row.indexname, row.indexdef]));

    expect(byName.has('idx_chunk_artifacts_branch_lookup')).toBe(true);
    expect(byName.get('idx_chunk_artifacts_branch_lookup')).toContain(
      '(workspace_id, branch_id, chunk_label, artifact_id)',
    );

    expect(byName.has('idx_chunk_artifacts_mainline')).toBe(true);
    const mainlineDef = byName.get('idx_chunk_artifacts_mainline') ?? '';
    expect(mainlineDef).toContain('UNIQUE INDEX idx_chunk_artifacts_mainline');
    expect(mainlineDef).toContain('branch_id IS NULL');
    expect(mainlineDef).toContain("(status)::text = 'active'::text");
  });

  it('creates the verification_signals table with the expected columns', async () => {
    const result = await pool.query<{ column_name: string }>(
      `SELECT column_name FROM information_schema.columns WHERE table_name = 'verification_signals'`,
    );
    const columns = result.rows.map((row) => row.column_name).sort();

    expect(columns).toEqual(
      [
        'id',
        'branch_id',
        'verifier_name',
        'status',
        'reason',
        'created_at',
        'workspace_id',
        'reported_by_stakeholder_id',
      ].sort(),
    );
  });

  it('enforces the verification_signals status CHECK constraint', async () => {
    const constraints = await pool.query<{ conname: string }>(
      `SELECT conname FROM pg_constraint WHERE conrelid = 'verification_signals'::regclass AND contype = 'c'`,
    );
    expect(constraints.rows.length).toBeGreaterThan(0);
  });

  it('creates the feedback_notifications table with the expected columns', async () => {
    const result = await pool.query<{ column_name: string }>(
      `SELECT column_name FROM information_schema.columns WHERE table_name = 'feedback_notifications'`,
    );
    const columns = result.rows.map((row) => row.column_name).sort();

    expect(columns).toEqual(
      ['id', 'branch_id', 'stakeholder_id', 'signal_id', 'status', 'created_at', 'updated_at', 'workspace_id'].sort(),
    );
  });

  it('creates idx_feedback_notifications_stakeholder on stakeholder_id and status', async () => {
    const result = await pool.query<{ indexname: string; indexdef: string }>(
      `SELECT indexname, indexdef FROM pg_indexes WHERE tablename = 'feedback_notifications'`,
    );
    const byName = new Map(result.rows.map((row) => [row.indexname, row.indexdef]));

    expect(byName.has('idx_feedback_notifications_stakeholder')).toBe(true);
    expect(byName.get('idx_feedback_notifications_stakeholder')).toContain(
      '(workspace_id, stakeholder_id, status)',
    );
  });

  it('creates the refresh_tokens table with the expected columns', async () => {
    const result = await pool.query<{ column_name: string }>(
      `SELECT column_name FROM information_schema.columns WHERE table_name = 'refresh_tokens'`,
    );
    const columns = result.rows.map((row) => row.column_name).sort();

    expect(columns).toEqual(
      [
        'created_at',
        'expires_at',
        'id',
        'replaced_by_id',
        'revoked_at',
        'stakeholder_id',
        'token_hash',
        'workspace_id',
      ].sort(),
    );
  });

  it('creates refresh_tokens indexes and foreign keys for stakeholder, workspace, and replacement chaining', async () => {
    const indexes = await pool.query<{ indexname: string; indexdef: string }>(
      `SELECT indexname, indexdef FROM pg_indexes WHERE tablename = 'refresh_tokens'`,
    );
    const byName = new Map(indexes.rows.map((row) => [row.indexname, row.indexdef]));

    expect(byName.has('idx_refresh_tokens_token_hash')).toBe(true);
    expect(byName.get('idx_refresh_tokens_token_hash')).toContain('(token_hash)');
    expect(byName.has('idx_refresh_tokens_stakeholder')).toBe(true);
    expect(byName.get('idx_refresh_tokens_stakeholder')).toContain('(stakeholder_id)');

    const constraints = await pool.query<{ conname: string; confrelid: string }>(
      `SELECT conname, confrelid::regclass::text AS confrelid
         FROM pg_constraint
        WHERE conrelid = 'refresh_tokens'::regclass
          AND contype = 'f'`,
    );
    const foreignKeys = new Map(constraints.rows.map((row) => [row.conname, row.confrelid]));

    expect(foreignKeys.get('refresh_tokens_stakeholder_id_fkey')).toBe('stakeholders');
    expect(foreignKeys.get('refresh_tokens_workspace_id_fkey')).toBe('workspaces');
    expect(foreignKeys.get('refresh_tokens_replaced_by_id_fkey')).toBe('refresh_tokens');
  });

  it('creates the pairing_codes table with the expected columns and lookup index', async () => {
    const columnsResult = await pool.query<{ column_name: string }>(
      `SELECT column_name FROM information_schema.columns WHERE table_name = 'pairing_codes'`,
    );
    const columns = columnsResult.rows.map((row) => row.column_name).sort();

    expect(columns).toEqual(
      ['code_hash', 'consumed_at', 'expires_at', 'id', 'refresh_token', 'session_token'].sort(),
    );

    const indexes = await pool.query<{ indexname: string; indexdef: string }>(
      `SELECT indexname, indexdef FROM pg_indexes WHERE tablename = 'pairing_codes'`,
    );
    const byName = new Map(indexes.rows.map((row) => [row.indexname, row.indexdef]));

    expect(byName.has('idx_pairing_codes_code_hash')).toBe(true);
    expect(byName.get('idx_pairing_codes_code_hash')).toContain('(code_hash)');
  });
});
