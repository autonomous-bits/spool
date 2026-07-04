/**
 * All machine-readable error codes produced by feedback/verification-signal
 * notification-routing invariants.
 *
 * Technical spec §"Required domain error categories": this feature adds no
 * new error categories — every code below reuses one of the categories
 * already required by feature-01/feature-02 (not found, invalid state
 * transition, tenant boundary violation), same precedent as
 * `ArtifactAssociationError`/`BranchLifecycleError` gaining `'not-found'` in
 * earlier stories, and `BranchLifecycleError`/`ArtifactAssociationError`
 * reusing `'invalid-state-transition'` for a duplicate-identity write.
 *
 * - `not-found`                 — no notification/feedback/signal exists for
 *   the requested workspace/identity, or the target branch has no durably
 *   recorded author to notify (see `NotificationRepository`)
 * - `invalid-state-transition`  — a feedback item, verification signal, or
 *   notification with this identity is already persisted in this workspace
 * - `tenant-boundary-violation` — an operation was asked to read or
 *   acknowledge a record outside its own workspace
 */
export type NotificationErrorCode = 'not-found' | 'invalid-state-transition' | 'tenant-boundary-violation';

/**
 * Thrown when a notification-routing invariant is violated.
 *
 * The `code` property is stable and machine-readable. Adapters must map
 * domain failures by code, never by inspecting free-form message text.
 *
 * Technical spec §"Required domain error categories".
 */
export class NotificationError extends Error {
  override readonly name = 'NotificationError';

  constructor(
    readonly code: NotificationErrorCode,
    readonly reason: string,
  ) {
    super(reason);
  }
}
