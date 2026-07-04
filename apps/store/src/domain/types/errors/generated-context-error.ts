/**
 * All machine-readable error codes produced by generated-context provenance
 * invariants.
 *
 * Technical spec §"Required domain error categories":
 * - `invalid-state-transition`       — a chunk that is not approved/promoted
 *   + active, or a relationship that is not active (or whose endpoint chunks
 *   are not approved/promoted + active), was offered as generated-context
 *   source material
 * - `tenant-boundary-violation`      — a generated context package was asked
 *   to include an item from a different workspace than the package itself
 * - `duplicate-active-relationship`  — a package would contain more than one
 *   active relationship item for the same source label, target label, and
 *   relationship type
 */
export type GeneratedContextErrorCode =
  | 'invalid-state-transition'
  | 'tenant-boundary-violation'
  | 'duplicate-active-relationship';

/**
 * Thrown when a generated-context provenance or packaging invariant is
 * violated.
 *
 * The `code` property is stable and machine-readable. Adapters must map
 * domain failures by code, never by inspecting free-form message text.
 *
 * Technical spec §"Required domain error categories".
 */
export class GeneratedContextError extends Error {
  override readonly name = 'GeneratedContextError';

  constructor(
    readonly code: GeneratedContextErrorCode,
    readonly reason: string,
  ) {
    super(reason);
  }
}
