/**
 * Adapter-level integration test proving chunk-artifact association
 * lineages (story S05) persist correctly against a real containerized
 * Postgres: mainline vs branch-scoped shadow lineages, append-only history,
 * duplicate-active rejection, and tenant/branch isolation.
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
import { ArtifactAssociationRepository } from '../src/persistence/artifact-association.repository.js';
import { ArtifactAssociationError } from '../src/domain/artifact-association-lineage.js';
import { ConflictDetectionRepository } from '../src/persistence/conflict-detection.repository.js';
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
  'ArtifactAssociationRepository (Postgres adapter, chunk-artifact association lineages)',
  () => {
    const workspaceA = workspaceId(`ws-${randomUUID()}`);
    const workspaceB = workspaceId(`ws-${randomUUID()}`);
    const branchA = `branch-${randomUUID()}` as BranchId;
    const branchB = `branch-${randomUUID()}` as BranchId;

    beforeAll(async () => {
      const bootstrapPool = openPool();
      await ensureSchema(bootstrapPool);
      const conflicts = new ConflictDetectionRepository(bootstrapPool);
      // Registers branchA/branchB (fixes rubber-duck-review gap: branch
      // write-lock enforcement now applies to chunk-artifact-association
      // writes too, so every branch used by a write in this suite must be
      // a registered 'draft' branch, mirroring
      // branch-graph-persistence-adapter.e2e-spec.ts's convention).
      await conflicts.registerBranch(workspaceA, branchA, 'engineering');
      await conflicts.registerBranch(workspaceA, branchB, 'engineering');
      await bootstrapPool.end();
    });

    afterAll(async () => {
      const cleanupPool = openPool();
      await cleanupPool.query(
        'DELETE FROM chunk_artifacts WHERE workspace_id = $1 OR workspace_id = $2',
        [workspaceA, workspaceB],
      );
      await cleanupPool.query(
        'DELETE FROM branches WHERE workspace_id = $1 OR workspace_id = $2',
        [workspaceA, workspaceB],
      );
      await cleanupPool.end();
    });

    it('AC1: a branch-scoped association is traceable while under branch review', async () => {
      const pool = openPool();
      const repo = new ArtifactAssociationRepository(pool);
      const chunkLabel = ideaLabel('IDEA-artifact-ac1');
      const artifact = artifactId(`artifact-${randomUUID()}`);

      const created = await repo.createAssociation({
        workspaceId: workspaceA,
        chunkLabel,
        artifactId: artifact,
        branchId: branchA,
      });
      const resolved = await repo.resolveAssociationsForBranch(workspaceA, branchA, chunkLabel);
      await pool.end();

      expect(created.state).toBe('active');
      expect(created.branchId).toBe(branchA);
      expect(resolved.some((a) => a.artifactId === artifact && a.state === 'active')).toBe(true);
    });

    it("AC2: deactivating a branch's association does not affect the mainline association", async () => {
      const pool = openPool();
      const repo = new ArtifactAssociationRepository(pool);
      const chunkLabel = ideaLabel('IDEA-artifact-ac2');
      const artifact = artifactId(`artifact-${randomUUID()}`);

      await repo.createAssociation({ workspaceId: workspaceA, chunkLabel, artifactId: artifact });
      await repo.deactivateAssociation(workspaceA, chunkLabel, artifact, branchA);

      const mainlineHistory = await repo.listAssociationHistory(workspaceA, chunkLabel, artifact);
      const branchHistory = await repo.listAssociationHistory(
        workspaceA,
        chunkLabel,
        artifact,
        branchA,
      );
      await pool.end();

      expect(mainlineHistory).toHaveLength(1);
      expect(mainlineHistory[0]?.state).toBe('active');
      // Branch got its own shadow lineage: active -> deactivated (2 versions),
      // never a lone deactivated row.
      expect(branchHistory).toHaveLength(2);
      expect(branchHistory.map((v) => v.state)).toEqual(['deactivated', 'superseded']);
    });

    it('AC3: current status is distinguishable and prior versions remain queryable after deactivation', async () => {
      const pool = openPool();
      const repo = new ArtifactAssociationRepository(pool);
      const chunkLabel = ideaLabel('IDEA-artifact-ac3');
      const artifact = artifactId(`artifact-${randomUUID()}`);

      await repo.createAssociation({ workspaceId: workspaceA, chunkLabel, artifactId: artifact });
      const deactivated = await repo.deactivateAssociation(workspaceA, chunkLabel, artifact);
      const history = await repo.listAssociationHistory(workspaceA, chunkLabel, artifact);
      await pool.end();

      expect(deactivated.state).toBe('deactivated');
      expect(deactivated.version).toBe(2);
      expect(history).toHaveLength(2);
      expect(history[0]?.state).toBe('deactivated');
      expect(history[1]?.state).toBe('superseded');
    });

    it('origin_branch_id is preserved unchanged across a branch lineage`s later versions', async () => {
      const pool = openPool();
      const repo = new ArtifactAssociationRepository(pool);
      const chunkLabel = ideaLabel('IDEA-artifact-origin');
      const artifact = artifactId(`artifact-${randomUUID()}`);

      const created = await repo.createAssociation({
        workspaceId: workspaceA,
        chunkLabel,
        artifactId: artifact,
        branchId: branchA,
      });
      const deactivated = await repo.deactivateAssociation(
        workspaceA,
        chunkLabel,
        artifact,
        branchA,
      );
      await pool.end();

      expect(created.originBranchId).toBe(branchA);
      expect(deactivated.originBranchId).toBe(branchA);
    });

    it('mainline-created association has no originBranchId', async () => {
      const pool = openPool();
      const repo = new ArtifactAssociationRepository(pool);
      const chunkLabel = ideaLabel('IDEA-artifact-mainline-origin');
      const artifact = artifactId(`artifact-${randomUUID()}`);

      const created = await repo.createAssociation({
        workspaceId: workspaceA,
        chunkLabel,
        artifactId: artifact,
      });
      await pool.end();

      expect(created.originBranchId).toBeUndefined();
    });

    it('invalid-state-transition: re-creating an association after it has been deactivated is rejected (no reactivation transition)', async () => {
      const pool = openPool();
      const repo = new ArtifactAssociationRepository(pool);
      const chunkLabel = ideaLabel('IDEA-artifact-recreate-after-deactivate');
      const artifact = artifactId(`artifact-${randomUUID()}`);

      await repo.createAssociation({ workspaceId: workspaceA, chunkLabel, artifactId: artifact });
      await repo.deactivateAssociation(workspaceA, chunkLabel, artifact);

      await expect(
        repo.createAssociation({ workspaceId: workspaceA, chunkLabel, artifactId: artifact }),
      ).rejects.toMatchObject({ code: 'invalid-state-transition' });

      // History remains exactly the two versions from before the rejected
      // recreate attempt -- no ambiguous second "version 1" was inserted.
      const history = await repo.listAssociationHistory(workspaceA, chunkLabel, artifact);
      await pool.end();
      expect(history).toHaveLength(2);
    });

    it('concurrent branch-shadow-seeding deactivate calls for the same identity do not both succeed or corrupt history', async () => {
      const pool = openPool();
      const repo = new ArtifactAssociationRepository(pool);
      const chunkLabel = ideaLabel('IDEA-artifact-concurrent-shadow');
      const artifact = artifactId(`artifact-${randomUUID()}`);

      await repo.createAssociation({ workspaceId: workspaceA, chunkLabel, artifactId: artifact });

      const results = await Promise.allSettled([
        repo.deactivateAssociation(workspaceA, chunkLabel, artifact, branchA),
        repo.deactivateAssociation(workspaceA, chunkLabel, artifact, branchA),
      ]);

      const history = await repo.listAssociationHistory(
        workspaceA,
        chunkLabel,
        artifact,
        branchA,
      );
      await pool.end();

      const fulfilled = results.filter((r) => r.status === 'fulfilled');
      const rejected = results.filter((r) => r.status === 'rejected');
      // Exactly one call wins; the loser gets a domain error (never a raw
      // pg unique-violation) and the branch's shadow lineage is exactly
      // two versions (active -> deactivated), never duplicated.
      expect(fulfilled).toHaveLength(1);
      expect(rejected).toHaveLength(1);
      expect((rejected[0] as PromiseRejectedResult).reason).toBeInstanceOf(
        ArtifactAssociationError,
      );
      expect(history).toHaveLength(2);
      expect(history.map((v) => v.state)).toEqual(['deactivated', 'superseded']);
    });

    it('duplicate-active-relationship: creating a second active mainline association for the same identity is rejected', async () => {
      const pool = openPool();
      const repo = new ArtifactAssociationRepository(pool);
      const chunkLabel = ideaLabel('IDEA-artifact-dup-mainline');
      const artifact = artifactId(`artifact-${randomUUID()}`);

      await repo.createAssociation({ workspaceId: workspaceA, chunkLabel, artifactId: artifact });

      await expect(
        repo.createAssociation({ workspaceId: workspaceA, chunkLabel, artifactId: artifact }),
      ).rejects.toMatchObject({
        code: 'duplicate-active-relationship',
      } satisfies Partial<ArtifactAssociationError>);
      await pool.end();
    });

    it('duplicate-active-relationship: creating a second active branch association for the same identity is rejected', async () => {
      const pool = openPool();
      const repo = new ArtifactAssociationRepository(pool);
      const chunkLabel = ideaLabel('IDEA-artifact-dup-branch');
      const artifact = artifactId(`artifact-${randomUUID()}`);

      await repo.createAssociation({
        workspaceId: workspaceA,
        chunkLabel,
        artifactId: artifact,
        branchId: branchA,
      });

      await expect(
        repo.createAssociation({
          workspaceId: workspaceA,
          chunkLabel,
          artifactId: artifact,
          branchId: branchA,
        }),
      ).rejects.toMatchObject({ code: 'duplicate-active-relationship' });
      await pool.end();
    });

    it('a mainline-active association and a branch-active association for the same identity can coexist', async () => {
      const pool = openPool();
      const repo = new ArtifactAssociationRepository(pool);
      const chunkLabel = ideaLabel('IDEA-artifact-coexist');
      const artifact = artifactId(`artifact-${randomUUID()}`);

      const mainline = await repo.createAssociation({
        workspaceId: workspaceA,
        chunkLabel,
        artifactId: artifact,
      });
      const branch = await repo.createAssociation({
        workspaceId: workspaceA,
        chunkLabel,
        artifactId: artifact,
        branchId: branchA,
      });
      await pool.end();

      expect(mainline.state).toBe('active');
      expect(branch.state).toBe('active');
    });

    it('not-found: deactivating a non-existent association is rejected', async () => {
      const pool = openPool();
      const repo = new ArtifactAssociationRepository(pool);
      const chunkLabel = ideaLabel('IDEA-artifact-not-found');
      const artifact = artifactId(`artifact-${randomUUID()}`);

      await expect(
        repo.deactivateAssociation(workspaceA, chunkLabel, artifact),
      ).rejects.toMatchObject({ code: 'not-found' });
      await pool.end();
    });

    it('invalid-state-transition: deactivating an already-deactivated association is rejected', async () => {
      const pool = openPool();
      const repo = new ArtifactAssociationRepository(pool);
      const chunkLabel = ideaLabel('IDEA-artifact-already-deactivated');
      const artifact = artifactId(`artifact-${randomUUID()}`);

      await repo.createAssociation({ workspaceId: workspaceA, chunkLabel, artifactId: artifact });
      await repo.deactivateAssociation(workspaceA, chunkLabel, artifact);

      await expect(
        repo.deactivateAssociation(workspaceA, chunkLabel, artifact),
      ).rejects.toMatchObject({ code: 'invalid-state-transition' });
      await pool.end();
    });

    it("a different branch's association changes never leak into another branch's resolved view or the mainline", async () => {
      const pool = openPool();
      const repo = new ArtifactAssociationRepository(pool);
      const chunkLabel = ideaLabel('IDEA-artifact-branch-leak');
      const artifact = artifactId(`artifact-${randomUUID()}`);

      await repo.createAssociation({
        workspaceId: workspaceA,
        chunkLabel,
        artifactId: artifact,
        branchId: branchB,
      });

      const resolvedForBranchA = await repo.resolveAssociationsForBranch(
        workspaceA,
        branchA,
        chunkLabel,
      );
      const mainline = await repo.listMainlineActiveAssociations(workspaceA, chunkLabel);
      await pool.end();

      expect(resolvedForBranchA.some((a) => a.artifactId === artifact)).toBe(false);
      expect(mainline.some((a) => a.artifactId === artifact)).toBe(false);
    });

    it("tenant isolation: an association saved under one workspace never resolves into another workspace's queries", async () => {
      const pool = openPool();
      const repo = new ArtifactAssociationRepository(pool);
      const chunkLabel = ideaLabel('IDEA-artifact-tenant-isolation');
      const artifact = artifactId(`artifact-${randomUUID()}`);

      await repo.createAssociation({
        workspaceId: workspaceB,
        chunkLabel,
        artifactId: artifact,
      });

      const mainlineForA = await repo.listMainlineActiveAssociations(workspaceA, chunkLabel);
      const resolvedForA = await repo.resolveAssociationsForBranch(
        workspaceA,
        branchA,
        chunkLabel,
      );
      await pool.end();

      expect(mainlineForA.some((a) => a.artifactId === artifact)).toBe(false);
      expect(resolvedForA.some((a) => a.artifactId === artifact)).toBe(false);
    });

    it('resolveAssociationsForBranch overrides mainline with the branch\'s own active association for the same identity', async () => {
      const pool = openPool();
      const repo = new ArtifactAssociationRepository(pool);
      const chunkLabel = ideaLabel('IDEA-artifact-override');
      const artifact = artifactId(`artifact-${randomUUID()}`);

      await repo.createAssociation({ workspaceId: workspaceA, chunkLabel, artifactId: artifact });
      await repo.deactivateAssociation(workspaceA, chunkLabel, artifact, branchA);

      const resolved = await repo.resolveAssociationsForBranch(workspaceA, branchA, chunkLabel);
      await pool.end();

      // Branch deactivated its shadow lineage for this identity, so it is
      // hidden from the branch's resolved view even though mainline is active.
      expect(resolved.some((a) => a.artifactId === artifact)).toBe(false);
    });

    it(
      'write-lock: creating or deactivating an association against a non-draft branch throws ' +
        "BranchLifecycleError(write-locked) (fixes rubber-duck-review gap: branch write-lock " +
        'enforcement previously covered branch_chunk_deltas/branch_edge_deltas but not chunk_artifacts)',
      async () => {
        const pool = openPool();
        const repo = new ArtifactAssociationRepository(pool);
        const conflicts = new ConflictDetectionRepository(pool);
        const lockedBranch = `branch-${randomUUID()}` as BranchId;
        const chunkLabel = ideaLabel('IDEA-artifact-write-lock');
        const artifact = artifactId(`artifact-${randomUUID()}`);

        await conflicts.registerBranch(workspaceA, lockedBranch, 'engineering');
        // Seed an active branch-scoped association while still draft, then
        // advance the branch past 'draft' directly, mirroring
        // branch-graph-persistence-adapter.e2e-spec.ts's "story S11" test.
        await repo.createAssociation({
          workspaceId: workspaceA,
          chunkLabel,
          artifactId: artifact,
          branchId: lockedBranch,
        });
        await pool.query(
          `UPDATE branches SET status = 'submitted' WHERE workspace_id = $1 AND branch_id = $2`,
          [workspaceA, lockedBranch],
        );

        await expect(
          repo.createAssociation({
            workspaceId: workspaceA,
            chunkLabel: ideaLabel('IDEA-artifact-write-lock-2'),
            artifactId: artifactId(`artifact-${randomUUID()}`),
            branchId: lockedBranch,
          }),
        ).rejects.toMatchObject({ name: 'BranchLifecycleError', code: 'write-locked' });

        await expect(
          repo.deactivateAssociation(workspaceA, chunkLabel, artifact, lockedBranch),
        ).rejects.toMatchObject({ name: 'BranchLifecycleError', code: 'write-locked' });

        await pool.end();
      },
    );

    it(
      'not-found: creating or deactivating a branch-scoped association against an unregistered ' +
        'branch throws BranchLifecycleError(not-found)',
      async () => {
        const pool = openPool();
        const repo = new ArtifactAssociationRepository(pool);
        const unregisteredBranch = `branch-${randomUUID()}` as BranchId;
        const chunkLabel = ideaLabel('IDEA-artifact-unregistered-branch');
        const artifact = artifactId(`artifact-${randomUUID()}`);

        await expect(
          repo.createAssociation({
            workspaceId: workspaceA,
            chunkLabel,
            artifactId: artifact,
            branchId: unregisteredBranch,
          }),
        ).rejects.toMatchObject({ name: 'BranchLifecycleError', code: 'not-found' });

        await expect(
          repo.deactivateAssociation(workspaceA, chunkLabel, artifact, unregisteredBranch),
        ).rejects.toMatchObject({ name: 'BranchLifecycleError', code: 'not-found' });

        await pool.end();
      },
    );
  },
);
