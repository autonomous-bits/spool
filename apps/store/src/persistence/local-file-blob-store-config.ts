import { tmpdir } from 'node:os';
import { join } from 'node:path';

export interface LocalFileBlobStoreConfig {
  basePath: string;
}

/**
 * Builds `LocalFileBlobStore`'s base directory from `ARTIFACT_BLOB_STORE_PATH`, mirroring
 * `database-config.ts`'s convention of a local dev default so unit/e2e tests run without extra
 * setup. Docker Compose sets `ARTIFACT_BLOB_STORE_PATH` explicitly to a path backed by the
 * `spoolstore-artifact-data` named volume (Meridian IDEA-85), so the default here is only ever
 * exercised on the host (tests, `pnpm dev`), never in the containerized runtime.
 */
export function loadLocalFileBlobStoreConfig(
  env: NodeJS.ProcessEnv = process.env,
): LocalFileBlobStoreConfig {
  const basePath = env.ARTIFACT_BLOB_STORE_PATH ?? join(tmpdir(), 'spool-artifacts');

  return { basePath } satisfies LocalFileBlobStoreConfig;
}
