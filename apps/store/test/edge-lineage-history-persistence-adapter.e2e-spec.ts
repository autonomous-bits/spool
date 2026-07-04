/**
 * Adapter-level integration test proving relationship (edge) history
 * survives repeated changes, including relationship-type changes, without
 * ever being deleted or silently collapsed (story S03, AC1-AC4).
 *
 * Technical spec §"Testing expectations" requires this kind of test to run
 * against a real containerized Postgres, not an in-memory substitute. Start
 * it locally before running this file:
 *
 *   docker compose up -d postgres
 *
 * and export the matching connection env vars (see
 * apps/store/AGENTS.md and config/store.env.example), e.g.:
 *
 *   export STORE_DB_HOST=localhost STORE_DB_PORT=5433 \
 *     STORE_DB_USER=spool STORE_DB_PASSWORD=spool_dev STORE_DB_NAME=spool
 *
 * Sources of authority:
 * - Story:          docs/specifications/feature-02-postgres-persistence/stories/S03-relationship-history-survives-change.md
 * - Technical spec: docs/specifications/feature-02-postgres-persistence/technical-specification.md
 *   §"Edge lineage persistence", §"Logical edge identity in persistence"
 * - Meridian:       IDEA-38
 */

import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { Pool } from 'pg';
import { randomUUID } from 'node:crypto';
import { loadDatabaseConfig } from '../src/persistence/database-config.js';
import { ensureSchema } from '../src/persistence/schema.js';
import { ChunkGraphRepository } from '../src/persistence/chunk-graph.repository.js';
import {
  createEdge,
  deactivateEdge,
  supersedeEdge,
  resolveLineage,
  currentEdgeVersion,
  EdgeLineageError,
} from '../src/domain/edge-lineage.js';
import { ideaLabel, workspaceId } from '../src/domain/types/index.js';

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
  'ChunkGraphRepository (Postgres adapter, relationship history / type-change lineage)',
  () => {
    const workspaceA = workspaceId(`ws-${randomUUID()}`);
    let pool: Pool;
    let repo: ChunkGraphRepository;

    beforeAll(async () => {
      const bootstrapPool = openPool();
      await ensureSchema(bootstrapPool);
      await bootstrapPool.end();
      pool = openPool();
      repo = new ChunkGraphRepository(pool);
    });

    afterAll(async () => {
      await pool.query('DELETE FROM chunks WHERE workspace_id = $1', [workspaceA]);
      await pool.query('DELETE FROM edge_versions WHERE workspace_id = $1', [workspaceA]);
      await pool.end();
    });

    it('AC1/AC2: repeated same-type supersession (3+ hops) reads back with unbroken, undeleted history', async () => {
      const source = ideaLabel('IDEA-repeated-source');
      const target = ideaLabel('IDEA-repeated-target');
      const identity = {
        workspaceId: workspaceA,
        sourceLabel: source,
        targetLabel: target,
        relationshipType: 'refines' as const,
      };

      let lineage = createEdge(workspaceA, source, target, 'refines');
      await repo.saveEdgeLineage(lineage);
      for (let i = 0; i < 3; i++) {
        lineage = supersedeEdge(lineage, identity);
        await repo.saveEdgeLineage(lineage);
      }

      const found = await repo.findEdgeLineage(workspaceA, source, target, 'refines');
      expect(found).toBeDefined();
      const history = resolveLineage(found!);
      expect(history).toHaveLength(4);
      expect(history[0]?.state).toBe('active');
      expect(history.slice(1).every((v) => v.state === 'superseded')).toBe(true);
    });

    it('AC3/AC4: changing relationship type creates a new active lineage and leaves a traceable link from the old one', async () => {
      const source = ideaLabel('IDEA-retype-source');
      const target = ideaLabel('IDEA-retype-target');

      const original = createEdge(workspaceA, source, target, 'depends-on');
      await repo.saveEdgeLineage(original);

      const { oldLineage, newLineage } = await repo.replaceEdgeRelationshipType(
        workspaceA,
        source,
        target,
        'depends-on',
        'refines',
      );

      // AC4: exactly one unambiguous current relationship for this pair —
      // the old type must not be active, and the new type must be active.
      expect(currentEdgeVersion(oldLineage).state).toBe('deactivated');
      expect(currentEdgeVersion(newLineage).state).toBe('active');
      // The old generation was closed by *appending* a deactivated version
      // (mirroring plain deactivation), not by flipping its sole version's
      // state in place: [superseded, deactivated].
      expect(resolveLineage(oldLineage).map((v) => v.state)).toEqual([
        'deactivated',
        'superseded',
      ]);

      const oldRecord = await repo.findEdgeLineageRecord(
        workspaceA,
        source,
        target,
        'depends-on',
      );
      const newLineageRead = await repo.findEdgeLineage(workspaceA, source, target, 'refines');

      expect(oldRecord).toBeDefined();
      expect(currentEdgeVersion(oldRecord!.lineage).state).toBe('deactivated');
      // AC3: the old type's record references the new type it was replaced by.
      expect(oldRecord!.succeededBy?.relationshipType).toBe('refines');
      expect(newLineageRead).toBeDefined();
      expect(currentEdgeVersion(newLineageRead!).state).toBe('active');

      // AC1: the traceable path can be walked backward from the current
      // relationship to what it replaced.
      const predecessor = await repo.findPredecessorEdgeLineageRecord(
        workspaceA,
        source,
        target,
        'refines',
        oldRecord!.succeededBy!.lineageSeq,
      );
      expect(predecessor).toBeDefined();
      expect(resolveLineage(predecessor!.lineage)[0]?.relationshipType).toBe('depends-on');
    });

    it('AC1/AC3: a type-change cycle (A -> B -> A) keeps both A generations independently readable without collision', async () => {
      const source = ideaLabel('IDEA-cycle-source');
      const target = ideaLabel('IDEA-cycle-target');

      await repo.saveEdgeLineage(createEdge(workspaceA, source, target, 'implements'));
      await repo.replaceEdgeRelationshipType(
        workspaceA,
        source,
        target,
        'implements',
        'informs',
      );
      await repo.replaceEdgeRelationshipType(
        workspaceA,
        source,
        target,
        'informs',
        'implements',
      );

      // The *current* generation of 'implements' must be active and fresh
      // (a single version), not confused with the original generation.
      const currentImplements = await repo.findEdgeLineageRecord(
        workspaceA,
        source,
        target,
        'implements',
      );
      expect(currentImplements).toBeDefined();
      expect(resolveLineage(currentImplements!.lineage)).toHaveLength(1);
      expect(currentEdgeVersion(currentImplements!.lineage).state).toBe('active');
      expect(currentImplements!.lineageSeq).toBe(2);

      // The original 'implements' generation (now deactivated) must still be
      // reachable by walking backward from 'informs', proving the first
      // generation was preserved, not overwritten or collapsed.
      const informsRecord = await repo.findEdgeLineageRecord(
        workspaceA,
        source,
        target,
        'informs',
      );
      expect(informsRecord).toBeDefined();
      expect(currentEdgeVersion(informsRecord!.lineage).state).toBe('deactivated');

      const originalImplements = await repo.findPredecessorEdgeLineageRecord(
        workspaceA,
        source,
        target,
        'informs',
        informsRecord!.lineageSeq,
      );
      expect(originalImplements).toBeDefined();
      expect(originalImplements!.lineageSeq).toBe(1);
      expect(currentEdgeVersion(originalImplements!.lineage).state).toBe('deactivated');
    });

    it('rejects a generation-oblivious saveEdgeLineage once an identity has multiple generations, leaving the current generation untouched', async () => {
      const source = ideaLabel('IDEA-multi-gen-write-source');
      const target = ideaLabel('IDEA-multi-gen-write-target');

      // Cycle 'depends-on' -> 'refines' -> 'depends-on' so 'depends-on' now
      // has two generations on record (generation 1: deactivated;
      // generation 2: active).
      await repo.saveEdgeLineage(createEdge(workspaceA, source, target, 'depends-on'));
      await repo.replaceEdgeRelationshipType(workspaceA, source, target, 'depends-on', 'refines');
      await repo.replaceEdgeRelationshipType(workspaceA, source, target, 'refines', 'depends-on');

      // A caller holding a stale, generation-oblivious 'depends-on' value
      // (indistinguishable from either generation, since the domain model
      // has no generation concept) must not be able to silently write into
      // whichever generation happens to be current.
      const staleValue = createEdge(workspaceA, source, target, 'depends-on');
      await expect(repo.saveEdgeLineage(staleValue)).rejects.toMatchObject({
        name: 'EdgeLineageError',
        code: 'lineage-violation',
      });

      const currentDependsOn = await repo.findEdgeLineageRecord(
        workspaceA,
        source,
        target,
        'depends-on',
      );
      expect(currentDependsOn).toBeDefined();
      expect(currentDependsOn!.lineageSeq).toBe(2);
      expect(currentEdgeVersion(currentDependsOn!.lineage).state).toBe('active');
      expect(resolveLineage(currentDependsOn!.lineage)).toHaveLength(1);
    });

    it('serializes concurrent retypes from different old types into the same new identity, mapping the loser to duplicate-active-relationship', async () => {
      const source = ideaLabel('IDEA-cross-retype-source');
      const target = ideaLabel('IDEA-cross-retype-target');

      await repo.saveEdgeLineage(createEdge(workspaceA, source, target, 'depends-on'));
      await repo.saveEdgeLineage(createEdge(workspaceA, source, target, 'implements'));

      const results = await Promise.allSettled([
        repo.replaceEdgeRelationshipType(workspaceA, source, target, 'depends-on', 'refines'),
        repo.replaceEdgeRelationshipType(workspaceA, source, target, 'implements', 'refines'),
      ]);

      // Both retypes target the identical new identity
      // (workspaceA, source, target, 'refines'); the advisory lock keyed on
      // that identity serializes `newSeq` allocation so the loser fails
      // deterministically on the "one active edge" unique index —
      // `duplicate-active-relationship` — rather than racing to a raw
      // primary-key collision.
      const fulfilled = results.filter((r) => r.status === 'fulfilled');
      const rejected = results.filter((r) => r.status === 'rejected');
      expect(fulfilled).toHaveLength(1);
      expect(rejected).toHaveLength(1);
      expect(rejected[0]).toMatchObject({
        reason: { name: 'EdgeLineageError', code: 'duplicate-active-relationship' },
      });

      const refinesLineage = await repo.findEdgeLineage(workspaceA, source, target, 'refines');
      expect(currentEdgeVersion(refinesLineage!).state).toBe('active');
    });

    it('rejects retyping a lineage that is not currently active', async () => {
      const source = ideaLabel('IDEA-inactive-retype-source');
      const target = ideaLabel('IDEA-inactive-retype-target');

      await repo.saveEdgeLineage(createEdge(workspaceA, source, target, 'supersedes'));
      await repo.replaceEdgeRelationshipType(
        workspaceA,
        source,
        target,
        'supersedes',
        'refines',
      );

      // 'supersedes' generation 1 is now deactivated; retyping it again must fail.
      await expect(
        repo.replaceEdgeRelationshipType(workspaceA, source, target, 'supersedes', 'informs'),
      ).rejects.toMatchObject({
        name: 'EdgeLineageError',
        code: 'invalid-state-transition',
      });
    });

    it('rejects retyping into a type with an existing active lineage and leaves the old lineage untouched', async () => {
      const source = ideaLabel('IDEA-conflict-retype-source');
      const target = ideaLabel('IDEA-conflict-retype-target');

      await repo.saveEdgeLineage(createEdge(workspaceA, source, target, 'depends-on'));
      await repo.saveEdgeLineage(createEdge(workspaceA, source, target, 'refines'));

      await expect(
        repo.replaceEdgeRelationshipType(workspaceA, source, target, 'depends-on', 'refines'),
      ).rejects.toMatchObject({ name: 'EdgeLineageError' });

      const stillActive = await repo.findEdgeLineage(workspaceA, source, target, 'depends-on');
      expect(stillActive).toBeDefined();
      expect(currentEdgeVersion(stillActive!).state).toBe('active');
    });

    it('rejects replacing a relationship type with itself', async () => {
      const source = ideaLabel('IDEA-same-type-source');
      const target = ideaLabel('IDEA-same-type-target');
      await repo.saveEdgeLineage(createEdge(workspaceA, source, target, 'refines'));

      await expect(
        repo.replaceEdgeRelationshipType(workspaceA, source, target, 'refines', 'refines'),
      ).rejects.toMatchObject({
        name: 'EdgeLineageError',
        code: 'invalid-state-transition',
      });
    });

    it('serializes concurrent retype attempts on the same lineage so only one succeeds', async () => {
      const source = ideaLabel('IDEA-race-retype-source');
      const target = ideaLabel('IDEA-race-retype-target');
      await repo.saveEdgeLineage(createEdge(workspaceA, source, target, 'depends-on'));

      const results = await Promise.allSettled([
        repo.replaceEdgeRelationshipType(workspaceA, source, target, 'depends-on', 'refines'),
        repo.replaceEdgeRelationshipType(workspaceA, source, target, 'depends-on', 'informs'),
      ]);

      const fulfilled = results.filter((r) => r.status === 'fulfilled');
      const rejected = results.filter((r) => r.status === 'rejected');
      expect(fulfilled).toHaveLength(1);
      expect(rejected).toHaveLength(1);
      expect((rejected[0] as PromiseRejectedResult).reason).toBeInstanceOf(EdgeLineageError);

      const oldLineage = await repo.findEdgeLineage(workspaceA, source, target, 'depends-on');
      expect(currentEdgeVersion(oldLineage!).state).toBe('deactivated');
    });

    it('tenant isolation: a relationship-type change in one workspace never becomes visible in another', async () => {
      const workspaceB = workspaceId(`ws-${randomUUID()}`);
      const source = ideaLabel('IDEA-tenant-retype-source');
      const target = ideaLabel('IDEA-tenant-retype-target');

      await repo.saveEdgeLineage(createEdge(workspaceB, source, target, 'depends-on'));
      await repo.replaceEdgeRelationshipType(
        workspaceB,
        source,
        target,
        'depends-on',
        'refines',
      );

      const crossWorkspaceOld = await repo.findEdgeLineage(
        workspaceA,
        source,
        target,
        'depends-on',
      );
      const crossWorkspaceNew = await repo.findEdgeLineage(workspaceA, source, target, 'refines');
      const ownWorkspaceNew = await repo.findEdgeLineage(workspaceB, source, target, 'refines');

      expect(crossWorkspaceOld).toBeUndefined();
      expect(crossWorkspaceNew).toBeUndefined();
      expect(ownWorkspaceNew).toBeDefined();

      await pool.query('DELETE FROM edge_versions WHERE workspace_id = $1', [workspaceB]);
    });

    it('listEdgeLineages surfaces the current generation for both the old and new relationship type after a change', async () => {
      const source = ideaLabel('IDEA-list-retype-source');
      const target = ideaLabel('IDEA-list-retype-target');

      await repo.saveEdgeLineage(createEdge(workspaceA, source, target, 'depends-on'));
      await repo.replaceEdgeRelationshipType(
        workspaceA,
        source,
        target,
        'depends-on',
        'refines',
      );

      const lineages = await repo.listEdgeLineages(workspaceA);
      const matching = lineages.filter(
        (l) =>
          resolveLineage(l)[0]?.sourceLabel === source &&
          resolveLineage(l)[0]?.targetLabel === target,
      );

      // 'depends-on' and 'refines' are distinct relationship-type identities,
      // so both remain listed (history is preserved, not hidden) — but each
      // must reflect only its own latest generation, and only one of the two
      // may be active (AC4).
      expect(matching).toHaveLength(2);
      const byType = new Map(
        matching.map((l) => [currentEdgeVersion(l).relationshipType, currentEdgeVersion(l)]),
      );
      expect(byType.get('depends-on')?.state).toBe('deactivated');
      expect(byType.get('refines')?.state).toBe('active');
    });

    it('a deactivated (never retyped) lineage has no predecessor record — it was not produced by a type change', async () => {
      const source = ideaLabel('IDEA-plain-deactivate-source');
      const target = ideaLabel('IDEA-plain-deactivate-target');

      let lineage = createEdge(workspaceA, source, target, 'informs');
      await repo.saveEdgeLineage(lineage);
      lineage = deactivateEdge(lineage);
      await repo.saveEdgeLineage(lineage);

      const predecessor = await repo.findPredecessorEdgeLineageRecord(
        workspaceA,
        source,
        target,
        'informs',
        1,
      );
      expect(predecessor).toBeUndefined();

      const record = await repo.findEdgeLineageRecord(workspaceA, source, target, 'informs');
      expect(record?.succeededBy).toBeUndefined();
    });

    it(
      'story S11 AC2: a relationship superseded by a type change is distinguishable ' +
        "from one plainly retired with no replacement — the former's record carries " +
        "a succeededBy link, the latter's does not",
      async () => {
        const supersededSource = ideaLabel('IDEA-s11-superseded-source');
        const supersededTarget = ideaLabel('IDEA-s11-superseded-target');
        const retiredSource = ideaLabel('IDEA-s11-retired-source');
        const retiredTarget = ideaLabel('IDEA-s11-retired-target');

        await repo.saveEdgeLineage(
          createEdge(workspaceA, supersededSource, supersededTarget, 'depends-on'),
        );
        await repo.replaceEdgeRelationshipType(
          workspaceA,
          supersededSource,
          supersededTarget,
          'depends-on',
          'refines',
        );

        let retiredLineage = createEdge(workspaceA, retiredSource, retiredTarget, 'depends-on');
        await repo.saveEdgeLineage(retiredLineage);
        retiredLineage = deactivateEdge(retiredLineage);
        await repo.saveEdgeLineage(retiredLineage);

        const supersededRecord = await repo.findEdgeLineageRecord(
          workspaceA,
          supersededSource,
          supersededTarget,
          'depends-on',
        );
        const retiredRecord = await repo.findEdgeLineageRecord(
          workspaceA,
          retiredSource,
          retiredTarget,
          'depends-on',
        );

        // Both terminal versions read back as 'deactivated' in isolation —
        // the state field alone cannot distinguish them (this is exactly
        // what AC2 requires the persistence layer to keep distinguishable).
        expect(currentEdgeVersion(supersededRecord!.lineage).state).toBe('deactivated');
        expect(currentEdgeVersion(retiredRecord!.lineage).state).toBe('deactivated');

        // The distinguishing fact lives in succeededBy: superseded-by-retype
        // carries a link to what replaced it; plain retirement does not.
        expect(supersededRecord!.succeededBy?.relationshipType).toBe('refines');
        expect(retiredRecord!.succeededBy).toBeUndefined();
      },
    );

    it('rejects saving a lineage whose stored version count matches but whose terminal state disagrees with what is stored', async () => {
      const source = ideaLabel('IDEA-same-count-diff-state-source');
      const target = ideaLabel('IDEA-same-count-diff-state-target');
      const identity = {
        workspaceId: workspaceA,
        sourceLabel: source,
        targetLabel: target,
        relationshipType: 'depends-on' as const,
      };

      const original = createEdge(workspaceA, source, target, 'depends-on');
      await repo.saveEdgeLineage(original);

      // One holder supersedes and saves: stored becomes [superseded, active].
      const superseded = supersedeEdge(original, identity);
      await repo.saveEdgeLineage(superseded);

      // A second, stale holder of the original ('active', pre-supersession)
      // lineage instead deactivates it: [superseded, deactivated] — the
      // same version count (2) as what is now stored, but a different
      // terminal state for version 2. This must be rejected, not silently
      // accepted (which would either resurrect a state the domain never
      // asked for, or corrupt the already-superseded row).
      const staleDeactivated = deactivateEdge(original);
      await expect(repo.saveEdgeLineage(staleDeactivated)).rejects.toMatchObject({
        name: 'EdgeLineageError',
        code: 'lineage-violation',
      });

      const stillCurrent = await repo.findEdgeLineage(
        workspaceA,
        source,
        target,
        'depends-on',
      );
      expect(currentEdgeVersion(stillCurrent!).state).toBe('active');
    });

    it('rejects reconstructing a lineage whose sole persisted row is deactivated', async () => {
      const source = ideaLabel('IDEA-lone-deactivated-row-source');
      const target = ideaLabel('IDEA-lone-deactivated-row-target');

      // A lone, first-ever version can never legitimately be 'deactivated'
      // — deactivation always appends onto a prior active version — so
      // insert this directly to prove `rowsToEdgeLineage` rejects it as
      // corrupt/invalid persisted state, rather than modeling a real
      // write path (no repository method can produce this row shape).
      await pool.query(
        `INSERT INTO edge_versions (
           workspace_id, source_label, target_label, relationship_type, lineage_seq, version, state
         ) VALUES ($1, $2, $3, $4, 1, 1, 'deactivated')`,
        [workspaceA, source, target, 'depends-on'],
      );

      await expect(
        repo.findEdgeLineage(workspaceA, source, target, 'depends-on'),
      ).rejects.toMatchObject({
        name: 'EdgeLineageError',
        code: 'lineage-violation',
      });
    });
  },
);
