/**
 * Lifecycle state of a single version in a chunk-to-artifact association
 * lineage. Mirrors `EdgeState` (technical spec §"Chunk-artifact association
 * lifecycle": "versioned per branch (active, superseded, deactivated) using
 * the same delta-based model as chunks and edges").
 */
export type ArtifactAssociationState = 'active' | 'superseded' | 'deactivated';
