import { randomUUID } from 'node:crypto';
import { mkdtemp, readFile, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { InvalidArtifactUriError, LocalFileBlobStore } from './local-file-blob-store.js';

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

  it('write persists content and returns a local-file:// uri keyed by the artifact id', async () => {
    const artifactId = randomUUID();
    const content = Buffer.from('some artifact content');

    const uri = await store.write(artifactId, content);

    expect(uri).toBe(`local-file://${artifactId}`);
    const written = await readFile(join(basePath, artifactId));
    expect(written).toEqual(content);
  });

  it('createReadStream reads back exactly the bytes written for a valid uri', async () => {
    const artifactId = randomUUID();
    const content = Buffer.from('round-trip me');
    const uri = await store.write(artifactId, content);

    const chunks: Buffer[] = [];
    for await (const chunk of store.createReadStream(uri)) {
      chunks.push(chunk as Buffer);
    }

    expect(Buffer.concat(chunks)).toEqual(content);
  });

  it('remove deletes the blob so it can no longer be read', async () => {
    const artifactId = randomUUID();
    const uri = await store.write(artifactId, Buffer.from('to be removed'));

    await store.remove(uri);

    await expect(readFile(join(basePath, artifactId))).rejects.toThrow();
  });

  it('remove is a no-op (does not throw) for a uri whose blob was never written', async () => {
    const uri = `local-file://${randomUUID()}`;

    await expect(store.remove(uri)).resolves.toBeUndefined();
  });

  it('rejects a malformed uri (wrong prefix) to prevent path traversal via createReadStream', () => {
    expect(() => store.createReadStream('not-a-valid-uri')).toThrow(InvalidArtifactUriError);
  });

  it('rejects a uri with a non-UUID artifact id (e.g. path traversal attempt)', () => {
    expect(() => store.createReadStream('local-file://../../etc/passwd')).toThrow(
      InvalidArtifactUriError,
    );
  });
});
