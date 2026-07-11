import { mkdtemp, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import type { Pool, PoolClient, QueryResult } from 'pg';
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it } from 'vitest';
import type { Artifact } from '../../src/domain/artifact.js';
import { Branch } from '../../src/domain/branch.js';
import { Chunk } from '../../src/domain/chunk.js';
import { ChunkArtifactAssociation } from '../../src/domain/chunk-artifact-association.js';
import { DeliverySubscription } from '../../src/domain/delivery-subscription.js';
import { Edge } from '../../src/domain/edge.js';
import { Workspace } from '../../src/domain/workspace.js';
import { ArtifactRepository } from '../../src/persistence/artifact.repository.js';
import { BranchRepository } from '../../src/persistence/branch.repository.js';
import { ChunkArtifactRepository } from '../../src/persistence/chunk-artifact.repository.js';
import { ChunkRepository } from '../../src/persistence/chunk.repository.js';
import { DeliverySubscriptionRepository } from '../../src/persistence/delivery-subscription.repository.js';
import { EdgeRepository } from '../../src/persistence/edge.repository.js';
import { BOOTSTRAP_STAKEHOLDER_ID } from '../../src/persistence/bootstrap-stakeholder.js';
import { LocalFileBlobStore } from '../../src/persistence/local-file-blob-store.js';
import { WorkspaceRepository } from '../../src/persistence/workspace.repository.js';
import { setUpTestDatabase, type TestDatabase } from '../support/test-database.js';

let workspaceId = '';

function buildBranch(overrides: Partial<ConstructorParameters<typeof Branch>[0]> = {}): Branch {
  return new Branch({
    workspaceId,
    name: `branch-${Math.random().toString(36).slice(2, 10)}`,
    discipline: 'engineering',
    createdByStakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
    ...overrides,
  });
}

function buildChunk(overrides: Partial<ConstructorParameters<typeof Chunk>[0]> = {}): Chunk {
  return new Chunk({
    workspaceId,
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
    workspaceId,
    fromChunkLabel: `from-${Math.random().toString(36).slice(2, 10)}`,
    toChunkLabel: `to-${Math.random().toString(36).slice(2, 10)}`,
    type: 'depends-on',
    discipline: 'engineering',
    createdByStakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
    ...overrides,
  });
}

function buildAssociation(
  overrides: Partial<ConstructorParameters<typeof ChunkArtifactAssociation>[0]> &
    Pick<ConstructorParameters<typeof ChunkArtifactAssociation>[0], 'chunkLabel' | 'artifactId'>,
): ChunkArtifactAssociation {
  return new ChunkArtifactAssociation({
    workspaceId,
    createdByStakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
    ...overrides,
  });
}

function buildSubscription(
  overrides: Partial<ConstructorParameters<typeof DeliverySubscription>[0]> = {},
): DeliverySubscription {
  return new DeliverySubscription({
    workspaceId,
    url: 'https://example.com/webhook',
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
  let chunkArtifactRepository: ChunkArtifactRepository;
  let deliverySubscriptionRepository: DeliverySubscriptionRepository;
  let workspaceRepository: WorkspaceRepository;
  let basePath: string;
  let artifactRepository: ArtifactRepository;
  let artifactA: Artifact;

  beforeAll(async () => {
    database = await setUpTestDatabase();
    pool = database.pool;
    branchRepository = new BranchRepository(pool);
    chunkRepository = new ChunkRepository(pool);
    edgeRepository = new EdgeRepository(pool);
    chunkArtifactRepository = new ChunkArtifactRepository(pool);
    deliverySubscriptionRepository = new DeliverySubscriptionRepository(pool);
    workspaceRepository = new WorkspaceRepository(pool);
  });

  afterAll(async () => {
    await database.close();
  });

  beforeEach(async () => {
    const workspace = await workspaceRepository.createWithFirstMember(
      new Workspace({
        name: `branch-merge-workspace-${Math.random().toString(36).slice(2, 10)}`,
        createdByStakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
      }),
    );
    workspaceId = workspace.id;
    basePath = await mkdtemp(join(tmpdir(), 'spool-branch-merge-artifacts-test-'));
    artifactRepository = new ArtifactRepository(pool, new LocalFileBlobStore({ basePath }));
    artifactA = await artifactRepository.create({
      workspaceId,
      content: Buffer.from('artifact A'),
      mimeType: 'text/plain',
      createdByStakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
    });
  });

  afterEach(async () => {
    await rm(basePath, { recursive: true, force: true });
  });

  async function createDraftBranch(
    overrides: Partial<ConstructorParameters<typeof Branch>[0]> = {},
  ): Promise<Branch> {
    return branchRepository.create(buildBranch(overrides));
  }

  async function verifyBranch(branchId: string): Promise<Branch> {
    await branchRepository.submit(branchId, workspaceId);
    const verified = await branchRepository.verify(branchId, workspaceId);
    if (verified === undefined) {
      throw new Error('verifyBranch: verify unexpectedly returned undefined');
    }
    return verified;
  }

  async function listDeliveryAttempts(branchId: string): Promise<
    { subscription_id: string; merge_event_id: string }[]
  > {
    const result = await pool.query<{ subscription_id: string; merge_event_id: string }>(
      `SELECT subscription_id, merge_event_id
         FROM delivery_attempts
        WHERE branch_id = $1
        ORDER BY subscription_id ASC`,
      [branchId],
    );
    return result.rows;
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

    const result = await branchRepository.merge(branch.id, workspaceId, BOOTSTRAP_STAKEHOLDER_ID);

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

  it('fans out one merge event id across all matching subscriptions', async () => {
    const draftBranch = await createDraftBranch();
    await chunkRepository.create(
      buildChunk({ branchId: draftBranch.id, originBranchId: draftBranch.id }),
    );
    const matchingA = await deliverySubscriptionRepository.create(buildSubscription());
    const matchingB = await deliverySubscriptionRepository.create(
      buildSubscription({ disciplineFilter: ['engineering', 'security'] }),
    );
    const branch = await verifyBranch(draftBranch.id);

    const result = await branchRepository.merge(branch.id, workspaceId, BOOTSTRAP_STAKEHOLDER_ID);

    expect(result?.kind).toBe('merged');

    const attempts = await listDeliveryAttempts(branch.id);
    expect(attempts.map((attempt) => attempt.subscription_id)).toEqual(
      [matchingA.id, matchingB.id].sort(),
    );
    expect(new Set(attempts.map((attempt) => attempt.merge_event_id)).size).toBe(1);
  });

  it('matches only active subscriptions whose discipline filter is absent or contains the branch discipline', async () => {
    const draftBranch = await createDraftBranch({ discipline: 'security' });
    await chunkRepository.create(
      buildChunk({ branchId: draftBranch.id, originBranchId: draftBranch.id }),
    );
    const noFilter = await deliverySubscriptionRepository.create(buildSubscription());
    const matchingFilter = await deliverySubscriptionRepository.create(
      buildSubscription({ disciplineFilter: ['engineering', 'security'] }),
    );
    await deliverySubscriptionRepository.create(
      buildSubscription({ disciplineFilter: ['product', 'design'] }),
    );
    await deliverySubscriptionRepository.create(
      buildSubscription({ disciplineFilter: ['security'], isActive: false }),
    );
    const branch = await verifyBranch(draftBranch.id);

    const result = await branchRepository.merge(branch.id, workspaceId, BOOTSTRAP_STAKEHOLDER_ID);

    expect(result?.kind).toBe('merged');

    const attempts = await listDeliveryAttempts(branch.id);
    expect(attempts.map((attempt) => attempt.subscription_id)).toEqual([
      matchingFilter.id,
      noFilter.id,
    ].sort());
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

    const result = await branchRepository.merge(branch.id, workspaceId, BOOTSTRAP_STAKEHOLDER_ID);

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

    const result = await branchRepository.merge(branch.id, workspaceId, BOOTSTRAP_STAKEHOLDER_ID);

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

    const result = await branchRepository.merge(branch.id, workspaceId, BOOTSTRAP_STAKEHOLDER_ID);

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

  it('promotes only the authoritative active chunk_artifacts row per pair, leaving older branch history untouched', async () => {
    const chunkLabel = `chunk-${Math.random().toString(36).slice(2, 10)}`;
    const draftBranch = await createDraftBranch();
    const t0 = new Date('2026-02-01T00:00:00Z');

    // Both rows target the same (chunkLabel, artifactId) pair, simulating a detach -> re-attach
    // history within the branch: only the most-recently-created row is authoritative.
    const olderAssociation = await chunkArtifactRepository.create(
      buildAssociation({
        chunkLabel,
        artifactId: artifactA.id,
        status: 'deactivated',
        branchId: draftBranch.id,
        originBranchId: draftBranch.id,
        createdAt: t0,
      }),
    );
    const authoritativeAssociation = await chunkArtifactRepository.create(
      buildAssociation({
        chunkLabel,
        artifactId: artifactA.id,
        branchId: draftBranch.id,
        originBranchId: draftBranch.id,
        createdAt: new Date(t0.getTime() + 1000),
      }),
    );
    const branch = await verifyBranch(draftBranch.id);

    const result = await branchRepository.merge(branch.id, workspaceId, BOOTSTRAP_STAKEHOLDER_ID);

    expect(result?.kind).toBe('merged');

    const olderRow = await pool.query<{ branch_id: string | null }>(
      'SELECT branch_id FROM chunk_artifacts WHERE id = $1',
      [olderAssociation.id],
    );
    expect(olderRow.rows[0]?.branch_id).toBe(branch.id);

    const authoritativeRow = await pool.query<{ branch_id: string | null; status: string }>(
      'SELECT branch_id, status FROM chunk_artifacts WHERE id = $1',
      [authoritativeAssociation.id],
    );
    expect(authoritativeRow.rows[0]?.branch_id).toBeNull();
    expect(authoritativeRow.rows[0]?.status).toBe('active');
  });

  it('promotes an authoritative deactivated chunk_artifacts row unconditionally, with no collision check', async () => {
    const chunkLabel = `chunk-${Math.random().toString(36).slice(2, 10)}`;
    await chunkArtifactRepository.create(buildAssociation({ chunkLabel, artifactId: artifactA.id }));

    const draftBranch = await createDraftBranch();
    const deactivation = await chunkArtifactRepository.create(
      buildAssociation({
        chunkLabel,
        artifactId: artifactA.id,
        status: 'deactivated',
        branchId: draftBranch.id,
        originBranchId: draftBranch.id,
      }),
    );
    const branch = await verifyBranch(draftBranch.id);

    const result = await branchRepository.merge(branch.id, workspaceId, BOOTSTRAP_STAKEHOLDER_ID);

    expect(result?.kind).toBe('merged');

    const deactivationRow = await pool.query<{ branch_id: string | null; status: string }>(
      'SELECT branch_id, status FROM chunk_artifacts WHERE id = $1',
      [deactivation.id],
    );
    expect(deactivationRow.rows[0]?.branch_id).toBeNull();
    expect(deactivationRow.rows[0]?.status).toBe('deactivated');
  });

  it('rejects a merge in full when a branch active chunk-artifact pair collides with a mainline active pair', async () => {
    const chunkLabel = `chunk-${Math.random().toString(36).slice(2, 10)}`;
    await chunkArtifactRepository.create(buildAssociation({ chunkLabel, artifactId: artifactA.id }));

    const draftBranch = await createDraftBranch();
    const branchAssociation = await chunkArtifactRepository.create(
      buildAssociation({
        chunkLabel,
        artifactId: artifactA.id,
        branchId: draftBranch.id,
        originBranchId: draftBranch.id,
      }),
    );
    const branch = await verifyBranch(draftBranch.id);

    const result = await branchRepository.merge(branch.id, workspaceId, BOOTSTRAP_STAKEHOLDER_ID);

    expect(result?.kind).toBe('conflict');

    const branchAssociationRow = await pool.query<{ branch_id: string | null }>(
      'SELECT branch_id FROM chunk_artifacts WHERE id = $1',
      [branchAssociation.id],
    );
    expect(branchAssociationRow.rows[0]?.branch_id).toBe(branch.id);

    const branchRow = await pool.query<{ status: string; merged_at: Date | null }>(
      'SELECT status, merged_at FROM branches WHERE id = $1',
      [branch.id],
    );
    expect(branchRow.rows[0]?.status).toBe('verified');
    expect(branchRow.rows[0]?.merged_at).toBeNull();
  });

  it('a delivery-attempt fan-out failure rolls back the whole merge transaction', async () => {
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
    await deliverySubscriptionRepository.create(buildSubscription());
    const branch = await verifyBranch(draftBranch.id);

    // Wrap the real pool so the fan-out insert fails after the branch/chunk/edge updates have run
    // within the same transaction, proving the whole merge still rolls back atomically.
    const poisonedPool: Pick<Pool, 'connect'> = {
      connect: async (): Promise<PoolClient> => {
        const client = await pool.connect();
        const originalQuery = client.query.bind(client) as (
          ...args: Parameters<PoolClient['query']>
        ) => Promise<QueryResult>;
        // Give the poisoned wrapper an explicit, non-overloaded signature (matching how
        // `BranchRepository` actually calls `query`: text + params, returning a promise); `pg`'s
        // real overload set also includes callback-style signatures returning `void`, which would
        // otherwise make forwarding `originalQuery`'s result look like a possible void return.
        const poisonedQuery = (
          ...args: Parameters<PoolClient['query']>
        ): Promise<QueryResult> => {
          const sql = typeof args[0] === 'string' ? args[0] : undefined;
          if (sql?.includes('INSERT INTO delivery_attempts')) {
            return Promise.reject(new Error('Simulated fan-out failure'));
          }
          return originalQuery(...args);
        };
        client.query = poisonedQuery as PoolClient['query'];
        return client;
      },
    };
    const poisonedBranchRepository = new BranchRepository(poisonedPool as Pool);

    await expect(
      poisonedBranchRepository.merge(branch.id, workspaceId, BOOTSTRAP_STAKEHOLDER_ID),
    ).rejects.toThrow('Simulated fan-out failure');

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

    expect(await listDeliveryAttempts(branch.id)).toEqual([]);
  });

  it('treats zero matching subscriptions as a successful no-op fan-out', async () => {
    const draftBranch = await createDraftBranch({ discipline: 'governance' });
    await chunkRepository.create(
      buildChunk({ branchId: draftBranch.id, originBranchId: draftBranch.id }),
    );
    const branch = await verifyBranch(draftBranch.id);

    const result = await branchRepository.merge(branch.id, workspaceId, BOOTSTRAP_STAKEHOLDER_ID);

    expect(result?.kind).toBe('merged');
    expect(await listDeliveryAttempts(branch.id)).toEqual([]);
  });
});
