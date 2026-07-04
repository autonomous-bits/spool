/**
 * Adapter-level integration test proving:
 *
 * 1. `ConflictGatedMergeService` refuses to merge a branch that has a real,
 *    Postgres-detected conflict (a chunk changed independently on both
 *    branch and mainline since divergence), and succeeds when there is no
 *    conflict — closing the gap found during rubber-duck review of
 *    Feature 01/02 against Meridian, where no production merge path
 *    enforced the feature-01 "merge branch" protected-operation contract's
 *    conflict checks.
 * 2. The append-only `chunk_history` table lets a stakeholder reconstruct
 *    every branch that ever merged a change for one idea label, even after
 *    a later, unrelated branch's merge overwrites `chunks.origin_branch_id`
 *    for that same label (technical spec §"Pre-merge history
 *    reconstruction", `IDEA-69`).
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
import { ConflictDetectionRepository } from '../src/persistence/conflict-detection.repository.js';
import { ConflictGatedMergeService } from '../src/persistence/conflict-gated-merge.service.js';
import { chunkLifecycleStatus } from '../src/domain/chunk-lifecycle.js';
import { humanActor } from '../src/domain/types/actor/actor-context.js';
import { ideaLabel, stakeholderId, workspaceId, type BranchId } from '../src/domain/types/index.js';

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
  'ConflictGatedMergeService + chunk_history (Postgres adapter)',
  () => {
    const ws = workspaceId(`ws-${randomUUID()}`);
    const actor = humanActor(stakeholderId(`stakeholder-${randomUUID()}`));

    beforeAll(async () => {
      const bootstrapPool = openPool();
      await ensureSchema(bootstrapPool);
      await bootstrapPool.end();
    });

    afterAll(async () => {
      const cleanupPool = openPool();
      await cleanupPool.query('DELETE FROM branches WHERE workspace_id = $1', [ws]);
      await cleanupPool.query('DELETE FROM branch_chunk_deltas WHERE workspace_id = $1', [ws]);
      await cleanupPool.query('DELETE FROM chunks WHERE workspace_id = $1', [ws]);
      await cleanupPool.query('DELETE FROM chunk_history WHERE workspace_id = $1', [ws]);
      await cleanupPool.end();
    });

    it('refuses to merge a branch with a real conflict, and never calls the underlying MergeRepository', async () => {
      const pool = openPool();
      const chunks = new ChunkGraphRepository(pool);
      const branches = new BranchGraphRepository(pool, chunks);
      const conflicts = new ConflictDetectionRepository(pool);
      const merges = new MergeRepository(pool, chunks);
      const service = new ConflictGatedMergeService(conflicts, merges);

      const branchId = `branch-${randomUUID()}` as BranchId;
      const label = ideaLabel('IDEA-conflict-gate-refuses');

      await conflicts.registerBranch(ws, branchId, 'engineering');
      // Mainline changes the same idea label *after* the branch diverged...
      await chunks.saveChunk({
        workspaceId: ws,
        ideaLabel: label,
        chunkType: 'feature',
        discipline: 'engineering',
        contextKind: 'permanent',
        content: 'Mainline changed this after divergence.',
        status: chunkLifecycleStatus('approved', 'active'),
      });
      // ...while the branch independently also changed it.
      await branches.saveChunkDelta({
        workspaceId: ws,
        branchId,
        ideaLabel: label,
        deltaKind: 'upsert',
        chunk: {
          workspaceId: ws,
          ideaLabel: label,
          chunkType: 'feature',
          discipline: 'engineering',
          contextKind: 'permanent',
          content: 'Branch changed this independently.',
          status: chunkLifecycleStatus('draft', 'active'),
        },
      });
      await merges.submitBranch(ws, branchId, actor, 'engineering');
      await merges.verifyBranch(ws, branchId, actor);

      await expect(service.mergeBranch(ws, branchId, actor)).rejects.toMatchObject({
        code: 'lineage-violation',
      });

      const statusAfterRefusal = await merges.getBranchStatus(ws, branchId);
      await pool.end();

      // A refused merge must not flip the branch to `merged` — the
      // underlying `MergeRepository.mergeBranch` (and its transaction) was
      // never invoked.
      expect(statusAfterRefusal).toBe('verified');
    });

    it('merges a clean (conflict-free) branch exactly like MergeRepository.mergeBranch would', async () => {
      const pool = openPool();
      const chunks = new ChunkGraphRepository(pool);
      const branches = new BranchGraphRepository(pool, chunks);
      const conflicts = new ConflictDetectionRepository(pool);
      const merges = new MergeRepository(pool, chunks);
      const service = new ConflictGatedMergeService(conflicts, merges);

      const branchId = `branch-${randomUUID()}` as BranchId;
      const label = ideaLabel('IDEA-conflict-gate-clean');

      await conflicts.registerBranch(ws, branchId, 'engineering');
      await branches.saveChunkDelta({
        workspaceId: ws,
        branchId,
        ideaLabel: label,
        deltaKind: 'upsert',
        chunk: {
          workspaceId: ws,
          ideaLabel: label,
          chunkType: 'feature',
          discipline: 'engineering',
          contextKind: 'permanent',
          content: 'Clean branch content.',
          status: chunkLifecycleStatus('draft', 'active'),
        },
      });
      await merges.submitBranch(ws, branchId, actor, 'engineering');
      await merges.verifyBranch(ws, branchId, actor);

      const outcome = await service.mergeBranch(ws, branchId, actor);
      const mergedChunk = await chunks.findChunk(ws, label);
      await pool.end();

      expect(outcome.mergedChunkLabels).toContain(label);
      expect(mergedChunk?.content).toBe('Clean branch content.');
      expect(mergedChunk?.originBranchId).toBe(branchId);
    });

    it('chunk_history: two sequential merges of the same idea label by different branches both remain independently visible, even though chunks.origin_branch_id only reflects the latest', async () => {
      const pool = openPool();
      const chunks = new ChunkGraphRepository(pool);
      const branches = new BranchGraphRepository(pool, chunks);
      const conflicts = new ConflictDetectionRepository(pool);
      const merges = new MergeRepository(pool, chunks);
      const label = ideaLabel('IDEA-chunk-history-sequential');

      const branchOne = `branch-${randomUUID()}` as BranchId;
      await conflicts.registerBranch(ws, branchOne, 'engineering');
      await branches.saveChunkDelta({
        workspaceId: ws,
        branchId: branchOne,
        ideaLabel: label,
        deltaKind: 'upsert',
        chunk: {
          workspaceId: ws,
          ideaLabel: label,
          chunkType: 'feature',
          discipline: 'engineering',
          contextKind: 'permanent',
          content: 'First branch content.',
          status: chunkLifecycleStatus('draft', 'active'),
        },
      });
      await merges.submitBranch(ws, branchOne, actor, 'engineering');
      await merges.verifyBranch(ws, branchOne, actor);
      await merges.mergeBranch(ws, branchOne, actor);

      const branchTwo = `branch-${randomUUID()}` as BranchId;
      await conflicts.registerBranch(ws, branchTwo, 'engineering');
      await branches.saveChunkDelta({
        workspaceId: ws,
        branchId: branchTwo,
        ideaLabel: label,
        deltaKind: 'upsert',
        chunk: {
          workspaceId: ws,
          ideaLabel: label,
          chunkType: 'feature',
          discipline: 'engineering',
          contextKind: 'permanent',
          content: 'Second branch content.',
          status: chunkLifecycleStatus('draft', 'active'),
        },
      });
      await merges.submitBranch(ws, branchTwo, actor, 'engineering');
      await merges.verifyBranch(ws, branchTwo, actor);
      await merges.mergeBranch(ws, branchTwo, actor);

      const mainlineChunk = await chunks.findChunk(ws, label);
      const history = await merges.listChunkHistoryByIdeaLabel(ws, label);
      await pool.end();

      // The mutable `chunks` row is last-writer-wins: only branchTwo's
      // provenance is visible there.
      expect(mainlineChunk?.originBranchId).toBe(branchTwo);
      expect(mainlineChunk?.content).toBe('Second branch content.');

      // ...but the append-only history preserves both merges, in order,
      // with branchOne's contribution still fully reconstructable.
      expect(history).toHaveLength(2);
      expect(history[0]?.originBranchId).toBe(branchOne);
      expect(history[0]?.content).toBe('First branch content.');
      expect(history[1]?.originBranchId).toBe(branchTwo);
      expect(history[1]?.content).toBe('Second branch content.');
    });
  },
);
