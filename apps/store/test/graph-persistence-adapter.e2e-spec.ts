/**
 * Adapter-level integration test proving the Postgres persistence adapter
 * durably stores the mainline chunk + edge-lineage graph so it survives
 * process restarts (story S01, AC1-AC3).
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
 * This test simulates "readable in a subsequent process lifetime" by
 * opening and fully closing independent `Pool` instances rather than
 * reusing one connection across the write and read phases.
 */

import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { Pool } from 'pg';
import { randomUUID } from 'node:crypto';
import { loadDatabaseConfig } from '../src/persistence/database-config.js';
import { ensureSchema } from '../src/persistence/schema.js';
import { ChunkGraphRepository } from '../src/persistence/chunk-graph.repository.js';
import { chunkLifecycleStatus } from '../src/domain/chunk-lifecycle.js';
import {
  createEdge,
  deactivateEdge,
  supersedeEdge,
  resolveLineage,
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

// This integration test requires a real, running Postgres instance and is
// intentionally not run against an in-memory substitute (technical spec
// §"Testing expectations"). Rather than failing the whole `pnpm test` gate
// in environments that have not started `docker compose up -d postgres`
// and exported the STORE_DB_* variables, the suite skips itself with a
// clear reason when the required connection configuration is absent.
const hasDatabaseConfig = [
  'STORE_DB_HOST',
  'STORE_DB_PORT',
  'STORE_DB_USER',
  'STORE_DB_PASSWORD',
  'STORE_DB_NAME',
].every((key) => Boolean(process.env[key]?.trim()));

describe.skipIf(!hasDatabaseConfig)(
  'ChunkGraphRepository (Postgres adapter, restart durability)',
  () => {
  // Unique per test run so repeated runs against a shared dev Postgres don't
  // collide.
  const workspaceA = workspaceId(`ws-${randomUUID()}`);
  const workspaceB = workspaceId(`ws-${randomUUID()}`);
  const label1 = ideaLabel('IDEA-checkout-flow');
  const label2 = ideaLabel('IDEA-payment-gateway');

  beforeAll(async () => {
    const bootstrapPool = openPool();
    await ensureSchema(bootstrapPool);
    await bootstrapPool.end();
  });

  afterAll(async () => {
    // Best-effort cleanup so repeated local runs stay tidy.
    const cleanupPool = openPool();
    await cleanupPool.query('DELETE FROM chunks WHERE workspace_id = $1 OR workspace_id = $2', [
      workspaceA,
      workspaceB,
    ]);
    await cleanupPool.query(
      'DELETE FROM edge_versions WHERE workspace_id = $1 OR workspace_id = $2',
      [workspaceA, workspaceB],
    );
    await cleanupPool.end();
  });

  it('AC1/AC2: a chunk written in one process lifetime reads back unchanged after that pool closes', async () => {
    const writePool = openPool();
    const writeRepo = new ChunkGraphRepository(writePool);

    await writeRepo.saveChunk({
      workspaceId: workspaceA,
      ideaLabel: label1,
      chunkType: 'feature',
      discipline: 'engineering',
      contextKind: 'permanent',
      content: 'Stakeholders can complete checkout in a single flow.',
      status: chunkLifecycleStatus('approved', 'active'),
    });
    await writePool.end();

    const readPool = openPool();
    const readRepo = new ChunkGraphRepository(readPool);
    const found = await readRepo.findChunk(workspaceA, label1);
    await readPool.end();

    expect(found).toEqual({
      workspaceId: workspaceA,
      ideaLabel: label1,
      chunkType: 'feature',
      discipline: 'engineering',
      contextKind: 'permanent',
      content: 'Stakeholders can complete checkout in a single flow.',
      status: chunkLifecycleStatus('approved', 'active'),
    });
  });

  it('AC1/AC2: an edge lineage (create -> supersede -> deactivate) reads back with full history intact after restart', async () => {
    const writePool = openPool();
    const writeRepo = new ChunkGraphRepository(writePool);

    let lineage = createEdge(workspaceA, label1, label2, 'depends-on');
    await writeRepo.saveEdgeLineage(lineage);

    lineage = supersedeEdge(lineage, {
      workspaceId: workspaceA,
      sourceLabel: label1,
      targetLabel: label2,
      relationshipType: 'depends-on',
    });
    await writeRepo.saveEdgeLineage(lineage);

    lineage = deactivateEdge(lineage);
    await writeRepo.saveEdgeLineage(lineage);
    await writePool.end();

    const readPool = openPool();
    const readRepo = new ChunkGraphRepository(readPool);
    const found = await readRepo.findEdgeLineage(
      workspaceA,
      label1,
      label2,
      'depends-on',
    );
    await readPool.end();

    expect(found).toBeDefined();
    expect(resolveLineage(found!)).toEqual(resolveLineage(lineage));
    // supersede appends version 2 (active); deactivate then supersedes
    // version 2 and appends version 3 (deactivated) rather than mutating
    // version 2 in place.
    expect(found!.versions).toHaveLength(3);
    expect(found!.versions[0]?.state).toBe('superseded');
    expect(found!.versions[1]?.state).toBe('superseded');
    expect(found!.versions[2]?.state).toBe('deactivated');
  });

  it('AC3: an implementation agent can list a workspace\'s chunks and edges from a fresh pool with no reliance on prior in-memory state', async () => {
    const writePool = openPool();
    const writeRepo = new ChunkGraphRepository(writePool);
    await writeRepo.saveChunk({
      workspaceId: workspaceA,
      ideaLabel: label2,
      chunkType: 'capability',
      discipline: 'product',
      contextKind: 'permanent',
      content: 'Payment gateway integration.',
      status: chunkLifecycleStatus('draft', 'active'),
    });
    await writePool.end();

    const freshPool = openPool();
    const freshRepo = new ChunkGraphRepository(freshPool);
    const chunks = await freshRepo.listChunks(workspaceA);
    const lineages = await freshRepo.listEdgeLineages(workspaceA);
    await freshPool.end();

    expect(chunks.map((c) => c.ideaLabel)).toEqual(
      expect.arrayContaining([label1, label2]),
    );
    expect(lineages.length).toBeGreaterThanOrEqual(1);
  });

  it('tenant isolation: a chunk saved under one workspace is not visible when querying another workspace', async () => {
    const pool = openPool();
    const repo = new ChunkGraphRepository(pool);

    await repo.saveChunk({
      workspaceId: workspaceB,
      ideaLabel: label1,
      chunkType: 'feature',
      discipline: 'engineering',
      contextKind: 'permanent',
      content: 'A different workspace entirely.',
      status: chunkLifecycleStatus('approved', 'active'),
    });

    const crossWorkspaceRead = await repo.findChunk(workspaceA, label1);
    const ownWorkspaceRead = await repo.findChunk(workspaceB, label1);
    const workspaceAChunks = await repo.listChunks(workspaceA);
    await pool.end();

    // workspaceA's own copy of label1 (from the first test) must remain
    // exactly what workspaceA wrote, never workspaceB's content.
    expect(crossWorkspaceRead?.content).not.toBe('A different workspace entirely.');
    expect(ownWorkspaceRead?.content).toBe('A different workspace entirely.');
    expect(
      workspaceAChunks.some((c) => c.content === 'A different workspace entirely.'),
    ).toBe(false);
  });

  it('tenant isolation: an edge lineage saved under one workspace does not appear when listing another workspace', async () => {
    const pool = openPool();
    const repo = new ChunkGraphRepository(pool);

    const lineage = createEdge(workspaceB, label1, label2, 'informs');
    await repo.saveEdgeLineage(lineage);

    const crossWorkspaceRead = await repo.findEdgeLineage(
      workspaceA,
      label1,
      label2,
      'informs',
    );
    const ownWorkspaceRead = await repo.findEdgeLineage(
      workspaceB,
      label1,
      label2,
      'informs',
    );
    const workspaceALineages = await repo.listEdgeLineages(workspaceA);
    await pool.end();

    expect(crossWorkspaceRead).toBeUndefined();
    expect(ownWorkspaceRead).toBeDefined();
    expect(
      workspaceALineages.some(
        (l) => resolveLineage(l)[0]?.relationshipType === 'informs',
      ),
    ).toBe(false);
  });

  it('rejects saving a stale in-memory lineage that is behind stored history', async () => {
    const pool = openPool();
    const repo = new ChunkGraphRepository(pool);
    const source = ideaLabel('IDEA-stale-source');
    const target = ideaLabel('IDEA-stale-target');

    let lineage = createEdge(workspaceA, source, target, 'refines');
    await repo.saveEdgeLineage(lineage);
    lineage = deactivateEdge(lineage);
    await repo.saveEdgeLineage(lineage); // stored: [superseded, deactivated]

    // A stale caller still holding the pre-deactivation ('active') lineage
    // is behind the 2 versions already stored and must be rejected.
    const staleLineage = createEdge(workspaceA, source, target, 'refines');
    await expect(repo.saveEdgeLineage(staleLineage)).rejects.toMatchObject({
      name: 'EdgeLineageError',
      code: 'lineage-violation',
    });

    const stillDeactivated = await repo.findEdgeLineage(
      workspaceA,
      source,
      target,
      'refines',
    );
    await pool.end();

    expect(stillDeactivated!.versions).toHaveLength(2);
    expect(
      stillDeactivated!.versions[stillDeactivated!.versions.length - 1]?.state,
    ).toBe('deactivated');
  });

  it('maps a real concurrent duplicate-insert conflict on the same edge identity to a domain EdgeLineageError', async () => {
    const pool = openPool();
    const repo = new ChunkGraphRepository(pool);
    const source = ideaLabel('IDEA-race-source');
    const target = ideaLabel('IDEA-race-target');

    // Deterministically force a real Postgres unique-constraint conflict
    // (rather than relying on ambiguous Promise.all scheduling, which is
    // not deterministic): hold an uncommitted insert open on one
    // connection so the repository's own insert (on a separate pool
    // connection) blocks on the same primary key, then commit the held
    // transaction so the repository's insert deterministically fails with
    // a real 23505 unique violation once it resumes.
    const blockingClient = await pool.connect();
    await blockingClient.query('BEGIN');
    await blockingClient.query(
      `INSERT INTO edge_versions (
         workspace_id, source_label, target_label, relationship_type, version, state
       ) VALUES ($1, $2, $3, $4, 1, 'active')`,
      [workspaceA, source, target, 'implements'],
    );

    const repoWrite = repo.saveEdgeLineage(
      createEdge(workspaceA, source, target, 'implements'),
    );
    // Give the repository's INSERT time to issue and block behind the
    // still-open, uncommitted transaction above.
    await new Promise((resolve) => setTimeout(resolve, 200));
    await blockingClient.query('COMMIT');
    blockingClient.release();

    await expect(repoWrite).rejects.toMatchObject({ name: 'EdgeLineageError' });

    const persisted = await repo.findEdgeLineage(
      workspaceA,
      source,
      target,
      'implements',
    );
    await pool.end();

    // The blocking client's committed row remains the sole, unambiguous
    // active edge for this identity.
    expect(persisted).toBeDefined();
    expect(persisted!.versions).toHaveLength(1);
    expect(persisted!.versions[0]?.state).toBe('active');
  });
  },
);
