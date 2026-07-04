/**
 * All machine-readable error codes produced by chunk-artifact association
 * lineage invariants.
 *
 * Technical spec §"Required domain error categories": this feature adds no
 * new error categories — every code below reuses one of the categories
 * already required by feature-01/feature-02 (not found, invalid state
 * transition, duplicate active relationship, lineage violation, tenant
 * boundary violation).
 *
 * - `not-found`                     — no association exists for the
 *   requested identity/scope
 * - `invalid-state-transition`      — deactivation attempted on an
 *   association whose current version is not `active`
 * - `lineage-violation`             — a caller-supplied lineage does not
 *   match the fixed identity it claims, or has zero versions
 * - `duplicate-active-relationship` — a resolved scope (mainline or a single
 *   branch) would contain more than one active association for the same
 *   chunk label and artifact
 * - `tenant-boundary-violation`     — an operation was asked to compare or
 *   resolve associations spanning more than one workspace
 */
export type ArtifactAssociationErrorCode =
  | 'not-found'
  | 'invalid-state-transition'
  | 'lineage-violation'
  | 'duplicate-active-relationship'
  | 'tenant-boundary-violation';

/**
 * Thrown when a chunk-artifact association lineage invariant is violated.
 *
 * The `code` property is stable and machine-readable. Adapters must map
 * domain failures by code, never by inspecting free-form message text.
 *
 * Technical spec §"Required domain error categories".
 */
export class ArtifactAssociationError extends Error {
  override readonly name = 'ArtifactAssociationError';

  constructor(
    readonly code: ArtifactAssociationErrorCode,
    readonly reason: string,
  ) {
    super(reason);
  }
}
