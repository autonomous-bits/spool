/**
 * DI token for the `ArtifactBlobStore` instance provided by PersistenceModule. See
 * `pg-pool.token.ts` for the same symbol-token convention used elsewhere in this codebase.
 */
export const ARTIFACT_BLOB_STORE = Symbol('ARTIFACT_BLOB_STORE');
