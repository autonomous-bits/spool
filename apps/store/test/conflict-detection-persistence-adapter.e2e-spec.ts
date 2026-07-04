/**
 * Adapter-level integration test proving pre-merge conflict detection
 * (story S06) persists divergence markers correctly and reports mainline
 * changes / branch-mainline conflicts against a real containerized
 * Postgres.
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
import { ConflictDetectionRepository } from '../src/persistence/conflict-detection.repository.js';
import { ChunkGraphRepository } from '../src/persistence/chunk-graph.repository.js';
import { BranchGraphRepository } from '../src/persistence/branch-graph.repository.js';
import { ArtifactAssociationRepository } from '../src/persistence/artifact-association.repository.js';
import { chunkLifecycleStatus } from '../src/domain/chunk-lifecycle.js';
import { createEdge } from '../src/domain/edge-lineage.js';
import { BranchLifecycleError } from '../src/domain/branch-lifecycle.js';
import {
  artifactId,
  ideaLabel,
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
  'ConflictDetectionRepository (Postgres adapter, pre-merge conflict detection)',
  () => {
    const workspaceA = workspaceId(`ws-${randomUUID()}`);
    const workspaceB = workspaceId(`ws-${randomUUID()}`);

    beforeAll(async () => {
      const bootstrapPool = openPool();
      await ensureSchema(bootstrapPool);
      await bootstrapPool.end();
    });

    afterAll(async () => {
      const cleanupPool = openPool();
      await cleanupPool.query(
        'DELETE FROM branches WHERE workspace_id = $1 OR workspace_id = $2',
        [workspaceA, workspaceB],
      );
      await cleanupPool.query(
        'DELETE FROM branch_chunk_deltas WHERE workspace_id = $1 OR workspace_id = $2',
        [workspaceA, workspaceB],
      );
      await cleanupPool.query(
        'DELETE FROM branch_edge_deltas WHERE workspace_id = $1 OR workspace_id = $2',
        [workspaceA, workspaceB],
      );
      await cleanupPool.query(
        'DELETE FROM chunks WHERE workspace_id = $1 OR workspace_id = $2',
        [workspaceA, workspaceB],
      );
      await cleanupPool.query(
        'DELETE FROM edge_versions WHERE workspace_id = $1 OR workspace_id = $2',
        [workspaceA, workspaceB],
      );
      await cleanupPool.query(
        'DELETE FROM chunk_artifacts WHERE workspace_id = $1 OR workspace_id = $2',
        [workspaceA, workspaceB],
      );
      await cleanupPool.end();
    });

    it('AC1: reports a mainline chunk change that happened after a branch diverged', async () => {
      const pool = openPool();
      const conflicts = new ConflictDetectionRepository(pool);
      const chunks = new ChunkGraphRepository(pool);
      const branchId = `branch-${randomUUID()}` as BranchId;
      const label = ideaLabel('IDEA-conflict-ac1');

      await conflicts.registerBranch(workspaceA, branchId, 'engineering');
      await chunks.saveChunk({
        workspaceId: workspaceA,
        ideaLabel: label,
        chunkType: 'feature',
        discipline: 'engineering',
        contextKind: 'permanent',
        content: 'Mainline changed this after divergence.',
        status: chunkLifecycleStatus('approved', 'active'),
      });

      const report = await conflicts.listMainlineChangesSinceDivergence(workspaceA, branchId);
      await pool.end();

      expect(report.chunkChanges.some((c) => c.ideaLabel === label)).toBe(true);
    });

    it('AC2: a chunk changed independently on both branch and mainline since divergence is a conflict', async () => {
      const pool = openPool();
      const conflicts = new ConflictDetectionRepository(pool);
      const chunks = new ChunkGraphRepository(pool);
      const branches = new BranchGraphRepository(pool, chunks);
      const branchId = `branch-${randomUUID()}` as BranchId;
      const label = ideaLabel('IDEA-conflict-ac2-both');

      await conflicts.registerBranch(workspaceA, branchId, 'engineering');
      await chunks.saveChunk({
        workspaceId: workspaceA,
        ideaLabel: label,
        chunkType: 'feature',
        discipline: 'engineering',
        contextKind: 'permanent',
        content: 'Mainline version.',
        status: chunkLifecycleStatus('approved', 'active'),
      });
      await branches.saveChunkDelta({
        workspaceId: workspaceA,
        branchId,
        ideaLabel: label,
        deltaKind: 'upsert',
        chunk: {
          workspaceId: workspaceA,
          ideaLabel: label,
          chunkType: 'feature',
          discipline: 'engineering',
          contextKind: 'permanent',
          content: 'Branch version.',
          status: chunkLifecycleStatus('draft', 'active'),
        },
      });

      const report = await conflicts.detectConflicts(workspaceA, branchId);
      await pool.end();

      expect(report.chunkConflicts.some((c) => c.ideaLabel === label)).toBe(true);
    });

    it('AC2: a mainline-only chunk change (branch never touched it) is not a conflict', async () => {
      const pool = openPool();
      const conflicts = new ConflictDetectionRepository(pool);
      const chunks = new ChunkGraphRepository(pool);
      const branchId = `branch-${randomUUID()}` as BranchId;
      const label = ideaLabel('IDEA-conflict-ac2-mainline-only');

      await conflicts.registerBranch(workspaceA, branchId, 'engineering');
      await chunks.saveChunk({
        workspaceId: workspaceA,
        ideaLabel: label,
        chunkType: 'feature',
        discipline: 'engineering',
        contextKind: 'permanent',
        content: 'Only mainline changed this.',
        status: chunkLifecycleStatus('approved', 'active'),
      });

      const mainlineChanges = await conflicts.listMainlineChangesSinceDivergence(
        workspaceA,
        branchId,
      );
      const report = await conflicts.detectConflicts(workspaceA, branchId);
      await pool.end();

      expect(mainlineChanges.chunkChanges.some((c) => c.ideaLabel === label)).toBe(true);
      expect(report.chunkConflicts.some((c) => c.ideaLabel === label)).toBe(false);
    });

    it("AC2: a branch-only chunk change (mainline untouched) is not a conflict", async () => {
      const pool = openPool();
      const conflicts = new ConflictDetectionRepository(pool);
      const chunks = new ChunkGraphRepository(pool);
      const branches = new BranchGraphRepository(pool, chunks);
      const branchId = `branch-${randomUUID()}` as BranchId;
      const label = ideaLabel('IDEA-conflict-ac2-branch-only');

      await conflicts.registerBranch(workspaceA, branchId, 'engineering');
      await branches.saveChunkDelta({
        workspaceId: workspaceA,
        branchId,
        ideaLabel: label,
        deltaKind: 'upsert',
        chunk: {
          workspaceId: workspaceA,
          ideaLabel: label,
          chunkType: 'feature',
          discipline: 'engineering',
          contextKind: 'permanent',
          content: 'Branch-only addition.',
          status: chunkLifecycleStatus('draft', 'active'),
        },
      });

      const report = await conflicts.detectConflicts(workspaceA, branchId);
      await pool.end();

      expect(report.chunkConflicts.some((c) => c.ideaLabel === label)).toBe(false);
    });

    it('AC2: an edge relationship-type replacement on mainline conflicts with a branch-side edge delta for the same pair', async () => {
      const pool = openPool();
      const conflicts = new ConflictDetectionRepository(pool);
      const chunks = new ChunkGraphRepository(pool);
      const branches = new BranchGraphRepository(pool, chunks);
      const branchId = `branch-${randomUUID()}` as BranchId;
      const source = ideaLabel('IDEA-conflict-edge-source');
      const target = ideaLabel('IDEA-conflict-edge-target');

      await conflicts.registerBranch(workspaceA, branchId, 'engineering');
      await chunks.saveEdgeLineage(createEdge(workspaceA, source, target, 'depends-on'));
      await chunks.replaceEdgeRelationshipType(workspaceA, source, target, 'depends-on', 'refines');
      await branches.saveEdgeDelta({
        workspaceId: workspaceA,
        branchId,
        sourceLabel: source,
        targetLabel: target,
        relationshipType: 'depends-on',
        deltaKind: 'deactivate',
      });

      const report = await conflicts.detectConflicts(workspaceA, branchId);
      await pool.end();

      expect(
        report.edgeConflicts.some(
          (e) =>
            e.sourceLabel === source &&
            e.targetLabel === target &&
            e.relationshipType === 'depends-on',
        ),
      ).toBe(true);
      // Grouped by the full (source, target, relationshipType) identity —
      // matching branch_edge_deltas' own identity — so the type replacement
      // (which appends rows under two different relationship_type values:
      // the old type's deactivation and the new type's creation) surfaces as
      // exactly one conflict entry for the *old* identity the branch
      // actually touched, not duplicated and not collapsed with the
      // unrelated new-type identity.
      expect(
        report.edgeConflicts.filter(
          (e) => e.sourceLabel === source && e.targetLabel === target,
        ),
      ).toHaveLength(1);
    });

    it('AC2: different relationship types between the same two ideas are independent identities — a mainline change to one type does not conflict with a branch change to another', async () => {
      const pool = openPool();
      const conflicts = new ConflictDetectionRepository(pool);
      const chunks = new ChunkGraphRepository(pool);
      const branches = new BranchGraphRepository(pool, chunks);
      const branchId = `branch-${randomUUID()}` as BranchId;
      const source = ideaLabel('IDEA-conflict-edge-type-source');
      const target = ideaLabel('IDEA-conflict-edge-type-target');

      await conflicts.registerBranch(workspaceA, branchId, 'engineering');
      // Mainline independently changes a 'depends-on' edge between the pair...
      await chunks.saveEdgeLineage(createEdge(workspaceA, source, target, 'depends-on'));
      // ...while the branch adds its own, unrelated 'refines' edge for the
      // same pair. These are different relationship identities and must not
      // be reported as conflicting with each other.
      await branches.saveEdgeDelta({
        workspaceId: workspaceA,
        branchId,
        sourceLabel: source,
        targetLabel: target,
        relationshipType: 'refines',
        deltaKind: 'upsert',
      });

      const report = await conflicts.detectConflicts(workspaceA, branchId);
      await pool.end();

      expect(
        report.edgeConflicts.some((e) => e.sourceLabel === source && e.targetLabel === target),
      ).toBe(false);
    });

    it('AC2: a chunk-artifact association changed on both sides is a conflict, reported once (not duplicated across versions)', async () => {
      const pool = openPool();
      const conflicts = new ConflictDetectionRepository(pool);
      const chunks = new ChunkGraphRepository(pool);
      const branches = new BranchGraphRepository(pool, chunks);
      const associations = new ArtifactAssociationRepository(pool);
      const branchId = `branch-${randomUUID()}` as BranchId;
      const chunkLabel = ideaLabel('IDEA-conflict-artifact');
      const artifact = artifactId(`artifact-${randomUUID()}`);

      await conflicts.registerBranch(workspaceA, branchId, 'engineering');
      await associations.createAssociation({
        workspaceId: workspaceA,
        chunkLabel,
        artifactId: artifact,
      });
      await associations.deactivateAssociation(workspaceA, chunkLabel, artifact);
      await associations.createAssociation({
        workspaceId: workspaceA,
        chunkLabel,
        artifactId: artifact,
        branchId,
      });

      const report = await conflicts.detectConflicts(workspaceA, branchId);
      await pool.end();

      const matches = report.artifactAssociationConflicts.filter(
        (a) => a.chunkLabel === chunkLabel && a.artifactId === artifact,
      );
      expect(matches).toHaveLength(1);
    });

    it('AC3: confirming catch-up advances the divergence marker so a previously-conflicting mainline change no longer appears', async () => {
      const pool = openPool();
      const conflicts = new ConflictDetectionRepository(pool);
      const chunks = new ChunkGraphRepository(pool);
      const branches = new BranchGraphRepository(pool, chunks);
      const branchId = `branch-${randomUUID()}` as BranchId;
      const label = ideaLabel('IDEA-conflict-ac3-catchup');

      const divergedAt = await conflicts.registerBranch(workspaceA, branchId, 'engineering');
      await chunks.saveChunk({
        workspaceId: workspaceA,
        ideaLabel: label,
        chunkType: 'feature',
        discipline: 'engineering',
        contextKind: 'permanent',
        content: 'Mainline change before catch-up.',
        status: chunkLifecycleStatus('approved', 'active'),
      });
      await branches.saveChunkDelta({
        workspaceId: workspaceA,
        branchId,
        ideaLabel: label,
        deltaKind: 'upsert',
        chunk: {
          workspaceId: workspaceA,
          ideaLabel: label,
          chunkType: 'feature',
          discipline: 'engineering',
          contextKind: 'permanent',
          content: 'Branch integrated the mainline change.',
          status: chunkLifecycleStatus('draft', 'active'),
        },
      });

      const beforeCatchUp = await conflicts.detectConflicts(workspaceA, branchId);
      expect(beforeCatchUp.chunkConflicts.some((c) => c.ideaLabel === label)).toBe(true);

      const newDivergedAt = await conflicts.confirmCatchUp(workspaceA, branchId);
      const afterCatchUp = await conflicts.detectConflicts(workspaceA, branchId);
      await pool.end();

      expect(Date.parse(newDivergedAt)).toBeGreaterThan(Date.parse(divergedAt));
      expect(afterCatchUp.divergedAt).toBe(newDivergedAt);
      expect(afterCatchUp.chunkConflicts.some((c) => c.ideaLabel === label)).toBe(false);
    });

    it('not-found: operating on an unregistered branch is rejected', async () => {
      const pool = openPool();
      const conflicts = new ConflictDetectionRepository(pool);
      const branchId = `branch-${randomUUID()}` as BranchId;

      await expect(conflicts.getDivergedAt(workspaceA, branchId)).rejects.toBeInstanceOf(
        BranchLifecycleError,
      );
      await expect(
        conflicts.listMainlineChangesSinceDivergence(workspaceA, branchId),
      ).rejects.toMatchObject({ code: 'not-found' });
      await expect(conflicts.detectConflicts(workspaceA, branchId)).rejects.toMatchObject({
        code: 'not-found',
      });
      await expect(conflicts.confirmCatchUp(workspaceA, branchId)).rejects.toMatchObject({
        code: 'not-found',
      });
      await pool.end();
    });

    it('registering the same branch identity twice is rejected', async () => {
      const pool = openPool();
      const conflicts = new ConflictDetectionRepository(pool);
      const branchId = `branch-${randomUUID()}` as BranchId;

      await conflicts.registerBranch(workspaceA, branchId, 'engineering');
      await expect(
        conflicts.registerBranch(workspaceA, branchId, 'engineering'),
      ).rejects.toMatchObject({ code: 'invalid-state-transition' });
      await pool.end();
    });

    it("tenant isolation: a mainline change in one workspace never appears in another workspace's conflict report", async () => {
      const pool = openPool();
      const conflicts = new ConflictDetectionRepository(pool);
      const chunks = new ChunkGraphRepository(pool);
      const branchId = `branch-${randomUUID()}` as BranchId;
      const label = ideaLabel('IDEA-conflict-tenant-isolation');

      await conflicts.registerBranch(workspaceA, branchId, 'engineering');
      await chunks.saveChunk({
        workspaceId: workspaceB,
        ideaLabel: label,
        chunkType: 'feature',
        discipline: 'engineering',
        contextKind: 'permanent',
        content: 'Different workspace entirely.',
        status: chunkLifecycleStatus('approved', 'active'),
      });

      const report = await conflicts.listMainlineChangesSinceDivergence(workspaceA, branchId);
      await pool.end();

      expect(report.chunkChanges.some((c) => c.ideaLabel === label)).toBe(false);
    });
  },
);
