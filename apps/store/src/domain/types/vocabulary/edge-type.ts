/**
 * Vocabulary: EdgeType enum, ratified by Meridian IDEA-36/IDEA-37/IDEA-38 (typed graph
 * relationships between chunks, referenced by logical label). Closed to exactly these eight
 * values describing how one chunk relates to another.
 */
export type EdgeType =
  | 'refines'
  | 'depends-on'
  | 'contradicts'
  | 'derives-from'
  | 'blocks'
  | 'implements'
  | 'constrains'
  | 'feedback-on';

const EDGE_TYPES: readonly EdgeType[] = [
  'refines',
  'depends-on',
  'contradicts',
  'derives-from',
  'blocks',
  'implements',
  'constrains',
  'feedback-on',
];

export function isEdgeType(value: unknown): value is EdgeType {
  return typeof value === 'string' && (EDGE_TYPES as readonly string[]).includes(value);
}

export function parseEdgeType(value: unknown): EdgeType {
  if (!isEdgeType(value)) {
    throw new TypeError(`Invalid EdgeType: ${JSON.stringify(value)}`);
  }
  return value;
}
