import { setTimeout as delay } from 'node:timers/promises';
import { ConflictException } from '@nestjs/common';
import type { Pool } from 'pg';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { Branch } from '../../src/domain/branch.js';
import { Chunk } from '../../src/domain/chunk.js';
import { Edge } from '../../src/domain/edge.js';
import { BranchRepository } from '../../src/persistence/branch.repository.js';
import { ChunkRepository } from '../../src/persistence/chunk.repository.js';
import { EdgeRepository } from '../../src/persistence/edge.repository.js';
import { BOOTSTRAP_STAKEHOLDER_ID } from '../../src/persistence/bootstrap-stakeholder.js';
import { setUpTestDatabase, type TestDatabase } from '../support/test-database.js';

const WORKSPACE_ID = '00000000-0000-0000-0000-00000000d0fa';

function buildEdge(overrides: Partial<ConstructorParameters<typeof Edge>[0]> = {}): Edge {
  return new Edge({
    workspaceId: WORKSPACE_ID,
    fromChunkLabel: `from-${Math.random().toString(36).slice(2, 10)}`,
    toChunkLabel: `to-${Math.random().toString(36).slice(2, 10)}`,
    type: 'depends-on',
    discipline: 'engineering',
    createdByStakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
    ...overrides,
  });
}

function buildChunk(overrides: Partial<ConstructorParameters<typeof Chunk>[0]> = {}): Chunk {
  return new Chunk({
    workspaceId: WORKSPACE_ID,
    label: `chunk-${Math.random().toString(36).slice(2, 10)}`,
    content: 'Some atomic idea content.',
    discipline: 'engineering',
    chunkType: 'feature',
    contextKind: 'permanent',
    createdByStakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
    ...overrides,
  });
}

function buildBranch(overrides: Partial<ConstructorParameters<typeof Branch>[0]> = {}): Branch {
  return new Branch({
    workspaceId: WORKSPACE_ID,
    name: `branch-${Math.random().toString(36).slice(2, 10)}`,
    discipline: 'engineering',
    createdByStakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
    ...overrides,
  });
}

describe('EdgeRepository (containerized Postgres)', () => {
  let database: TestDatabase;
  let pool: Pool;
  let edgeRepository: EdgeRepository;
  let chunkRepository: ChunkRepository;
  let branchRepository: BranchRepository;

  beforeAll(async () => {
    database = await setUpTestDatabase();
    pool = database.pool;
    edgeRepository = new EdgeRepository(pool);
    chunkRepository = new ChunkRepository(pool);
    branchRepository = new BranchRepository(pool);
  });

  afterAll(async () => {
    await database.close();
  });

  it('create persists an edge with status active and supersededByEdgeId NULL', async () => {
    const edge = buildEdge();

    const created = await edgeRepository.create(edge);

    expect(created.status).toBe('active');
    expect(created.supersededByEdgeId).toBeUndefined();

    const row = await pool.query<{ status: string; superseded_by_edge_id: string | null }>(
      'SELECT status, superseded_by_edge_id FROM edges WHERE id = $1',
      [created.id],
    );

    expect(row.rows).toHaveLength(1);
    expect(row.rows[0]?.status).toBe('active');
    expect(row.rows[0]?.superseded_by_edge_id).toBeNull();
  });

  it('findById round-trips every field exactly for a persisted edge', async () => {
    const edge = buildEdge({
      type: 'contradicts',
      discipline: 'security',
    });

    const created = await edgeRepository.create(edge);
    const found = await edgeRepository.findById(created.id, WORKSPACE_ID);

    expect(found).toBeDefined();
    expect(found).toEqual(created);
  });

  it('findById returns an explicit not-found result (undefined) for an unknown id', async () => {
    const found = await edgeRepository.findById('00000000-0000-0000-0000-00000000dead', WORKSPACE_ID);

    expect(found).toBeUndefined();
  });

  it('rejects a duplicate active edge (same from/to/type, branchless scope) via idx_edges_mainline', async () => {
    const fromChunkLabel = `from-${Math.random().toString(36).slice(2, 10)}`;
    const toChunkLabel = `to-${Math.random().toString(36).slice(2, 10)}`;

    await edgeRepository.create(buildEdge({ fromChunkLabel, toChunkLabel, type: 'blocks' }));

    await expect(
      edgeRepository.create(buildEdge({ fromChunkLabel, toChunkLabel, type: 'blocks' })),
    ).rejects.toThrow();
  });

  it('rejects a duplicate active edge within the same branch scope via idx_edges_branch_active', async () => {
    const branch = await branchRepository.create(buildBranch());
    const fromChunkLabel = `from-${Math.random().toString(36).slice(2, 10)}`;
    const toChunkLabel = `to-${Math.random().toString(36).slice(2, 10)}`;

    await edgeRepository.create(
      buildEdge({
        fromChunkLabel,
        toChunkLabel,
        type: 'refines',
        branchId: branch.id,
        originBranchId: branch.id,
      }),
    );

    await expect(
      edgeRepository.create(
        buildEdge({
          fromChunkLabel,
          toChunkLabel,
          type: 'refines',
          branchId: branch.id,
          originBranchId: branch.id,
        }),
      ),
    ).rejects.toThrow();
  });

  it('rejects a branch-scoped create when the branch has already been submitted', async () => {
    const branch = await branchRepository.create(buildBranch());
    const fromChunk = await chunkRepository.create(
      buildChunk({
        label: `from-${Math.random().toString(36).slice(2, 10)}`,
        branchId: branch.id,
        originBranchId: branch.id,
      }),
    );
    const toChunk = await chunkRepository.create(
      buildChunk({
        label: `to-${Math.random().toString(36).slice(2, 10)}`,
        branchId: branch.id,
        originBranchId: branch.id,
      }),
    );
    await branchRepository.submit(branch.id, WORKSPACE_ID);

    await expect(
      edgeRepository.create(
        buildEdge({
          fromChunkLabel: fromChunk.label,
          toChunkLabel: toChunk.label,
          branchId: branch.id,
          originBranchId: branch.id,
        }),
      ),
    ).rejects.toThrow(new ConflictException(`Branch ${branch.id} is not in draft status`));
  });

  it('serializes a racing submit and branch-scoped create so the create wins and submit loses', async () => {
    const branch = await branchRepository.create(buildBranch());
    const fromChunk = await chunkRepository.create(
      buildChunk({
        label: `from-${Math.random().toString(36).slice(2, 10)}`,
        branchId: branch.id,
        originBranchId: branch.id,
      }),
    );
    const toChunk = await chunkRepository.create(
      buildChunk({
        label: `to-${Math.random().toString(36).slice(2, 10)}`,
        branchId: branch.id,
        originBranchId: branch.id,
      }),
    );
    const edge = buildEdge({
      fromChunkLabel: fromChunk.label,
      toChunkLabel: toChunk.label,
      branchId: branch.id,
      originBranchId: branch.id,
    });
    const lockClient = await pool.connect();
    let transactionOpen = false;

    try {
      await lockClient.query('BEGIN');
      transactionOpen = true;
      await lockClient.query('SELECT id FROM branches WHERE id = $1 FOR UPDATE', [branch.id]);

      const createPromise = edgeRepository.create(edge);
      await delay(25);
      const submitPromise = branchRepository.submit(branch.id, WORKSPACE_ID);

      await delay(50);
      await lockClient.query('COMMIT');
      transactionOpen = false;

      const [submitResult, createResult] = await Promise.allSettled([submitPromise, createPromise]);
      expect(createResult.status).toBe('fulfilled');
      expect(submitResult).toEqual({ status: 'fulfilled', value: undefined });

      const branchAfterRace = await branchRepository.findById(branch.id, WORKSPACE_ID);
      expect(branchAfterRace).toBeDefined();

      const edgeRow = await pool.query<{ exists: boolean }>(
        'SELECT EXISTS(SELECT 1 FROM edges WHERE id = $1) AS exists',
        [edge.id],
      );
      expect(branchAfterRace?.status).toBe('draft');
      expect(edgeRow.rows[0]?.exists).toBe(true);
    } finally {
      if (transactionOpen) {
        await lockClient.query('ROLLBACK');
      }
      lockClient.release();
    }
  });

  describe('ChunkRepository.findByLabel', () => {
    it('finds a branchless chunk by label when branchId is omitted', async () => {
      const chunk = await chunkRepository.create(buildChunk());

      const found = await chunkRepository.findByLabel(chunk.label, undefined, WORKSPACE_ID);

      expect(found).toEqual(chunk);
    });

    it('finds a branch-scoped chunk by label within that branch scope', async () => {
      const branch = await branchRepository.create(buildBranch());
      const chunk = await chunkRepository.create(
        buildChunk({ branchId: branch.id, originBranchId: branch.id }),
      );

      const found = await chunkRepository.findByLabel(chunk.label, branch.id, WORKSPACE_ID);

      expect(found).toEqual(chunk);
    });

    it('returns undefined when the label exists but not in the requested branch scope', async () => {
      const branch = await branchRepository.create(buildBranch());
      const chunk = await chunkRepository.create(buildChunk());

      const found = await chunkRepository.findByLabel(chunk.label, branch.id, WORKSPACE_ID);

      expect(found).toBeUndefined();
    });

    it('returns undefined when the label does not exist at all', async () => {
      const found = await chunkRepository.findByLabel('does-not-exist-label', undefined, WORKSPACE_ID);

      expect(found).toBeUndefined();
    });
  });
});
