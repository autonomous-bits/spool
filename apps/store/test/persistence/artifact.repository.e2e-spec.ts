import { mkdtemp, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import type { Pool } from 'pg';
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it } from 'vitest';
import { Workspace } from '../../src/domain/workspace.js';
import { ArtifactRepository } from '../../src/persistence/artifact.repository.js';
import { BOOTSTRAP_STAKEHOLDER_ID } from '../../src/persistence/bootstrap-stakeholder.js';
import { LocalFileBlobStore } from '../../src/persistence/local-file-blob-store.js';
import { WorkspaceRepository } from '../../src/persistence/workspace.repository.js';
import { setUpTestDatabase, type TestDatabase } from '../support/test-database.js';

const WORKSPACE_ID = '00000000-0000-0000-0000-00000000d0fa';

describe('ArtifactRepository (containerized Postgres)', () => {
  let database: TestDatabase;
  let pool: Pool;
  let basePath: string;
  let blobStore: LocalFileBlobStore;
  let artifactRepository: ArtifactRepository;
  let workspaceRepository: WorkspaceRepository;

  beforeAll(async () => {
    database = await setUpTestDatabase();
    pool = database.pool;
    workspaceRepository = new WorkspaceRepository(pool);
  });

  afterAll(async () => {
    await database.close();
  });

  beforeEach(async () => {
    basePath = await mkdtemp(join(tmpdir(), 'spool-artifacts-repo-test-'));
    blobStore = new LocalFileBlobStore({ basePath });
    artifactRepository = new ArtifactRepository(pool, blobStore);
  });

  afterEach(async () => {
    await rm(basePath, { recursive: true, force: true });
  });

  it('create writes the blob via the ArtifactBlobStore then inserts one immutable metadata row', async () => {
    const content = Buffer.from('sample artifact bytes');

    const artifact = await artifactRepository.create({
      workspaceId: WORKSPACE_ID,
      content,
      mimeType: 'text/plain',
      createdByStakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
    });

    expect(artifact.mimeType).toBe('text/plain');
    expect(artifact.uri).toBe(`local-file://${WORKSPACE_ID}/${artifact.id}`);

    const row = await pool.query<{ count: string }>(
      'SELECT COUNT(*)::text AS count FROM artifacts WHERE id = $1 AND workspace_id = $2',
      [artifact.id, WORKSPACE_ID],
    );
    expect(row.rows[0]?.count).toBe('1');

    const readBack: Buffer[] = [];
    for await (const chunk of blobStore.createReadStream(artifact.uri)) {
      readBack.push(chunk as Buffer);
    }
    expect(Buffer.concat(readBack)).toEqual(content);
  });

  it('findById round-trips a persisted artifact', async () => {
    const created = await artifactRepository.create({
      workspaceId: WORKSPACE_ID,
      content: Buffer.from('another artifact'),
      mimeType: 'application/octet-stream',
      createdByStakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
    });

    const found = await artifactRepository.findById(created.id, WORKSPACE_ID);

    expect(found).toEqual(created);
  });

  it('findById returns an explicit not-found result (undefined) for an unknown id', async () => {
    const found = await artifactRepository.findById(
      '00000000-0000-0000-0000-00000000dead',
      WORKSPACE_ID,
    );

    expect(found).toBeUndefined();
  });

  it('findById returns undefined (not the row) when the id exists but in a different workspace', async () => {
    const otherWorkspace = await workspaceRepository.createWithFirstMember(
      new Workspace({ name: `artifact-workspace-${String(Date.now())}`, createdByStakeholderId: BOOTSTRAP_STAKEHOLDER_ID }),
    );
    const created = await artifactRepository.create({
      workspaceId: WORKSPACE_ID,
      content: Buffer.from('scoped'),
      mimeType: 'text/plain',
      createdByStakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
    });

    const found = await artifactRepository.findById(created.id, otherWorkspace.id);

    expect(found).toBeUndefined();
  });

  it('never mutates an existing artifact row: each create produces a distinct id/uri', async () => {
    const first = await artifactRepository.create({
      workspaceId: WORKSPACE_ID,
      content: Buffer.from('version one'),
      mimeType: 'text/plain',
      createdByStakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
    });
    const second = await artifactRepository.create({
      workspaceId: WORKSPACE_ID,
      content: Buffer.from('version two'),
      mimeType: 'text/plain',
      createdByStakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
    });

    expect(second.id).not.toBe(first.id);
    expect(second.uri).not.toBe(first.uri);

    const firstStillReadable = await artifactRepository.findById(first.id, WORKSPACE_ID);
    expect(firstStillReadable?.uri).toBe(first.uri);
  });
});
