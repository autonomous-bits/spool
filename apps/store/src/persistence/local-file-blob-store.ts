import { createReadStream } from 'node:fs';
import { mkdir, rm, writeFile } from 'node:fs/promises';
import type { Readable } from 'node:stream';
import { Inject, Injectable } from '@nestjs/common';
import type { ArtifactBlobStore } from './artifact-blob-store.js';
import type { LocalFileBlobStoreConfig } from './local-file-blob-store-config.js';
import { LOCAL_FILE_BLOB_STORE_CONFIG } from './local-file-blob-store-config.token.js';

const URI_PREFIX = 'local-file://';
// artifactId/workspaceId are always server-generated UUIDs (Artifact.id/Workspace.id); reject
// anything else outright so a malformed/tampered `uri` can never be used to build a path outside
// `basePath` (no `..`, no path separators reach `join`).
const UUID_PATTERN = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

export class InvalidArtifactUriError extends Error {
  constructor(uri: string) {
    super(`Invalid artifact blob URI: ${uri}`);
    this.name = 'InvalidArtifactUriError';
  }
}

/**
 * A resolved blob location: `workspaceId` is `undefined` for a legacy (pre-G11-SG5) URI written
 * before workspace-prefixed keys existed (Meridian IDEA-93) -- those blobs live directly under
 * `basePath/<artifactId>` and are never moved by this migration (G11 SG1's default-workspace
 * backfill means their `artifacts.workspace_id` row is already correct; only the on-disk blob
 * path is grandfathered).
 */
interface BlobLocation {
  workspaceId: string | undefined;
  artifactId: string;
}

function extractBlobLocation(uri: string): BlobLocation {
  if (!uri.startsWith(URI_PREFIX)) {
    throw new InvalidArtifactUriError(uri);
  }

  const remainder = uri.slice(URI_PREFIX.length);
  const segments = remainder.split('/');

  if (segments.length === 1) {
    // Legacy (pre-G11-SG5) URI shape: `local-file://<artifactId>`.
    const [artifactId] = segments;
    if (artifactId === undefined || !UUID_PATTERN.test(artifactId)) {
      throw new InvalidArtifactUriError(uri);
    }
    return { workspaceId: undefined, artifactId };
  }

  if (segments.length === 2) {
    // Current URI shape (Meridian IDEA-93): `local-file://<workspaceId>/<artifactId>`.
    const [workspaceId, artifactId] = segments;
    if (
      workspaceId === undefined ||
      artifactId === undefined ||
      !UUID_PATTERN.test(workspaceId) ||
      !UUID_PATTERN.test(artifactId)
    ) {
      throw new InvalidArtifactUriError(uri);
    }
    return { workspaceId, artifactId };
  }

  throw new InvalidArtifactUriError(uri);
}

/**
 * `ArtifactBlobStore` backed by local-filesystem storage on a Docker-named-volume (Meridian
 * IDEA-85's resolution of the IDEA-84 gap report: no S3/MinIO service exists in this
 * environment). Each artifact is stored as a single file named after its id, under a
 * `{workspaceId}/` subdirectory of `basePath` (Meridian IDEA-93, G11 SG5); the returned `uri`
 * (`local-file://<workspaceId>/<artifactId>`) is an opaque identifier for this backend only —
 * callers must never construct a filesystem path from it directly. Blobs written before G11 SG5
 * (legacy `local-file://<artifactId>` URIs, no workspace segment) are read from their original
 * flat `basePath/<artifactId>` location rather than migrated, so pre-existing artifacts keep
 * streaming correctly without a data migration.
 */
@Injectable()
export class LocalFileBlobStore implements ArtifactBlobStore {
  constructor(
    @Inject(LOCAL_FILE_BLOB_STORE_CONFIG) private readonly config: LocalFileBlobStoreConfig,
  ) {}

  private pathFor(location: BlobLocation): string {
    return location.workspaceId === undefined
      ? `${this.config.basePath}/${location.artifactId}`
      : `${this.config.basePath}/${location.workspaceId}/${location.artifactId}`;
  }

  async write(artifactId: string, content: Buffer, workspaceId: string): Promise<string> {
    const dir = `${this.config.basePath}/${workspaceId}`;
    await mkdir(dir, { recursive: true });
    await writeFile(`${dir}/${artifactId}`, content);
    return `${URI_PREFIX}${workspaceId}/${artifactId}`;
  }

  createReadStream(uri: string): Readable {
    const location = extractBlobLocation(uri);
    return createReadStream(this.pathFor(location));
  }

  async remove(uri: string): Promise<void> {
    const location = extractBlobLocation(uri);
    await rm(this.pathFor(location), { force: true });
  }
}
