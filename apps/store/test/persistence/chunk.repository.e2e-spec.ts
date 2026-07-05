import type { Pool } from 'pg';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { Chunk } from '../../src/domain/chunk.js';
import { ChunkRepository } from '../../src/persistence/chunk.repository.js';
import { BOOTSTRAP_STAKEHOLDER_ID } from '../../src/persistence/bootstrap-stakeholder.js';
import { setUpTestDatabase, type TestDatabase } from '../support/test-database.js';

function buildChunk(overrides: Partial<ConstructorParameters<typeof Chunk>[0]> = {}): Chunk {
  return new Chunk({
    label: `chunk-${Math.random().toString(36).slice(2, 10)}`,
    content: 'Some atomic idea content.',
    discipline: 'engineering',
    chunkType: 'feature',
    contextKind: 'permanent',
    createdByStakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
    ...overrides,
  });
}

describe('ChunkRepository (containerized Postgres)', () => {
  let database: TestDatabase;
  let pool: Pool;
  let repository: ChunkRepository;

  beforeAll(async () => {
    database = await setUpTestDatabase();
    pool = database.pool;
    repository = new ChunkRepository(pool);
  });

  afterAll(async () => {
    await database.close();
  });

  it('create persists a chunk with status draft, branch_id NULL, and chunk_type/context_kind populated', async () => {
    const chunk = buildChunk();

    const created = await repository.create(chunk);

    expect(created.status).toBe('draft');

    const row = await pool.query<{
      branch_id: string | null;
      origin_branch_id: string | null;
      status: string;
      chunk_type: string;
      context_kind: string;
    }>(
      'SELECT branch_id, origin_branch_id, status, chunk_type, context_kind FROM chunks WHERE id = $1',
      [created.id],
    );

    expect(row.rows).toHaveLength(1);
    expect(row.rows[0]?.branch_id).toBeNull();
    expect(row.rows[0]?.origin_branch_id).toBeNull();
    expect(row.rows[0]?.status).toBe('draft');
    expect(row.rows[0]?.chunk_type).toBe('feature');
    expect(row.rows[0]?.context_kind).toBe('permanent');
  });

  it('findById round-trips every field exactly for a persisted chunk', async () => {
    const chunk = buildChunk({
      label: `roundtrip-${Math.random().toString(36).slice(2, 10)}`,
      content: 'Round-trip content check.',
      discipline: 'security',
      chunkType: 'constraint',
      contextKind: 'transient',
    });

    const created = await repository.create(chunk);
    const found = await repository.findById(created.id);

    expect(found).toBeDefined();
    expect(found).toEqual(created);
  });

  it('findById returns an explicit not-found result (undefined) for an unknown id', async () => {
    const found = await repository.findById('00000000-0000-0000-0000-00000000dead');

    expect(found).toBeUndefined();
  });
});
