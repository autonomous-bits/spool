import type { Pool, PoolClient } from 'pg';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { Branch } from '../../src/domain/branch.js';
import { Chunk } from '../../src/domain/chunk.js';
import { Edge } from '../../src/domain/edge.js';
import { BranchRepository } from '../../src/persistence/branch.repository.js';
import { ChunkRepository } from '../../src/persistence/chunk.repository.js';
import { EdgeRepository } from '../../src/persistence/edge.repository.js';
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

function buildEdge(overrides: Partial<ConstructorParameters<typeof Edge>[0]> = {}): Edge {
  return new Edge({
    fromChunkLabel: `from-${Math.random().toString(36).slice(2, 10)}`,
    toChunkLabel: `to-${Math.random().toString(36).slice(2, 10)}`,
    type: 'depends-on',
    discipline: 'engineering',
    createdByStakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
    ...overrides,
  });
}

describe('BranchRepository.merge (containerized Postgres)', () => {
  let database: TestDatabase;
  let pool: Pool;
  let branchRepository: BranchRepository;
  let chunkRepository: ChunkRepository;
  let edgeRepository: EdgeRepository;

  beforeAll(async () => {
    database = await setUpTestDatabase();
    pool = database.pool;
    branchRepository = new BranchRepository(pool);
    chunkRepository = new ChunkRepository(pool);
    edgeRepository = new EdgeRepository(pool);
  });

  afterAll(async () => {
    await database?.close();
  });

  async function createDraftBranch(
    overrides: Partial<ConstructorParameters<typeof Branch>[0]> = {},
  ): Promise<Branch> {
    return branchRepository.create(buildBranch(overrides));
  }

  async function verifyBranch(branchId: string): Promise<Branch> {
    await branchRepository.submit(branchId);
    const verified = await branchRepository.verify(branchId);
    if (verified === undefined) {
      throw new Error('verifyBranch: verify unexpectedly returned undefined');
    }
    return verified;
  }

  it('merges a verified branch with no mainline collisions: promotes chunks/edges and marks branch merged', async () => {
    const draftBranch = await createDraftBranch();
    const chunk = await chunkRepository.create(
      buildChunk({ branchId: draftBranch.id, originBranchId: draftBranch.id }),
    );
    const edge = await edgeRepository.create(
      buildEdge({
        fromChunkLabel: chunk.label,
        toChunkLabel: `target-${Math.random().toString(36).slice(2, 10)}`,
        branchId: draftBranch.id,
        originBranchId: draftBranch.id,
      }),
    );
    const branch = await verifyBranch(draftBranch.id);

    const result = await branchRepository.merge(branch.id, BOOTSTRAP_STAKEHOLDER_ID);

    expect(result?.kind).toBe('merged');
    if (result?.kind !== 'merged') {
      throw new Error('expected merged result');
    }
    expect(result.branch.status).toBe('merged');
    expect(result.branch.mergedAt).toBeInstanceOf(Date);
    expect(result.branch.mergedByStakeholderId).toBe(BOOTSTRAP_STAKEHOLDER_ID);

    const chunkRow = await pool.query<{
      branch_id: string | null;
      status: string;
      origin_branch_id: string | null;
    }>('SELECT branch_id, status, origin_branch_id FROM chunks WHERE id = $1', [chunk.id]);
    expect(chunkRow.rows[0]?.branch_id).toBeNull();
    expect(chunkRow.rows[0]?.status).toBe('promoted');
    expect(chunkRow.rows[0]?.origin_branch_id).toBe(branch.id);

    const edgeRow = await pool.query<{ branch_id: string | null; origin_branch_id: string | null }>(
      'SELECT branch_id, origin_branch_id FROM edges WHERE id = $1',
      [edge.id],
    );
    expect(edgeRow.rows[0]?.branch_id).toBeNull();
    expect(edgeRow.rows[0]?.origin_branch_id).toBe(branch.id);

    const branchRow = await pool.query<{
      status: string;
      merged_at: Date | null;
      merged_by_stakeholder_id: string | null;
    }>('SELECT status, merged_at, merged_by_stakeholder_id FROM branches WHERE id = $1', [branch.id]);
    expect(branchRow.rows[0]?.status).toBe('merged');
    expect(branchRow.rows[0]?.merged_at).toBeInstanceOf(Date);
    expect(branchRow.rows[0]?.merged_by_stakeholder_id).toBe(BOOTSTRAP_STAKEHOLDER_ID);
  });

  it('rejects a merge in full when a branch chunk label collides with a promoted mainline chunk', async () => {
    const collidingLabel = `collide-chunk-${Math.random().toString(36).slice(2, 10)}`;
    await chunkRepository.create(
      buildChunk({ label: collidingLabel, status: 'promoted' }),
    );

    const draftBranch = await createDraftBranch();
    const chunk = await chunkRepository.create(
      buildChunk({ label: collidingLabel, branchId: draftBranch.id, originBranchId: draftBranch.id }),
    );
    const branch = await verifyBranch(draftBranch.id);

    const result = await branchRepository.merge(branch.id, BOOTSTRAP_STAKEHOLDER_ID);

    expect(result?.kind).toBe('conflict');

    const chunkRow = await pool.query<{ branch_id: string | null; status: string }>(
      'SELECT branch_id, status FROM chunks WHERE id = $1',
      [chunk.id],
    );
    expect(chunkRow.rows[0]?.branch_id).toBe(branch.id);
    expect(chunkRow.rows[0]?.status).toBe('draft');

    const branchRow = await pool.query<{ status: string; merged_at: Date | null }>(
      'SELECT status, merged_at FROM branches WHERE id = $1',
      [branch.id],
    );
    expect(branchRow.rows[0]?.status).toBe('verified');
    expect(branchRow.rows[0]?.merged_at).toBeNull();
  });

  it('rejects a merge in full when a branch edge identity collides with a mainline active edge', async () => {
    const fromLabel = `edge-from-${Math.random().toString(36).slice(2, 10)}`;
    const toLabel = `edge-to-${Math.random().toString(36).slice(2, 10)}`;
    await edgeRepository.create(
      buildEdge({ fromChunkLabel: fromLabel, toChunkLabel: toLabel, type: 'depends-on' }),
    );

    const draftBranch = await createDraftBranch();
    const edge = await edgeRepository.create(
      buildEdge({
        fromChunkLabel: fromLabel,
        toChunkLabel: toLabel,
        type: 'depends-on',
        branchId: draftBranch.id,
        originBranchId: draftBranch.id,
      }),
    );
    const branch = await verifyBranch(draftBranch.id);

    const result = await branchRepository.merge(branch.id, BOOTSTRAP_STAKEHOLDER_ID);

    expect(result?.kind).toBe('conflict');

    const edgeRow = await pool.query<{ branch_id: string | null }>(
      'SELECT branch_id FROM edges WHERE id = $1',
      [edge.id],
    );
    expect(edgeRow.rows[0]?.branch_id).toBe(branch.id);

    const branchRow = await pool.query<{ status: string; merged_at: Date | null }>(
      'SELECT status, merged_at FROM branches WHERE id = $1',
      [branch.id],
    );
    expect(branchRow.rows[0]?.status).toBe('verified');
    expect(branchRow.rows[0]?.merged_at).toBeNull();
  });

  it('returns undefined and mutates nothing when the branch is not in verified status', async () => {
    const branch = await branchRepository.create(buildBranch());
    const chunk = await chunkRepository.create(
      buildChunk({ branchId: branch.id, originBranchId: branch.id }),
    );

    const result = await branchRepository.merge(branch.id, BOOTSTRAP_STAKEHOLDER_ID);

    expect(result).toBeUndefined();

    const chunkRow = await pool.query<{ branch_id: string | null; status: string }>(
      'SELECT branch_id, status FROM chunks WHERE id = $1',
      [chunk.id],
    );
    expect(chunkRow.rows[0]?.branch_id).toBe(branch.id);
    expect(chunkRow.rows[0]?.status).toBe('draft');

    const branchRow = await pool.query<{ status: string }>(
      'SELECT status FROM branches WHERE id = $1',
      [branch.id],
    );
    expect(branchRow.rows[0]?.status).toBe('draft');
  });

  it('a simulated mid-transaction failure leaves the database in its pre-merge state', async () => {
    const draftBranch = await createDraftBranch();
    const chunk = await chunkRepository.create(
      buildChunk({ branchId: draftBranch.id, originBranchId: draftBranch.id }),
    );
    const edge = await edgeRepository.create(
      buildEdge({
        fromChunkLabel: chunk.label,
        toChunkLabel: `target-${Math.random().toString(36).slice(2, 10)}`,
        branchId: draftBranch.id,
        originBranchId: draftBranch.id,
      }),
    );
    const branch = await verifyBranch(draftBranch.id);

    // Wrap the real pool so the finalizing "mark branch merged" UPDATE fails after chunks/edges
    // have already been promoted within the same transaction — a genuine mid-transaction failure
    // distinct from the pre-checked conflict path, proving the whole merge rolls back atomically.
    const poisonedPool: Pick<Pool, 'connect'> = {
      connect: async (): Promise<PoolClient> => {
        const client = await pool.connect();
        const originalQuery = client.query.bind(client);
        const poisonedQuery = (
          ...args: Parameters<PoolClient['query']>
        ): ReturnType<PoolClient['query']> => {
          const sql = typeof args[0] === 'string' ? args[0] : undefined;
          if (sql?.includes("SET status = 'merged'")) {
            return Promise.reject(new Error('Simulated mid-transaction failure'));
          }
          return originalQuery(...args);
        };
        client.query = poisonedQuery as PoolClient['query'];
        return client;
      },
    };
    const poisonedBranchRepository = new BranchRepository(poisonedPool as Pool);

    await expect(
      poisonedBranchRepository.merge(branch.id, BOOTSTRAP_STAKEHOLDER_ID),
    ).rejects.toThrowError('Simulated mid-transaction failure');

    const branchRow = await pool.query<{ status: string; merged_at: Date | null }>(
      'SELECT status, merged_at FROM branches WHERE id = $1',
      [branch.id],
    );
    expect(branchRow.rows[0]?.status).toBe('verified');
    expect(branchRow.rows[0]?.merged_at).toBeNull();

    const chunkRow = await pool.query<{ branch_id: string | null; status: string }>(
      'SELECT branch_id, status FROM chunks WHERE id = $1',
      [chunk.id],
    );
    expect(chunkRow.rows[0]?.branch_id).toBe(branch.id);
    expect(chunkRow.rows[0]?.status).toBe('draft');

    const edgeRow = await pool.query<{ branch_id: string | null }>(
      'SELECT branch_id FROM edges WHERE id = $1',
      [edge.id],
    );
    expect(edgeRow.rows[0]?.branch_id).toBe(branch.id);
  });
});
