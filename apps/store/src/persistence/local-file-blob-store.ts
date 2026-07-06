import { createReadStream } from 'node:fs';
import { mkdir, rm, writeFile } from 'node:fs/promises';
import type { Readable } from 'node:stream';
import { Inject, Injectable } from '@nestjs/common';
import type { ArtifactBlobStore } from './artifact-blob-store.js';
import type { LocalFileBlobStoreConfig } from './local-file-blob-store-config.js';
import { LOCAL_FILE_BLOB_STORE_CONFIG } from './local-file-blob-store-config.token.js';

const URI_PREFIX = 'local-file://';
// artifactId is always a server-generated UUID (Artifact.id); reject anything else outright so a
// malformed/tampered `uri` can never be used to build a path outside `basePath` (no `..`, no
// path separators reach `join`).
const UUID_PATTERN = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

export class InvalidArtifactUriError extends Error {
  constructor(uri: string) {
    super(`Invalid artifact blob URI: ${uri}`);
    this.name = 'InvalidArtifactUriError';
  }
}

function extractArtifactId(uri: string): string {
  if (!uri.startsWith(URI_PREFIX)) {
    throw new InvalidArtifactUriError(uri);
  }

  const artifactId = uri.slice(URI_PREFIX.length);
  if (!UUID_PATTERN.test(artifactId)) {
    throw new InvalidArtifactUriError(uri);
  }

  return artifactId;
}

/**
 * `ArtifactBlobStore` backed by local-filesystem storage on a Docker-named-volume (Meridian
 * IDEA-85's resolution of the IDEA-84 gap report: no S3/MinIO service exists in this
 * environment). Each artifact is stored as a single file named after its id under `basePath`;
 * the returned `uri` (`local-file://<artifactId>`) is an opaque identifier for this backend only
 * — callers must never construct a filesystem path from it directly.
 */
@Injectable()
export class LocalFileBlobStore implements ArtifactBlobStore {
  constructor(
    @Inject(LOCAL_FILE_BLOB_STORE_CONFIG) private readonly config: LocalFileBlobStoreConfig,
  ) {}

  private pathFor(artifactId: string): string {
    return `${this.config.basePath}/${artifactId}`;
  }

  async write(artifactId: string, content: Buffer): Promise<string> {
    await mkdir(this.config.basePath, { recursive: true });
    await writeFile(this.pathFor(artifactId), content);
    return `${URI_PREFIX}${artifactId}`;
  }

  createReadStream(uri: string): Readable {
    const artifactId = extractArtifactId(uri);
    return createReadStream(this.pathFor(artifactId));
  }

  async remove(uri: string): Promise<void> {
    const artifactId = extractArtifactId(uri);
    await rm(this.pathFor(artifactId), { force: true });
  }
}
