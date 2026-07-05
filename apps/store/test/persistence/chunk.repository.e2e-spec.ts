import { setTimeout as delay } from 'node:timers/promises';
import { ConflictException } from '@nestjs/common';
import type { Pool } from 'pg';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { Branch } from '../../src/domain/branch.js';
import { Chunk } from '../../src/domain/chunk.js';
import { BranchRepository } from '../../src/persistence/branch.repository.js';
import { ChunkRepository } from '../../src/persistence/chunk.repository.js';
import { BOOTSTRAP_STAKEHOLDER_ID } from '../../src/persistence/bootstrap-stakeholder.js';
import { setUpTestDatabase, type TestDatabase } from '../support/test-database.js';

function buildBranch(overrides: Partial<ConstructorParameters<typeof Branch>[0]> = {}): Branch {
  return new Branch({
    name: `branch-${Math.random().toString(36).slice(2, 10)}`,
    discipline: 'engineering',
    createdByStakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
    ...overrides,
  });
}

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
  let branchRepository: BranchRepository;

  beforeAll(async () => {
    database = await setUpTestDatabase();
    pool = database.pool;
    repository = new ChunkRepository(pool);
    branchRepository = new BranchRepository(pool);
  });

  afterAll(async () => {
    await database?.close();
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

  it('rejects a branch-scoped create when the branch has already been submitted', async () => {
    const branch = await branchRepository.create(buildBranch());
    await branchRepository.submit(branch.id);

    await expect(
      repository.create(buildChunk({ branchId: branch.id, originBranchId: branch.id })),
    ).rejects.toThrowError(new ConflictException(`Branch ${branch.id} is not in draft status`));
  });

  it('serializes a racing submit and branch-scoped create so the create wins and submit loses', async () => {
    const branch = await branchRepository.create(buildBranch());
    const chunk = buildChunk({ branchId: branch.id, originBranchId: branch.id });
    const lockClient = await pool.connect();
    let transactionOpen = false;

    try {
      await lockClient.query('BEGIN');
      transactionOpen = true;
      await lockClient.query('SELECT id FROM branches WHERE id = $1 FOR UPDATE', [branch.id]);

      const createPromise = repository.create(chunk);
      await delay(25);
      const submitPromise = branchRepository.submit(branch.id);

      await delay(50);
      await lockClient.query('COMMIT');
      transactionOpen = false;

      const [submitResult, createResult] = await Promise.allSettled([submitPromise, createPromise]);
      expect(createResult.status).toBe('fulfilled');
      expect(submitResult).toEqual({ status: 'fulfilled', value: undefined });

      const branchAfterRace = await branchRepository.findById(branch.id);
      expect(branchAfterRace).toBeDefined();

      const chunkRow = await pool.query<{ exists: boolean }>(
        'SELECT EXISTS(SELECT 1 FROM chunks WHERE id = $1) AS exists',
        [chunk.id],
      );
      expect(branchAfterRace?.status).toBe('draft');
      expect(chunkRow.rows[0]?.exists).toBe(true);
    } finally {
      if (transactionOpen) {
        await lockClient.query('ROLLBACK');
      }
      lockClient.release();
    }
  });
});
