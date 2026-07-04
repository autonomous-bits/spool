/**
 * Adapter-level integration test proving branch-scoped chunk/edge deltas
 * (story S02) persist separately from mainline records and resolve into a
 * correct branch view at read time, against a real containerized Postgres.
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
import { ChunkGraphRepository } from '../src/persistence/chunk-graph.repository.js';
import { BranchGraphRepository } from '../src/persistence/branch-graph.repository.js';
import { ConflictDetectionRepository } from '../src/persistence/conflict-detection.repository.js';
import { chunkLifecycleStatus } from '../src/domain/chunk-lifecycle.js';
import { createEdge, currentEdgeVersion } from '../src/domain/edge-lineage.js';
import {
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
  'BranchGraphRepository (Postgres adapter, branch-scoped delta resolution)',
  () => {
    const workspaceA = workspaceId(`ws-${randomUUID()}`);
    const workspaceB = workspaceId(`ws-${randomUUID()}`);
    const branchA = `branch-${randomUUID()}` as BranchId;
    const branchB = `branch-${randomUUID()}` as BranchId;
    const label1 = ideaLabel('IDEA-branch-checkout-flow');
    const label2 = ideaLabel('IDEA-branch-payment-gateway');
    const label3 = ideaLabel('IDEA-branch-only-addition');

    beforeAll(async () => {
      const bootstrapPool = openPool();
      await ensureSchema(bootstrapPool);
      // Story S11: branch-scoped chunk/edge deltas now enforce the
      // registered branch's write-lock and discipline boundary, so both
      // branches used across this file's tests must be registered (as
      // 'engineering', matching every delta chunk's discipline below)
      // before any saveChunkDelta/saveEdgeDelta call.
      const conflicts = new ConflictDetectionRepository(bootstrapPool);
      await conflicts.registerBranch(workspaceA, branchA, 'engineering');
      await conflicts.registerBranch(workspaceA, branchB, 'engineering');
      await conflicts.registerBranch(workspaceB, branchA, 'engineering');
      await bootstrapPool.end();
    });

    afterAll(async () => {
      const cleanupPool = openPool();
      await cleanupPool.query(
        'DELETE FROM chunks WHERE workspace_id = $1 OR workspace_id = $2',
        [workspaceA, workspaceB],
      );
      await cleanupPool.query(
        'DELETE FROM edge_versions WHERE workspace_id = $1 OR workspace_id = $2',
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
        'DELETE FROM branches WHERE workspace_id = $1 OR workspace_id = $2',
        [workspaceA, workspaceB],
      );
      await cleanupPool.end();
    });

    it('AC1/AC3: mainline chunk/edge reads are unaffected by a branch draft (approved context stays approved-only)', async () => {
      const pool = openPool();
      const chunkRepo = new ChunkGraphRepository(pool);
      const branchRepo = new BranchGraphRepository(pool, chunkRepo);

      await chunkRepo.saveChunk({
        workspaceId: workspaceA,
        ideaLabel: label1,
        chunkType: 'feature',
        discipline: 'engineering',
        contextKind: 'permanent',
        content: 'Approved mainline checkout flow.',
        status: chunkLifecycleStatus('approved', 'active'),
      });

      // Branch drafts an override of the same idea label.
      await branchRepo.saveChunkDelta({
        workspaceId: workspaceA,
        branchId: branchA,
        ideaLabel: label1,
        deltaKind: 'upsert',
        chunk: {
          workspaceId: workspaceA,
          ideaLabel: label1,
          chunkType: 'feature',
          discipline: 'engineering',
          contextKind: 'permanent',
          content: 'DRAFT: reworked checkout flow, not yet approved.',
          status: chunkLifecycleStatus('draft', 'active'),
        },
      });

      const mainlineOnly = await chunkRepo.findChunk(workspaceA, label1);
      const mainlineList = await chunkRepo.listChunks(workspaceA);
      await pool.end();

      expect(mainlineOnly?.content).toBe('Approved mainline checkout flow.');
      expect(
        mainlineList.some((c) => c.content.startsWith('DRAFT:')),
      ).toBe(false);
    });

    it("AC2: a branch's resolved view combines its own deltas with mainline, without mutating mainline", async () => {
      const pool = openPool();
      const chunkRepo = new ChunkGraphRepository(pool);
      const branchRepo = new BranchGraphRepository(pool, chunkRepo);

      await chunkRepo.saveChunk({
        workspaceId: workspaceA,
        ideaLabel: label2,
        chunkType: 'capability',
        discipline: 'engineering',
        contextKind: 'permanent',
        content: 'Approved payment gateway integration.',
        status: chunkLifecycleStatus('approved', 'active'),
      });

      // Branch A (registered discipline: 'engineering'): overrides label2,
      // adds label3 (pure addition). Story S11: a branch may only modify a
      // chunk owned by its own discipline (technical spec §"Discipline
      // boundary"), so label2's mainline discipline above must match
      // branchA's registered discipline for this override to be permitted.
      await branchRepo.saveChunkDelta({
        workspaceId: workspaceA,
        branchId: branchA,
        ideaLabel: label2,
        deltaKind: 'upsert',
        chunk: {
          workspaceId: workspaceA,
          ideaLabel: label2,
          chunkType: 'capability',
          discipline: 'engineering',
          contextKind: 'permanent',
          content: 'DRAFT: expanded payment gateway integration.',
          status: chunkLifecycleStatus('draft', 'active'),
        },
      });
      await branchRepo.saveChunkDelta({
        workspaceId: workspaceA,
        branchId: branchA,
        ideaLabel: label3,
        deltaKind: 'upsert',
        chunk: {
          workspaceId: workspaceA,
          ideaLabel: label3,
          chunkType: 'feature',
          discipline: 'engineering',
          contextKind: 'permanent',
          content: 'Branch-only new idea, never in mainline.',
          status: chunkLifecycleStatus('draft', 'active'),
        },
      });

      const resolvedForBranchA = await branchRepo.resolveChunks(workspaceA, branchA);
      const mainlineAfterResolve = await chunkRepo.listChunks(workspaceA);
      await pool.end();

      const resolvedLabel2 = resolvedForBranchA.find((c) => c.ideaLabel === label2);
      const resolvedLabel3 = resolvedForBranchA.find((c) => c.ideaLabel === label3);
      expect(resolvedLabel2?.content).toBe(
        'DRAFT: expanded payment gateway integration.',
      );
      expect(resolvedLabel3?.content).toBe(
        'Branch-only new idea, never in mainline.',
      );
      // Mainline row for label2 remains exactly what was approved.
      expect(
        mainlineAfterResolve.find((c) => c.ideaLabel === label2)?.content,
      ).toBe('Approved payment gateway integration.');
    });

    it("AC3: a different branch's deltas never leak into another branch's or the mainline's resolved view", async () => {
      const pool = openPool();
      const chunkRepo = new ChunkGraphRepository(pool);
      const branchRepo = new BranchGraphRepository(pool, chunkRepo);

      await branchRepo.saveChunkDelta({
        workspaceId: workspaceA,
        branchId: branchB,
        ideaLabel: label1,
        deltaKind: 'upsert',
        chunk: {
          workspaceId: workspaceA,
          ideaLabel: label1,
          chunkType: 'feature',
          discipline: 'engineering',
          contextKind: 'permanent',
          content: "DRAFT from branch B, should not appear in branch A's view.",
          status: chunkLifecycleStatus('draft', 'active'),
        },
      });

      const resolvedForBranchA = await branchRepo.resolveChunks(workspaceA, branchA);
      const mainlineList = await chunkRepo.listChunks(workspaceA);
      await pool.end();

      expect(
        resolvedForBranchA.some((c) => c.content.includes('branch B')),
      ).toBe(false);
      expect(mainlineList.some((c) => c.content.includes('branch B'))).toBe(
        false,
      );
    });

    it("AC2: a branch 'delete' delta hides a mainline chunk from its resolved view without touching mainline", async () => {
      const pool = openPool();
      const chunkRepo = new ChunkGraphRepository(pool);
      const branchRepo = new BranchGraphRepository(pool, chunkRepo);

      await branchRepo.saveChunkDelta({
        workspaceId: workspaceA,
        branchId: branchA,
        ideaLabel: label1,
        deltaKind: 'delete',
      });

      const resolvedForBranchA = await branchRepo.resolveChunks(workspaceA, branchA);
      const mainlineStillThere = await chunkRepo.findChunk(workspaceA, label1);
      await pool.end();

      expect(resolvedForBranchA.some((c) => c.ideaLabel === label1)).toBe(false);
      expect(mainlineStillThere?.content).toBe(
        'Approved mainline checkout flow.',
      );
    });

    it('AC2: edge deltas resolve additions and branch-scoped deactivations without mutating mainline edge lineages', async () => {
      const pool = openPool();
      const chunkRepo = new ChunkGraphRepository(pool);
      const branchRepo = new BranchGraphRepository(pool, chunkRepo);

      // Mainline: an active edge that the branch will deactivate in its view.
      await chunkRepo.saveEdgeLineage(
        createEdge(workspaceA, label1, label2, 'depends-on'),
      );
      await branchRepo.saveEdgeDelta({
        workspaceId: workspaceA,
        branchId: branchA,
        sourceLabel: label1,
        targetLabel: label2,
        relationshipType: 'depends-on',
        deltaKind: 'deactivate',
      });

      // Branch-only addition: an edge that never existed on mainline.
      await branchRepo.saveEdgeDelta({
        workspaceId: workspaceA,
        branchId: branchA,
        sourceLabel: label2,
        targetLabel: label3,
        relationshipType: 'informs',
        deltaKind: 'upsert',
      });

      const resolved = await branchRepo.resolveEdgeLineages(workspaceA, branchA);
      const mainlineLineages = await chunkRepo.listEdgeLineages(workspaceA);
      await pool.end();

      const resolvedDependsOn = resolved.find(
        (l) => currentEdgeVersion(l).relationshipType === 'depends-on',
      );
      const resolvedInforms = resolved.find(
        (l) => currentEdgeVersion(l).relationshipType === 'informs',
      );

      expect(currentEdgeVersion(resolvedDependsOn!).state).toBe('deactivated');
      expect(currentEdgeVersion(resolvedInforms!).state).toBe('active');

      // Mainline lineage for depends-on remains active — the branch's
      // deactivation never touched it.
      const mainlineDependsOn = mainlineLineages.find(
        (l) => currentEdgeVersion(l).relationshipType === 'depends-on',
      );
      expect(currentEdgeVersion(mainlineDependsOn!).state).toBe('active');
      // The branch-only addition must not appear as a mainline lineage.
      expect(
        mainlineLineages.some(
          (l) => currentEdgeVersion(l).relationshipType === 'informs',
        ),
      ).toBe(false);
    });

    it("AC3: a different branch's edge deltas never leak into another branch's resolved edge view", async () => {
      const pool = openPool();
      const chunkRepo = new ChunkGraphRepository(pool);
      const branchRepo = new BranchGraphRepository(pool, chunkRepo);
      const onlyOnBranchB = ideaLabel('IDEA-branch-b-edge-source');

      await branchRepo.saveEdgeDelta({
        workspaceId: workspaceA,
        branchId: branchB,
        sourceLabel: onlyOnBranchB,
        targetLabel: label3,
        relationshipType: 'refines',
        deltaKind: 'upsert',
      });

      const resolvedForBranchA = await branchRepo.resolveEdgeLineages(
        workspaceA,
        branchA,
      );
      const mainlineLineages = await chunkRepo.listEdgeLineages(workspaceA);
      await pool.end();

      expect(
        resolvedForBranchA.some(
          (l) => currentEdgeVersion(l).relationshipType === 'refines',
        ),
      ).toBe(false);
      expect(
        mainlineLineages.some(
          (l) => currentEdgeVersion(l).relationshipType === 'refines',
        ),
      ).toBe(false);
    });

    it("tenant isolation: an edge delta saved under one workspace never resolves into another workspace's branch view", async () => {
      const pool = openPool();
      const chunkRepo = new ChunkGraphRepository(pool);
      const branchRepo = new BranchGraphRepository(pool, chunkRepo);
      const onlyOnWorkspaceB = ideaLabel('IDEA-workspace-b-edge-source');

      await branchRepo.saveEdgeDelta({
        workspaceId: workspaceB,
        branchId: branchA,
        sourceLabel: onlyOnWorkspaceB,
        targetLabel: label3,
        relationshipType: 'supersedes',
        deltaKind: 'upsert',
      });

      const resolvedForWorkspaceABranchA = await branchRepo.resolveEdgeLineages(
        workspaceA,
        branchA,
      );
      await pool.end();

      expect(
        resolvedForWorkspaceABranchA.some(
          (l) => currentEdgeVersion(l).relationshipType === 'supersedes',
        ),
      ).toBe(false);
    });

    it('tenant isolation: branch deltas saved under one workspace never resolve into another workspace', async () => {
      const pool = openPool();
      const chunkRepo = new ChunkGraphRepository(pool);
      const branchRepo = new BranchGraphRepository(pool, chunkRepo);

      await branchRepo.saveChunkDelta({
        workspaceId: workspaceB,
        branchId: branchA,
        ideaLabel: label1,
        deltaKind: 'upsert',
        chunk: {
          workspaceId: workspaceB,
          ideaLabel: label1,
          chunkType: 'feature',
          discipline: 'engineering',
          contextKind: 'permanent',
          content: 'workspace B only content',
          status: chunkLifecycleStatus('draft', 'active'),
        },
      });

      const resolvedForWorkspaceABranchA = await branchRepo.resolveChunks(
        workspaceA,
        branchA,
      );
      await pool.end();

      expect(
        resolvedForWorkspaceABranchA.some(
          (c) => c.content === 'workspace B only content',
        ),
      ).toBe(false);
    });

    it('story S11: a chunk/edge delta against an unregistered branch throws BranchLifecycleError(not-found)', async () => {
      const pool = openPool();
      const chunkRepo = new ChunkGraphRepository(pool);
      const branchRepo = new BranchGraphRepository(pool, chunkRepo);
      const unregisteredBranch = `branch-${randomUUID()}` as BranchId;

      await expect(
        branchRepo.saveChunkDelta({
          workspaceId: workspaceA,
          branchId: unregisteredBranch,
          ideaLabel: label1,
          deltaKind: 'upsert',
          chunk: {
            workspaceId: workspaceA,
            ideaLabel: label1,
            chunkType: 'feature',
            discipline: 'engineering',
            contextKind: 'permanent',
            content: 'should never persist',
            status: chunkLifecycleStatus('draft', 'active'),
          },
        }),
      ).rejects.toMatchObject({ name: 'BranchLifecycleError', code: 'not-found' });

      await expect(
        branchRepo.saveEdgeDelta({
          workspaceId: workspaceA,
          branchId: unregisteredBranch,
          sourceLabel: label1,
          targetLabel: label2,
          relationshipType: 'depends-on',
          deltaKind: 'upsert',
        }),
      ).rejects.toMatchObject({ name: 'BranchLifecycleError', code: 'not-found' });

      await pool.end();
    });

    it('story S11: a chunk/edge delta against a non-draft (write-locked) branch throws BranchLifecycleError(write-locked)', async () => {
      const pool = openPool();
      const chunkRepo = new ChunkGraphRepository(pool);
      const branchRepo = new BranchGraphRepository(pool, chunkRepo);
      const lockedBranch = `branch-${randomUUID()}` as BranchId;
      const conflicts = new ConflictDetectionRepository(pool);

      await conflicts.registerBranch(workspaceA, lockedBranch, 'engineering');
      // Advance the branch past 'draft' directly, mirroring how
      // MergeRepository.submitBranch transitions status, without pulling in
      // the actor/notification machinery this test doesn't need.
      await pool.query(
        `UPDATE branches SET status = 'submitted' WHERE workspace_id = $1 AND branch_id = $2`,
        [workspaceA, lockedBranch],
      );

      await expect(
        branchRepo.saveChunkDelta({
          workspaceId: workspaceA,
          branchId: lockedBranch,
          ideaLabel: label1,
          deltaKind: 'upsert',
          chunk: {
            workspaceId: workspaceA,
            ideaLabel: label1,
            chunkType: 'feature',
            discipline: 'engineering',
            contextKind: 'permanent',
            content: 'should never persist',
            status: chunkLifecycleStatus('draft', 'active'),
          },
        }),
      ).rejects.toMatchObject({ name: 'BranchLifecycleError', code: 'write-locked' });

      await expect(
        branchRepo.saveEdgeDelta({
          workspaceId: workspaceA,
          branchId: lockedBranch,
          sourceLabel: label1,
          targetLabel: label2,
          relationshipType: 'depends-on',
          deltaKind: 'upsert',
        }),
      ).rejects.toMatchObject({ name: 'BranchLifecycleError', code: 'write-locked' });

      await pool.query('DELETE FROM branches WHERE workspace_id = $1 AND branch_id = $2', [
        workspaceA,
        lockedBranch,
      ]);
      await pool.end();
    });

    it('story S11: a brand-new chunk delta whose discipline differs from its branch throws BranchLifecycleError(branch-isolation-violation)', async () => {
      const pool = openPool();
      const chunkRepo = new ChunkGraphRepository(pool);
      const branchRepo = new BranchGraphRepository(pool, chunkRepo);
      const isolatedBranch = `branch-${randomUUID()}` as BranchId;
      const conflicts = new ConflictDetectionRepository(pool);
      const brandNewLabel = ideaLabel('IDEA-s11-brand-new-wrong-discipline');

      await conflicts.registerBranch(workspaceA, isolatedBranch, 'engineering');

      await expect(
        branchRepo.saveChunkDelta({
          workspaceId: workspaceA,
          branchId: isolatedBranch,
          ideaLabel: brandNewLabel,
          deltaKind: 'upsert',
          chunk: {
            workspaceId: workspaceA,
            ideaLabel: brandNewLabel,
            chunkType: 'capability',
            discipline: 'product', // mismatch: branch registered as 'engineering'
            contextKind: 'permanent',
            content: 'should never persist',
            status: chunkLifecycleStatus('draft', 'active'),
          },
        }),
      ).rejects.toMatchObject({
        name: 'BranchLifecycleError',
        code: 'branch-isolation-violation',
      });

      await pool.query('DELETE FROM branches WHERE workspace_id = $1 AND branch_id = $2', [
        workspaceA,
        isolatedBranch,
      ]);
      await pool.end();
    });

    it(
      'story S11: overriding an existing mainline chunk owned by a different discipline ' +
        "throws BranchLifecycleError(branch-isolation-violation), regardless of what " +
        "discipline the delta's own payload declares",
      async () => {
        const pool = openPool();
        const chunkRepo = new ChunkGraphRepository(pool);
        const branchRepo = new BranchGraphRepository(pool, chunkRepo);
        const isolatedBranch = `branch-${randomUUID()}` as BranchId;
        const conflicts = new ConflictDetectionRepository(pool);
        const productOwnedLabel = ideaLabel('IDEA-s11-product-owned-existing');

        await conflicts.registerBranch(workspaceA, isolatedBranch, 'engineering');
        await chunkRepo.saveChunk({
          workspaceId: workspaceA,
          ideaLabel: productOwnedLabel,
          chunkType: 'capability',
          discipline: 'product',
          contextKind: 'permanent',
          content: 'Mainline content owned by product.',
          status: chunkLifecycleStatus('approved', 'active'),
        });

        // Ownership is decided by the *existing* mainline chunk's
        // discipline ('product'), not by whatever the override payload
        // claims — an engineering branch cannot launder a product-owned
        // idea into an engineering one just by relabeling it in the delta.
        await expect(
          branchRepo.saveChunkDelta({
            workspaceId: workspaceA,
            branchId: isolatedBranch,
            ideaLabel: productOwnedLabel,
            deltaKind: 'upsert',
            chunk: {
              workspaceId: workspaceA,
              ideaLabel: productOwnedLabel,
              chunkType: 'capability',
              discipline: 'engineering', // payload claims 'engineering', matching the branch
              contextKind: 'permanent',
              content: 'should never persist',
              status: chunkLifecycleStatus('draft', 'active'),
            },
          }),
        ).rejects.toMatchObject({
          name: 'BranchLifecycleError',
          code: 'branch-isolation-violation',
        });

        // The same ownership rule applies to a 'delete' delta, which
        // carries no chunk payload at all to check against.
        await expect(
          branchRepo.saveChunkDelta({
            workspaceId: workspaceA,
            branchId: isolatedBranch,
            ideaLabel: productOwnedLabel,
            deltaKind: 'delete',
          }),
        ).rejects.toMatchObject({
          name: 'BranchLifecycleError',
          code: 'branch-isolation-violation',
        });

        await pool.query('DELETE FROM chunks WHERE workspace_id = $1 AND idea_label = $2', [
          workspaceA,
          productOwnedLabel,
        ]);
        await pool.query('DELETE FROM branches WHERE workspace_id = $1 AND branch_id = $2', [
          workspaceA,
          isolatedBranch,
        ]);
        await pool.end();
      },
    );

    it(
      'story S11: saveEdgeDelta rejects when the edge\'s source chunk is owned by a ' +
        'different discipline, even though the target chunk is untouched',
      async () => {
        const pool = openPool();
        const chunkRepo = new ChunkGraphRepository(pool);
        const branchRepo = new BranchGraphRepository(pool, chunkRepo);
        const isolatedBranch = `branch-${randomUUID()}` as BranchId;
        const conflicts = new ConflictDetectionRepository(pool);
        const productOwnedSource = ideaLabel('IDEA-s11-edge-product-owned-source');
        const anyTarget = ideaLabel('IDEA-s11-edge-any-target');

        await conflicts.registerBranch(workspaceA, isolatedBranch, 'engineering');
        await chunkRepo.saveChunk({
          workspaceId: workspaceA,
          ideaLabel: productOwnedSource,
          chunkType: 'capability',
          discipline: 'product',
          contextKind: 'permanent',
          content: 'Mainline source chunk owned by product.',
          status: chunkLifecycleStatus('approved', 'active'),
        });

        await expect(
          branchRepo.saveEdgeDelta({
            workspaceId: workspaceA,
            branchId: isolatedBranch,
            sourceLabel: productOwnedSource,
            targetLabel: anyTarget,
            relationshipType: 'depends-on',
            deltaKind: 'upsert',
          }),
        ).rejects.toMatchObject({
          name: 'BranchLifecycleError',
          code: 'branch-isolation-violation',
        });

        await pool.query('DELETE FROM chunks WHERE workspace_id = $1 AND idea_label = $2', [
          workspaceA,
          productOwnedSource,
        ]);
        await pool.query('DELETE FROM branches WHERE workspace_id = $1 AND branch_id = $2', [
          workspaceA,
          isolatedBranch,
        ]);
        await pool.end();
      },
    );

    it(
      'story S11: saveEdgeDelta allows a cross-disciplinary target when the source ' +
        "chunk is owned by the branch's own discipline",
      async () => {
        const pool = openPool();
        const chunkRepo = new ChunkGraphRepository(pool);
        const branchRepo = new BranchGraphRepository(pool, chunkRepo);
        const isolatedBranch = `branch-${randomUUID()}` as BranchId;
        const conflicts = new ConflictDetectionRepository(pool);
        const engineeringOwnedSource = ideaLabel('IDEA-s11-edge-engineering-owned-source');
        const productOwnedTarget = ideaLabel('IDEA-s11-edge-cross-disciplinary-target');

        await conflicts.registerBranch(workspaceA, isolatedBranch, 'engineering');
        await chunkRepo.saveChunk({
          workspaceId: workspaceA,
          ideaLabel: engineeringOwnedSource,
          chunkType: 'feature',
          discipline: 'engineering',
          contextKind: 'permanent',
          content: 'Mainline source chunk owned by engineering.',
          status: chunkLifecycleStatus('approved', 'active'),
        });
        await chunkRepo.saveChunk({
          workspaceId: workspaceA,
          ideaLabel: productOwnedTarget,
          chunkType: 'capability',
          discipline: 'product',
          contextKind: 'permanent',
          content: 'Mainline target chunk owned by product, never modified by this branch.',
          status: chunkLifecycleStatus('approved', 'active'),
        });

        await expect(
          branchRepo.saveEdgeDelta({
            workspaceId: workspaceA,
            branchId: isolatedBranch,
            sourceLabel: engineeringOwnedSource,
            targetLabel: productOwnedTarget,
            relationshipType: 'informs',
            deltaKind: 'upsert',
          }),
        ).resolves.toBeUndefined();

        const resolved = await branchRepo.resolveEdgeLineages(workspaceA, isolatedBranch);
        await pool.query('DELETE FROM chunks WHERE workspace_id = $1 AND idea_label IN ($2, $3)', [
          workspaceA,
          engineeringOwnedSource,
          productOwnedTarget,
        ]);
        await pool.query(
          'DELETE FROM branch_edge_deltas WHERE workspace_id = $1 AND branch_id = $2',
          [workspaceA, isolatedBranch],
        );
        await pool.query('DELETE FROM branches WHERE workspace_id = $1 AND branch_id = $2', [
          workspaceA,
          isolatedBranch,
        ]);
        await pool.end();

        expect(
          resolved.some(
            (l) => currentEdgeVersion(l).relationshipType === 'informs',
          ),
        ).toBe(true);
      },
    );

    it(
      'story S11 regression: a saveChunkDelta racing a concurrent submit is serialized ' +
        "by the branch row lock — it either commits before the submit (succeeds) or " +
        "observes the submitted status and is rejected write-locked, never both " +
        "silently succeeding against a branch that is already write-locked",
      async () => {
        const pool = openPool();
        const chunkRepo = new ChunkGraphRepository(pool);
        const branchRepo = new BranchGraphRepository(pool, chunkRepo);
        const raceBranch = `branch-${randomUUID()}` as BranchId;
        const conflicts = new ConflictDetectionRepository(pool);

        await conflicts.registerBranch(workspaceA, raceBranch, 'engineering');

        // Deterministically force the race (rather than relying on
        // ambiguous Promise.all scheduling): hold an uncommitted UPDATE
        // that flips the branch to 'submitted' open on one connection, so
        // saveChunkDelta's own `SELECT ... FOR UPDATE` (on a separate pool
        // connection) blocks behind it, then commit the held transaction so
        // saveChunkDelta deterministically resumes and observes the
        // now-'submitted' status.
        const blockingClient = await pool.connect();
        await blockingClient.query('BEGIN');
        await blockingClient.query(
          `UPDATE branches SET status = 'submitted' WHERE workspace_id = $1 AND branch_id = $2`,
          [workspaceA, raceBranch],
        );

        const racingSave = branchRepo.saveChunkDelta({
          workspaceId: workspaceA,
          branchId: raceBranch,
          ideaLabel: label1,
          deltaKind: 'upsert',
          chunk: {
            workspaceId: workspaceA,
            ideaLabel: label1,
            chunkType: 'feature',
            discipline: 'engineering',
            contextKind: 'permanent',
            content: 'must not persist once the branch is submitted',
            status: chunkLifecycleStatus('draft', 'active'),
          },
        });
        // Give saveChunkDelta's SELECT ... FOR UPDATE time to issue and
        // block behind the still-open, uncommitted UPDATE above.
        await new Promise((resolve) => setTimeout(resolve, 200));
        await blockingClient.query('COMMIT');
        blockingClient.release();

        await expect(racingSave).rejects.toMatchObject({
          name: 'BranchLifecycleError',
          code: 'write-locked',
        });

        const persistedDelta = await pool.query(
          `SELECT 1 FROM branch_chunk_deltas WHERE workspace_id = $1 AND branch_id = $2 AND idea_label = $3`,
          [workspaceA, raceBranch, label1],
        );
        await pool.query('DELETE FROM branches WHERE workspace_id = $1 AND branch_id = $2', [
          workspaceA,
          raceBranch,
        ]);
        await pool.end();

        // The race must never let the delta slip through: the branch was
        // 'submitted' by the time saveChunkDelta's locked read resumed, so
        // no row should have been written.
        expect(persistedDelta.rows).toHaveLength(0);
      },
    );

    it(
      'listDeactivatedIdeaLabels: an explicit delete delta is distinguishable from an idea the ' +
        "branch never touched, even though resolveChunks omits both from its net resolved view " +
        '(fixes rubber-duck-review ambiguity found against Meridian IDEA-32)',
      async () => {
        const pool = openPool();
        const chunkRepo = new ChunkGraphRepository(pool);
        const branchRepo = new BranchGraphRepository(pool, chunkRepo);
        const conflicts = new ConflictDetectionRepository(pool);
        const deactivationBranch = `branch-${randomUUID()}` as BranchId;
        const deactivatedLabel = ideaLabel('IDEA-branch-explicit-deactivation');
        const untouchedLabel = ideaLabel('IDEA-branch-never-touched');

        await conflicts.registerBranch(workspaceA, deactivationBranch, 'engineering');
        await chunkRepo.saveChunk({
          workspaceId: workspaceA,
          ideaLabel: deactivatedLabel,
          chunkType: 'feature',
          discipline: 'engineering',
          contextKind: 'permanent',
          content: 'Mainline chunk this branch proposes to deactivate.',
          status: chunkLifecycleStatus('approved', 'active'),
        });
        await branchRepo.saveChunkDelta({
          workspaceId: workspaceA,
          branchId: deactivationBranch,
          ideaLabel: deactivatedLabel,
          deltaKind: 'delete',
        });

        const deactivated = await branchRepo.listDeactivatedIdeaLabels(
          workspaceA,
          deactivationBranch,
        );
        const resolved = await branchRepo.resolveChunks(workspaceA, deactivationBranch);
        await pool.end();

        // The explicit deactivation is queryable...
        expect(deactivated).toContain(deactivatedLabel);
        // ...and distinguishable from an idea label the branch never
        // mentioned at all (not present in either list).
        expect(deactivated).not.toContain(untouchedLabel);
        // resolveChunks's net view omits both, by design — that ambiguity
        // is exactly what listDeactivatedIdeaLabels resolves.
        expect(resolved.some((chunk) => chunk.ideaLabel === deactivatedLabel)).toBe(false);
        expect(resolved.some((chunk) => chunk.ideaLabel === untouchedLabel)).toBe(false);
      },
    );

    it('ensureSchema is safe to run concurrently from separate pools without racing on DDL creation', async () => {
      // Regression test for the `pg_advisory_lock` fix in schema.ts: two
      // pools calling `ensureSchema` at the same moment previously could
      // race on Postgres's internal `pg_type` catalog when the tables did
      // not yet exist (observed in CI as two vitest e2e spec files both
      // calling `ensureSchema` in their own `beforeAll`).
      const poolOne = openPool();
      const poolTwo = openPool();
      const poolThree = openPool();

      await expect(
        Promise.all([
          ensureSchema(poolOne),
          ensureSchema(poolTwo),
          ensureSchema(poolThree),
        ]),
      ).resolves.toBeDefined();

      await Promise.all([poolOne.end(), poolTwo.end(), poolThree.end()]);
    });
  },
);
