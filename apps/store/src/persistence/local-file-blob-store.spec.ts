import { randomUUID } from 'node:crypto';
import { mkdtemp, readFile, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { InvalidArtifactUriError, LocalFileBlobStore } from './local-file-blob-store.js';

const WORKSPACE_ID = '00000000-0000-0000-0000-00000000d0fa';

describe('LocalFileBlobStore', () => {
  let basePath: string;
  let store: LocalFileBlobStore;

  beforeEach(async () => {
    basePath = await mkdtemp(join(tmpdir(), 'spool-artifacts-test-'));
    store = new LocalFileBlobStore({ basePath });
  });

  afterEach(async () => {
    await rm(basePath, { recursive: true, force: true });
  });

  it('write persists content and returns a local-file:// uri keyed by workspaceId/artifactId', async () => {
    const artifactId = randomUUID();
    const content = Buffer.from('some artifact content');

    const uri = await store.write(artifactId, content, WORKSPACE_ID);

    expect(uri).toBe(`local-file://${WORKSPACE_ID}/${artifactId}`);
    const written = await readFile(join(basePath, WORKSPACE_ID, artifactId));
    expect(written).toEqual(content);
  });

  it('createReadStream reads back exactly the bytes written for a valid uri', async () => {
    const artifactId = randomUUID();
    const content = Buffer.from('round-trip me');
    const uri = await store.write(artifactId, content, WORKSPACE_ID);

    const chunks: Buffer[] = [];
    for await (const chunk of store.createReadStream(uri)) {
      chunks.push(chunk as Buffer);
    }

    expect(Buffer.concat(chunks)).toEqual(content);
  });

  it('remove deletes the blob so it can no longer be read', async () => {
    const artifactId = randomUUID();
    const uri = await store.write(artifactId, Buffer.from('to be removed'), WORKSPACE_ID);

    await store.remove(uri);

    await expect(readFile(join(basePath, WORKSPACE_ID, artifactId))).rejects.toThrow();
  });

  it('remove is a no-op (does not throw) for a uri whose blob was never written', async () => {
    const uri = `local-file://${WORKSPACE_ID}/${randomUUID()}`;

    await expect(store.remove(uri)).resolves.toBeUndefined();
  });

  it('reads a legacy (pre-G11-SG5) flat local-file://<artifactId> uri from basePath directly', async () => {
    const artifactId = randomUUID();
    const content = Buffer.from('legacy artifact bytes');
    await writeFile(join(basePath, artifactId), content);
    const legacyUri = `local-file://${artifactId}`;

    const chunks: Buffer[] = [];
    for await (const chunk of store.createReadStream(legacyUri)) {
      chunks.push(chunk as Buffer);
    }

    expect(Buffer.concat(chunks)).toEqual(content);
  });

  it('rejects a malformed uri (wrong prefix) to prevent path traversal via createReadStream', () => {
    expect(() => store.createReadStream('not-a-valid-uri')).toThrow(InvalidArtifactUriError);
  });

  it('rejects a uri with a non-UUID artifact id (e.g. path traversal attempt)', () => {
    expect(() => store.createReadStream('local-file://../../etc/passwd')).toThrow(
      InvalidArtifactUriError,
    );
  });

  it('rejects a uri with a non-UUID workspace segment (e.g. path traversal attempt)', () => {
    expect(() => store.createReadStream(`local-file://../../etc/${randomUUID()}`)).toThrow(
      InvalidArtifactUriError,
    );
  });

  it('rejects a uri with too many path segments', () => {
    expect(() =>
      store.createReadStream(`local-file://${WORKSPACE_ID}/${randomUUID()}/extra`),
    ).toThrow(InvalidArtifactUriError);
  });
});
