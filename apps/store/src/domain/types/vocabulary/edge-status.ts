/**
 * Vocabulary: EdgeStatus enum, per Meridian IDEA-38 (mainline edges are immutable; relationships
 * are only modified or deactivated by superseding with a new edge version, chained via
 * supersededByEdgeId). This goal's write path only ever produces 'active'; 'superseded' and
 * 'deactivated' are modeled here so later goals can round-trip them once the supersede/deactivate
 * command exists.
 */
export type EdgeStatus = 'active' | 'superseded' | 'deactivated';

const EDGE_STATUSES: readonly EdgeStatus[] = ['active', 'superseded', 'deactivated'];

export function isEdgeStatus(value: unknown): value is EdgeStatus {
  return typeof value === 'string' && (EDGE_STATUSES as readonly string[]).includes(value);
}

export function parseEdgeStatus(value: unknown): EdgeStatus {
  if (!isEdgeStatus(value)) {
    throw new TypeError(`Invalid EdgeStatus: ${JSON.stringify(value)}`);
  }
  return value;
}
