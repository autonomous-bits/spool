/**
 * Vocabulary: ChunkArtifactAssociationStatus enum (Meridian IDEA-60/IDEA-32 branch-scoped delta
 * pattern). 'active' associates an artifact with a chunk; 'deactivated' is a branch-scoped delta
 * row that removes a previously-effective association without deleting history. 'superseded' is
 * schema-level parity with Edge/Chunk status vocabularies only — no G08 write path produces it.
 */
export type ChunkArtifactAssociationStatus = 'active' | 'superseded' | 'deactivated';

const CHUNK_ARTIFACT_ASSOCIATION_STATUSES: readonly ChunkArtifactAssociationStatus[] = [
  'active',
  'superseded',
  'deactivated',
];

export function isChunkArtifactAssociationStatus(
  value: unknown,
): value is ChunkArtifactAssociationStatus {
  return (
    typeof value === 'string' &&
    (CHUNK_ARTIFACT_ASSOCIATION_STATUSES as readonly string[]).includes(value)
  );
}

export function parseChunkArtifactAssociationStatus(
  value: unknown,
): ChunkArtifactAssociationStatus {
  if (!isChunkArtifactAssociationStatus(value)) {
    throw new TypeError(`Invalid ChunkArtifactAssociationStatus: ${JSON.stringify(value)}`);
  }
  return value;
}
