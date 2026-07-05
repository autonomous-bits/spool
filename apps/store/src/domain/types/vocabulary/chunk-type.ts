/**
 * Vocabulary: ChunkType enum, ratified by Meridian IDEA-72. Closed to exactly these five values;
 * `feature`/`capability` describe user-facing behavior, `constraint` is a non-negotiable rule,
 * `adr` is an architecture decision record, `spike` is exploratory/time-boxed research.
 */
export type ChunkType = 'feature' | 'capability' | 'constraint' | 'adr' | 'spike';

const CHUNK_TYPES: readonly ChunkType[] = ['feature', 'capability', 'constraint', 'adr', 'spike'];

export function isChunkType(value: unknown): value is ChunkType {
  return typeof value === 'string' && (CHUNK_TYPES as readonly string[]).includes(value);
}

export function parseChunkType(value: unknown): ChunkType {
  if (!isChunkType(value)) {
    throw new TypeError(`Invalid ChunkType: ${JSON.stringify(value)}`);
  }
  return value;
}
