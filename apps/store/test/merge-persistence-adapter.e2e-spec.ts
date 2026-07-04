/**
 * Adapter-level integration test proving branch merges are all-or-nothing
 * and traceable (story S07) against a real containerized Postgres.
 *
 * Technical spec §"Testing expectations" requires this kind of test to run
 * against a real containerized Postgres, not an in-memory substitute. Start
 * it locally before running this file:
 *
 *   docker compose up -d postgres
 *
 * and export the matching connection env vars (see apps/store/AGENTS.md and
 * config/store.env.example), e.g.:
 *
 *   export STORE_DB_HOST=localhost STORE_DB_PORT=5433 \
 *     STORE_DB_USER=spool STORE_DB_PASSWORD=spool_dev STORE_DB_NAME=spool
 */

import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { Pool } from 'pg';
import { randomUUID } from 'node:crypto';
import { loadDatabaseConfig } from '../src/persistence/database-config.js';
import { ensureSchema } from '../src/persistence/schema.js';
import { MergeRepository } from '../src/persistence/merge.repository.js';
import { ChunkGraphRepository } from '../src/persistence/chunk-graph.repository.js';
import { BranchGraphRepository } from '../src/persistence/branch-graph.repository.js';
import { ArtifactAssociationRepository } from '../src/persistence/artifact-association.repository.js';
import { ConflictDetectionRepository } from '../src/persistence/conflict-detection.repository.js';
import { chunkLifecycleStatus } from '../src/domain/chunk-lifecycle.js';
import { createEdge } from '../src/domain/edge-lineage.js';
import { humanActor } from '../src/domain/types/actor/actor-context.js';
import { BranchLifecycleError } from '../src/domain/branch-lifecycle.js';
import {
  artifactId,
  ideaLabel,
  stakeholderId,
  workspaceId,
  type BranchId,
} from '../src/domain/types/index.js';

function openPool(): Pool {
  const config = loadDatabaseConfig();
  return new Pool({
    host: config.host,
    port: config.port,
    user: config.user,
    password: config.password,
    database: config.database,
  });
}

const hasDatabaseConfig = [
  'STORE_DB_HOST',
  'STORE_DB_PORT',
  'STORE_DB_USER',
  'STORE_DB_PASSWORD',
  'STORE_DB_NAME',
].every((key) => Boolean(process.env[key]?.trim()));

describe.skipIf(!hasDatabaseConfig)(
  'MergeRepository (Postgres adapter, atomic branch merge)',
  () => {
    const ws = workspaceId(`ws-${randomUUID()}`);
    const wsOther = workspaceId(`ws-${randomUUID()}`);
    const actor = humanActor(stakeholderId(`stakeholder-${randomUUID()}`));

    beforeAll(async () => {
      const bootstrapPool = openPool();
      await ensureSchema(bootstrapPool);
      await bootstrapPool.end();
    });

    afterAll(async () => {
      const cleanupPool = openPool();
      for (const workspace of [ws, wsOther]) {
        await cleanupPool.query('DELETE FROM branches WHERE workspace_id = $1', [workspace]);
        await cleanupPool.query('DELETE FROM branch_chunk_deltas WHERE workspace_id = $1', [
          workspace,
        ]);
        await cleanupPool.query('DELETE FROM branch_edge_deltas WHERE workspace_id = $1', [
          workspace,
        ]);
        await cleanupPool.query('DELETE FROM chunks WHERE workspace_id = $1', [workspace]);
        await cleanupPool.query('DELETE FROM edge_versions WHERE workspace_id = $1', [workspace]);
        await cleanupPool.query('DELETE FROM chunk_artifacts WHERE workspace_id = $1', [
          workspace,
        ]);
      }
      await cleanupPool.end();
    });

    /**
     * Registers a branch and drives it through draft -> submitted ->
     * verified. `writeDraftDeltas`, when supplied, runs immediately after
     * registration and before submit/verify — branch-scoped chunk/edge
     * deltas (story S11) are only writable while the branch is still
     * `draft` (`BranchLifecycleError('write-locked')` otherwise), so any
     * delta writes a test needs must happen inside this callback rather
     * than after this helper returns.
     */
    async function registerVerifiedBranch(
      pool: Pool,
      workspace: ReturnType<typeof workspaceId>,
      branch: BranchId,
      writeDraftDeltas?: () => Promise<void>,
    ): Promise<MergeRepository> {
      const conflicts = new ConflictDetectionRepository(pool);
      const merges = new MergeRepository(pool, new ChunkGraphRepository(pool));
      await conflicts.registerBranch(workspace, branch, 'engineering');
      if (writeDraftDeltas) {
        await writeDraftDeltas();
      }
      await merges.submitBranch(workspace, branch, actor, 'engineering');
      await merges.verifyBranch(workspace, branch, actor);
      return merges;
    }

    it('AC2 + AC3: a successful merge promotes chunk, edge, and artifact-association changes together, each traceable back to the branch', async () => {
      const pool = openPool();
      const chunks = new ChunkGraphRepository(pool);
      const branches = new BranchGraphRepository(pool, chunks);
      const associations = new ArtifactAssociationRepository(pool);
      const branch = `branch-${randomUUID()}` as BranchId;
      const chunkLabel = ideaLabel('IDEA-merge-success-chunk');
      const source = ideaLabel('IDEA-merge-success-source');
      const target = ideaLabel('IDEA-merge-success-target');
      const artifact = artifactId(`artifact-${randomUUID()}`);

      const merges = await registerVerifiedBranch(pool, ws, branch, async () => {
        await branches.saveChunkDelta({
          workspaceId: ws,
          branchId: branch,
          ideaLabel: chunkLabel,
          deltaKind: 'upsert',
          chunk: {
            workspaceId: ws,
            ideaLabel: chunkLabel,
            chunkType: 'feature',
            discipline: 'engineering',
            contextKind: 'permanent',
            content: 'Branch-authored chunk.',
            status: chunkLifecycleStatus('draft', 'active'),
          },
        });
        await branches.saveEdgeDelta({
          workspaceId: ws,
          branchId: branch,
          sourceLabel: source,
          targetLabel: target,
          relationshipType: 'depends-on',
          deltaKind: 'upsert',
        });
        // Association writes must happen while the branch is still 'draft'
        // (branch write-lock enforcement now covers chunk_artifacts too —
        // fixes rubber-duck-review gap found against Meridian IDEA-35).
        await associations.createAssociation({
          workspaceId: ws,
          chunkLabel,
          artifactId: artifact,
          branchId: branch,
        });
      });

      const outcome = await merges.mergeBranch(ws, branch, actor);

      const mergedChunk = await chunks.findChunk(ws, chunkLabel);
      const mergedEdge = await chunks.findEdgeLineage(ws, source, target, 'depends-on');
      const mergedAssociations = await merges.listArtifactAssociationsByOriginBranch(ws, branch);
      const status = await merges.getBranchStatus(ws, branch);
      await pool.end();

      expect(outcome.mergedChunkLabels).toContain(chunkLabel);
      expect(mergedChunk?.content).toBe('Branch-authored chunk.');
      expect(mergedChunk?.originBranchId).toBe(branch);

      expect(mergedEdge?.versions.at(-1)?.state).toBe('active');
      expect(
        outcome.mergedEdgeIdentities.some(
          (e) => e.sourceLabel === source && e.targetLabel === target,
        ),
      ).toBe(true);

      expect(mergedAssociations.some((a) => a.chunkLabel === chunkLabel && a.status === 'active')).toBe(
        true,
      );

      expect(status).toBe('merged');
    });

    it('artifact-association promotion matrix: branch-active over mainline-active is an idempotent no-op; branch-active over empty mainline appends a fresh active row stamped with the merging branch; branch-deactivated over mainline-active supersedes then appends, stamped with the merging branch', async () => {
      const pool = openPool();
      const chunks = new ChunkGraphRepository(pool);
      const associations = new ArtifactAssociationRepository(pool);
      const artifactFreshMainline = artifactId(`artifact-fresh-${randomUUID()}`);
      const artifactBothActive = artifactId(`artifact-both-active-${randomUUID()}`);
      const artifactDeactivateOverActive = artifactId(`artifact-deactivate-over-active-${randomUUID()}`);
      const chunkLabel = ideaLabel('IDEA-merge-artifact-matrix');

      // Case A: mainline never had it — branch's active association appends fresh.
      const branchA = `branch-${randomUUID()}` as BranchId;
      const mergesA = await registerVerifiedBranch(pool, ws, branchA, async () => {
        await associations.createAssociation({
          workspaceId: ws,
          chunkLabel,
          artifactId: artifactFreshMainline,
          branchId: branchA,
        });
      });
      await mergesA.mergeBranch(ws, branchA, actor);
      const afterFresh = await mergesA.listArtifactAssociationsByOriginBranch(ws, branchA);
      expect(
        afterFresh.some((a) => a.artifactId === artifactFreshMainline && a.status === 'active'),
      ).toBe(true);

      // Case B: mainline already active, branch independently also active —
      // must no-op (idempotent), not attempt a second insert.
      const branchB = `branch-${randomUUID()}` as BranchId;
      await associations.createAssociation({
        workspaceId: ws,
        chunkLabel,
        artifactId: artifactBothActive,
      });
      const mergesB = await registerVerifiedBranch(pool, ws, branchB, async () => {
        await associations.createAssociation({
          workspaceId: ws,
          chunkLabel,
          artifactId: artifactBothActive,
          branchId: branchB,
        });
      });
      await expect(mergesB.mergeBranch(ws, branchB, actor)).resolves.toBeDefined();
      const bothActiveAfter = await mergesB.listArtifactAssociationsByOriginBranch(ws, branchB);
      // The mainline row was never touched by this merge (no-op), so it is
      // not attributed to branchB.
      expect(bothActiveAfter.some((a) => a.artifactId === artifactBothActive)).toBe(false);

      // Case C: mainline active, branch deactivates — supersede then append,
      // the newly-appended 'deactivated' row is stamped with this merge's branch.
      const branchC = `branch-${randomUUID()}` as BranchId;
      await associations.createAssociation({
        workspaceId: ws,
        chunkLabel,
        artifactId: artifactDeactivateOverActive,
      });
      const mergesC = await registerVerifiedBranch(pool, ws, branchC, async () => {
        await associations.createAssociation({
          workspaceId: ws,
          chunkLabel,
          artifactId: artifactDeactivateOverActive,
          branchId: branchC,
        });
        await associations.deactivateAssociation(ws, chunkLabel, artifactDeactivateOverActive, branchC);
      });
      await mergesC.mergeBranch(ws, branchC, actor);
      const deactivateAfter = await mergesC.listArtifactAssociationsByOriginBranch(ws, branchC);
      await pool.end();

      expect(
        deactivateAfter.some(
          (a) => a.artifactId === artifactDeactivateOverActive && a.status === 'deactivated',
        ),
      ).toBe(true);
    });

    it('AC1: a forced failure mid-merge (branch reactivates an already-deactivated mainline edge) rolls back every change attempted in that merge — chunk, artifact association, and branch status included', async () => {
      const pool = openPool();
      const chunks = new ChunkGraphRepository(pool);
      const branches = new BranchGraphRepository(pool, chunks);
      const associations = new ArtifactAssociationRepository(pool);
      const branch = `branch-${randomUUID()}` as BranchId;
      const chunkLabel = ideaLabel('IDEA-merge-rollback-chunk');
      const source = ideaLabel('IDEA-merge-rollback-source');
      const target = ideaLabel('IDEA-merge-rollback-target');
      const artifact = artifactId(`artifact-${randomUUID()}`);

      // Mainline already has this edge deactivated before the branch diverged.
      await chunks.saveEdgeLineage(createEdge(ws, source, target, 'depends-on'));
      const deactivated = await chunks.findEdgeLineage(ws, source, target, 'depends-on');
      expect(deactivated).toBeDefined();
      const { deactivateEdge } = await import('../src/domain/edge-lineage.js');
      await chunks.saveEdgeLineage(deactivateEdge(deactivated!));

      const merges = await registerVerifiedBranch(pool, ws, branch, async () => {
        // Chunk and edge deltas are written FIRST (while the branch is
        // still draft) so the test proves multi-kind rollback, not just
        // edge rollback: if the merge did not roll back atomically, these
        // would be visible on mainline even though the merge as a whole
        // failed.
        await branches.saveChunkDelta({
          workspaceId: ws,
          branchId: branch,
          ideaLabel: chunkLabel,
          deltaKind: 'upsert',
          chunk: {
            workspaceId: ws,
            ideaLabel: chunkLabel,
            chunkType: 'feature',
            discipline: 'engineering',
            contextKind: 'permanent',
            content: 'Should never reach mainline.',
            status: chunkLifecycleStatus('draft', 'active'),
          },
        });
        // This delta forces `resolveEdgeDelta` to throw mid-transaction:
        // the branch asserts 'upsert' over a mainline edge that is already
        // 'deactivated', and the domain has no reactivation transition.
        await branches.saveEdgeDelta({
          workspaceId: ws,
          branchId: branch,
          sourceLabel: source,
          targetLabel: target,
          relationshipType: 'depends-on',
          deltaKind: 'upsert',
        });
        await associations.createAssociation({
          workspaceId: ws,
          chunkLabel,
          artifactId: artifact,
          branchId: branch,
        });
      });

      await expect(merges.mergeBranch(ws, branch, actor)).rejects.toMatchObject({
        name: 'EdgeLineageError',
        code: 'invalid-state-transition',
      });

      const chunkAfter = await chunks.findChunk(ws, chunkLabel);
      const associationsAfter = await merges.listArtifactAssociationsByOriginBranch(ws, branch);
      const statusAfter = await merges.getBranchStatus(ws, branch);
      const edgeAfter = await chunks.findEdgeLineage(ws, source, target, 'depends-on');
      await pool.end();

      expect(chunkAfter).toBeUndefined();
      expect(associationsAfter).toHaveLength(0);
      expect(statusAfter).toBe('verified');
      expect(edgeAfter?.versions.at(-1)?.state).toBe('deactivated');
    });

    it('guard: merging a branch that is not yet verified is rejected and changes nothing', async () => {
      const pool = openPool();
      const conflicts = new ConflictDetectionRepository(pool);
      const merges = new MergeRepository(pool, new ChunkGraphRepository(pool));
      const branch = `branch-${randomUUID()}` as BranchId;

      await conflicts.registerBranch(ws, branch, 'engineering');
      await expect(merges.mergeBranch(ws, branch, actor)).rejects.toBeInstanceOf(
        BranchLifecycleError,
      );
      const status = await merges.getBranchStatus(ws, branch);
      await pool.end();

      expect(status).toBe('draft');
    });

    it('guard: submitting/verifying/merging with a non-human actor is rejected', async () => {
      const pool = openPool();
      const conflicts = new ConflictDetectionRepository(pool);
      const merges = new MergeRepository(pool, new ChunkGraphRepository(pool));
      const branch = `branch-${randomUUID()}` as BranchId;
      const { delegatedActor } = await import('../src/domain/types/actor/actor-context.js');
      const nonHuman = delegatedActor(stakeholderId(`delegate-${randomUUID()}`));

      await conflicts.registerBranch(ws, branch, 'engineering');
      await expect(
        merges.submitBranch(ws, branch, nonHuman, 'engineering'),
      ).rejects.toMatchObject({ code: 'unauthorized-actor' });
      await pool.end();
    });

    it('not-found: operating on an unregistered branch is rejected', async () => {
      const pool = openPool();
      const merges = new MergeRepository(pool, new ChunkGraphRepository(pool));
      const branch = `branch-${randomUUID()}` as BranchId;

      await expect(merges.getBranchStatus(ws, branch)).rejects.toMatchObject({
        code: 'not-found',
      });
      await expect(merges.mergeBranch(ws, branch, actor)).rejects.toMatchObject({
        code: 'not-found',
      });
      await pool.end();
    });

    it("tenant isolation: merging a branch in one workspace never promotes into another workspace's mainline", async () => {
      const pool = openPool();
      const chunks = new ChunkGraphRepository(pool);
      const branches = new BranchGraphRepository(pool, chunks);
      const branch = `branch-${randomUUID()}` as BranchId;
      const chunkLabel = ideaLabel('IDEA-merge-tenant-isolation');

      const merges = await registerVerifiedBranch(pool, ws, branch, async () => {
        await branches.saveChunkDelta({
          workspaceId: ws,
          branchId: branch,
          ideaLabel: chunkLabel,
          deltaKind: 'upsert',
          chunk: {
            workspaceId: ws,
            ideaLabel: chunkLabel,
            chunkType: 'feature',
            discipline: 'engineering',
            contextKind: 'permanent',
            content: 'Workspace-scoped merge.',
            status: chunkLifecycleStatus('draft', 'active'),
          },
        });
      });
      await merges.mergeBranch(ws, branch, actor);

      const otherWorkspaceChunk = await chunks.findChunk(wsOther, chunkLabel);
      await pool.end();

      expect(otherWorkspaceChunk).toBeUndefined();
    });

    it('tenant isolation: the same branchId registered in two workspaces is two independent branch records (S10)', async () => {
      const pool = openPool();
      const conflicts = new ConflictDetectionRepository(pool);
      const chunks = new ChunkGraphRepository(pool);
      const branches = new BranchGraphRepository(pool, chunks);
      const merges = new MergeRepository(pool, chunks);
      const sharedBranchId = `branch-${randomUUID()}` as BranchId;
      const chunkLabel = ideaLabel('IDEA-merge-branch-identity-isolation');

      // Register the identical branchId independently in both workspaces.
      await conflicts.registerBranch(ws, sharedBranchId, 'engineering');
      await conflicts.registerBranch(wsOther, sharedBranchId, 'engineering');

      // Capture wsOther's baseline before driving ws's branch through its full lifecycle.
      const otherDivergedAtBefore = await conflicts.getDivergedAt(wsOther, sharedBranchId);
      const otherStatusBefore = await merges.getBranchStatus(wsOther, sharedBranchId);

      await branches.saveChunkDelta({
        workspaceId: ws,
        branchId: sharedBranchId,
        ideaLabel: chunkLabel,
        deltaKind: 'upsert',
        chunk: {
          workspaceId: ws,
          ideaLabel: chunkLabel,
          chunkType: 'feature',
          discipline: 'engineering',
          contextKind: 'permanent',
          content: 'ws-only branch-identity-collision content.',
          status: chunkLifecycleStatus('draft', 'active'),
        },
      });
      await merges.submitBranch(ws, sharedBranchId, actor, 'engineering');
      await merges.verifyBranch(ws, sharedBranchId, actor);
      await merges.mergeBranch(ws, sharedBranchId, actor);

      const wsStatusAfter = await merges.getBranchStatus(ws, sharedBranchId);
      const otherDivergedAtAfter = await conflicts.getDivergedAt(wsOther, sharedBranchId);
      const otherStatusAfter = await merges.getBranchStatus(wsOther, sharedBranchId);
      const otherWorkspaceChunk = await chunks.findChunk(wsOther, chunkLabel);
      await pool.end();

      expect(wsStatusAfter).toBe('merged');
      // wsOther's identically-identified branch must be completely unaffected: still its
      // original status, still its original divergence marker, and never sees ws's chunk.
      expect(otherStatusBefore).toBe('draft');
      expect(otherStatusAfter).toBe('draft');
      expect(otherDivergedAtAfter).toBe(otherDivergedAtBefore);
      expect(otherWorkspaceChunk).toBeUndefined();
    });
  },
);
