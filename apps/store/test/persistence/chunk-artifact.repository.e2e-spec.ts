import { mkdtemp, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import type { Pool } from 'pg';
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it } from 'vitest';
import type { Artifact } from '../../src/domain/artifact.js';
import { Branch } from '../../src/domain/branch.js';
import { ChunkArtifactAssociation } from '../../src/domain/chunk-artifact-association.js';
import { ArtifactRepository } from '../../src/persistence/artifact.repository.js';
import { BranchRepository } from '../../src/persistence/branch.repository.js';
import { ChunkArtifactRepository } from '../../src/persistence/chunk-artifact.repository.js';
import { BOOTSTRAP_STAKEHOLDER_ID } from '../../src/persistence/bootstrap-stakeholder.js';
import { LocalFileBlobStore } from '../../src/persistence/local-file-blob-store.js';
import { setUpTestDatabase, type TestDatabase } from '../support/test-database.js';

const WORKSPACE_ID = '00000000-0000-0000-0000-00000000d0fa';

function buildBranch(overrides: Partial<ConstructorParameters<typeof Branch>[0]> = {}): Branch {
  return new Branch({
    workspaceId: WORKSPACE_ID,
    name: `branch-${Math.random().toString(36).slice(2, 10)}`,
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
    workspaceId: WORKSPACE_ID,
    createdByStakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
    ...overrides,
  });
}

describe('ChunkArtifactRepository (containerized Postgres)', () => {
  let database: TestDatabase;
  let pool: Pool;
  let basePath: string;
  let artifactRepository: ArtifactRepository;
  let chunkArtifactRepository: ChunkArtifactRepository;
  let branchRepository: BranchRepository;
  let artifactA: Artifact;
  let artifactB: Artifact;
  let artifactC: Artifact;

  beforeAll(async () => {
    database = await setUpTestDatabase();
    pool = database.pool;
    chunkArtifactRepository = new ChunkArtifactRepository(pool);
    branchRepository = new BranchRepository(pool);
  });

  afterAll(async () => {
    await database.close();
  });

  beforeEach(async () => {
    basePath = await mkdtemp(join(tmpdir(), 'spool-chunk-artifacts-test-'));
    artifactRepository = new ArtifactRepository(pool, new LocalFileBlobStore({ basePath }));
    artifactA = await artifactRepository.create({
      workspaceId: WORKSPACE_ID,
      content: Buffer.from('artifact A'),
      mimeType: 'text/plain',
      createdByStakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
    });
    artifactB = await artifactRepository.create({
      workspaceId: WORKSPACE_ID,
      content: Buffer.from('artifact B'),
      mimeType: 'text/plain',
      createdByStakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
    });
    artifactC = await artifactRepository.create({
      workspaceId: WORKSPACE_ID,
      content: Buffer.from('artifact C'),
      mimeType: 'text/plain',
      createdByStakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
    });
  });

  afterEach(async () => {
    await rm(basePath, { recursive: true, force: true });
  });

  it('(a) mainline A+B active, no branch rows -> both effective with branchId null', async () => {
    const chunkLabel = `chunk-${Math.random().toString(36).slice(2, 10)}`;
    await chunkArtifactRepository.create(
      buildAssociation({ chunkLabel, artifactId: artifactA.id }),
    );
    await chunkArtifactRepository.create(
      buildAssociation({ chunkLabel, artifactId: artifactB.id }),
    );

    const effective = await chunkArtifactRepository.findEffectiveForChunk(chunkLabel, undefined, WORKSPACE_ID);

    expect(new Map(effective.map((entry) => [entry.artifactId, entry.branchId]))).toEqual(
      new Map([
        [artifactA.id, null],
        [artifactB.id, null],
      ]),
    );
  });

  it('(b) mainline A+B active, branch deactivates A -> only B effective', async () => {
    const chunkLabel = `chunk-${Math.random().toString(36).slice(2, 10)}`;
    const branch = await branchRepository.create(buildBranch());
    await chunkArtifactRepository.create(
      buildAssociation({ chunkLabel, artifactId: artifactA.id }),
    );
    await chunkArtifactRepository.create(
      buildAssociation({ chunkLabel, artifactId: artifactB.id }),
    );
    await chunkArtifactRepository.create(
      buildAssociation({
        chunkLabel,
        artifactId: artifactA.id,
        status: 'deactivated',
        branchId: branch.id,
        originBranchId: branch.id,
      }),
    );

    const effective = await chunkArtifactRepository.findEffectiveForChunk(chunkLabel, branch.id, WORKSPACE_ID);

    expect(new Map(effective.map((entry) => [entry.artifactId, entry.branchId]))).toEqual(
      new Map([[artifactB.id, null]]),
    );
  });

  it('(c) mainline A active, branch adds active C -> A (mainline) + C (branchId) effective', async () => {
    const chunkLabel = `chunk-${Math.random().toString(36).slice(2, 10)}`;
    const branch = await branchRepository.create(buildBranch());
    await chunkArtifactRepository.create(
      buildAssociation({ chunkLabel, artifactId: artifactA.id }),
    );
    await chunkArtifactRepository.create(
      buildAssociation({
        chunkLabel,
        artifactId: artifactC.id,
        branchId: branch.id,
        originBranchId: branch.id,
      }),
    );

    const effective = await chunkArtifactRepository.findEffectiveForChunk(chunkLabel, branch.id, WORKSPACE_ID);

    expect(new Map(effective.map((entry) => [entry.artifactId, entry.branchId]))).toEqual(
      new Map([
        [artifactA.id, null],
        [artifactC.id, branch.id],
      ]),
    );
  });

  it('resolves the most-recently-created row per artifact_id within a single scope (active superseded by a later same-scope deactivation)', async () => {
    const chunkLabel = `chunk-${Math.random().toString(36).slice(2, 10)}`;
    const t0 = new Date('2026-01-01T00:00:00Z');

    await chunkArtifactRepository.create(
      buildAssociation({ chunkLabel, artifactId: artifactA.id, createdAt: t0 }),
    );
    await chunkArtifactRepository.create(
      buildAssociation({
        chunkLabel,
        artifactId: artifactA.id,
        status: 'deactivated',
        createdAt: new Date(t0.getTime() + 1000),
      }),
    );

    const effective = await chunkArtifactRepository.findEffectiveForChunk(chunkLabel, undefined, WORKSPACE_ID);

    expect(effective).toEqual([]);
  });

  it('rejects a duplicate active mainline association (same chunk_label/artifact_id) via idx_chunk_artifacts_mainline', async () => {
    const chunkLabel = `chunk-${Math.random().toString(36).slice(2, 10)}`;
    await chunkArtifactRepository.create(
      buildAssociation({ chunkLabel, artifactId: artifactA.id }),
    );

    await expect(
      chunkArtifactRepository.create(buildAssociation({ chunkLabel, artifactId: artifactA.id })),
    ).rejects.toThrow();
  });

  it('supports an attach -> detach -> re-attach history within a single branch scope (no ratified index blocks it), resolving to the latest active row', async () => {
    const chunkLabel = `chunk-${Math.random().toString(36).slice(2, 10)}`;
    const branch = await branchRepository.create(buildBranch());
    const t0 = new Date('2026-01-01T00:00:00Z');

    await chunkArtifactRepository.create(
      buildAssociation({
        chunkLabel,
        artifactId: artifactA.id,
        branchId: branch.id,
        originBranchId: branch.id,
        createdAt: t0,
      }),
    );
    await chunkArtifactRepository.create(
      buildAssociation({
        chunkLabel,
        artifactId: artifactA.id,
        status: 'deactivated',
        branchId: branch.id,
        originBranchId: branch.id,
        createdAt: new Date(t0.getTime() + 1000),
      }),
    );
    await chunkArtifactRepository.create(
      buildAssociation({
        chunkLabel,
        artifactId: artifactA.id,
        branchId: branch.id,
        originBranchId: branch.id,
        createdAt: new Date(t0.getTime() + 2000),
      }),
    );

    const effective = await chunkArtifactRepository.findEffectiveForChunk(chunkLabel, branch.id, WORKSPACE_ID);

    expect(effective).toEqual([
      { artifactId: artifactA.id, branchId: branch.id, status: 'active' },
    ]);
  });
});
