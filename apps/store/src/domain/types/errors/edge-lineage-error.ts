/**
 * All machine-readable error codes produced by edge lineage and determinism
 * invariants.
 *
 * Technical spec §"Required domain error categories":
 * - `duplicate-active-relationship` — a resolved view would contain more than
 *   one active edge for the same source label, target label, and relationship
 *   type
 * - `lineage-violation`             — a supersession would rewrite the fixed
 *   workspace/source/target/type identity of a relationship lineage
 * - `invalid-state-transition`      — supersession or deactivation attempted
 *   on a lineage whose current version is not active
 * - `tenant-boundary-violation`     — a resolved relationship view was asked
 *   to validate lineages spanning more than one workspace
 */
export type EdgeLineageErrorCode =
  | 'duplicate-active-relationship'
  | 'lineage-violation'
  | 'invalid-state-transition'
  | 'tenant-boundary-violation';

/**
 * Thrown when an edge lineage or determinism invariant is violated.
 *
 * The `code` property is stable and machine-readable. Adapters must map
 * domain failures by code, never by inspecting free-form message text.
 *
 * Technical spec §"Required domain error categories".
 */
export class EdgeLineageError extends Error {
  override readonly name = 'EdgeLineageError';

  constructor(
    readonly code: EdgeLineageErrorCode,
    readonly reason: string,
  ) {
    super(reason);
  }
}
