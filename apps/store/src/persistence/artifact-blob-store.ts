import type { Readable } from 'node:stream';

/**
 * Port for artifact blob storage (Meridian IDEA-61/IDEA-85). Domain/repository code depends only
 * on this interface, never on a concrete backend, so the backend can be swapped for a real
 * S3-compatible object store later without touching `ArtifactRepository` or any API/MCP code.
 * The only implementation today is `LocalFileBlobStore` (local-filesystem storage on a
 * Docker-named-volume, per IDEA-85's resolution of the IDEA-84 gap report).
 */
export interface ArtifactBlobStore {
  /**
   * Persists `content` for `artifactId` and returns the URI recorded in the `artifacts` table.
   * Callers must treat the written blob as immutable afterward (IDEA-59): there is no `update`.
   */
  write(artifactId: string, content: Buffer): Promise<string>;

  /**
   * Opens a readable stream over the blob at `uri`, for large-file-safe streaming reads (signed
   * download responses, SG4) rather than buffering whole files into memory.
   */
  createReadStream(uri: string): Readable;

  /**
   * Best-effort removal of the blob at `uri`. Used to compensate a blob write when the paired
   * metadata-row insert fails, so storage and metadata don't diverge. Implementations must not
   * throw for a missing/already-removed blob.
   */
  remove(uri: string): Promise<void>;
}
