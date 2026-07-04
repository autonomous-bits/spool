/**
 * Notification routing domain module: an immutable, workspace/branch-scoped
 * alert record pointing at the feedback item or verification signal that
 * triggered it, and a non-destructive acknowledgement operation.
 *
 * Sources of authority:
 * - Story:          docs/specifications/feature-02-postgres-persistence/stories/S09-feedback-and-verification-notifications-remembered.md
 * - Technical spec: docs/specifications/feature-02-postgres-persistence/technical-specification.md
 *                   §"Feedback notification routing",
 *                   §"Notification acknowledgement is non-destructive"
 * - Constitution:   docs/constitution.md (Principle IV — Rich Domain Models)
 * - Meridian:       IDEA-67 ("dispatches unread notification objects to the
 *   author of the branch and other stakeholders when evaluation feedback is
 *   submitted"), IDEA-68 ("Evaluation feedback and verification signals are
 *   routed to stakeholders immediately upon ingestion, storing notifications
 *   in a dedicated table")
 *
 * Story: S09 — Remember feedback and verification notifications without
 * losing the record.
 *
 * AC3 (non-destructive acknowledgement): `Notification` carries only a
 * reference (`source`) to the feedback/signal record, never a copy of its
 * content, and `acknowledgeNotification` returns a new `Notification` value
 * with only `acknowledgedAt` changed — there is no function in this module
 * that can mutate or delete a `FeedbackItem`/`VerificationSignal`.
 *
 * On "author must be notified" (AC2): this module intentionally has no
 * "resolve the branch author" function — a caller-supplied claim of who the
 * author is would be an unverified actor claim, the exact thing AC5
 * prohibits. The persistence adapter (`NotificationRepository`) resolves the
 * author recipient itself from the durably stored `branches` row, never
 * from request input (see that module's doc header for the full rationale).
 */

import type { BranchId } from '../identifiers/branch-id.js';
import type { FeedbackItemId } from '../identifiers/feedback-item-id.js';
import type { NotificationId } from '../identifiers/notification-id.js';
import type { StakeholderId } from '../identifiers/stakeholder-id.js';
import type { VerificationSignalId } from '../identifiers/verification-signal-id.js';
import type { WorkspaceId } from '../identifiers/workspace-id.js';
import { VocabularyValidationError } from '../errors/vocabulary-validation-error.js';

/**
 * The feedback/verification record that triggered a notification. A
 * discriminated union so a notification always points unambiguously at
 * exactly one underlying record — never both, never neither.
 */
export type NotificationSource =
  | { readonly kind: 'feedback-item'; readonly feedbackItemId: FeedbackItemId }
  | { readonly kind: 'verification-signal'; readonly signalId: VerificationSignalId };

/**
 * A routed notification alerting a stakeholder to new feedback/verification
 * activity on a branch.
 *
 * `acknowledgedAt` is `undefined` for an unread notification (Meridian
 * IDEA-67: "unread notification objects") and set, once, by
 * `acknowledgeNotification`.
 */
export interface Notification {
  readonly workspaceId: WorkspaceId;
  readonly branchId: BranchId;
  readonly notificationId: NotificationId;
  readonly recipientStakeholderId: StakeholderId;
  readonly source: NotificationSource;
  readonly createdAt: string;
  readonly acknowledgedAt?: string;
}

function assertIsoTimestamp(concept: string, value: string): string {
  const trimmed = value.trim();
  if (
    !trimmed ||
    !/^\d{4}-\d{2}-\d{2}/.test(trimmed) ||
    Number.isNaN(Date.parse(trimmed))
  ) {
    throw new VocabularyValidationError(concept, `${concept} must be a valid ISO-8601 date string`);
  }
  return trimmed;
}

/**
 * Constructs a notification routing a feedback/verification source to one
 * recipient stakeholder for one branch.
 *
 * This is a pure value constructor; persisting one row per resolved
 * recipient (technical spec §"Feedback notification routing" — "At minimum,
 * the author of the evaluated branch must be notified; other relevant
 * stakeholders may also be notified") is the persistence adapter's
 * responsibility.
 */
export function routeNotification(
  workspaceId: WorkspaceId,
  branchId: BranchId,
  notificationId: NotificationId,
  recipientStakeholderId: StakeholderId,
  source: NotificationSource,
  createdAt: string,
): Notification {
  return Object.freeze({
    workspaceId,
    branchId,
    notificationId,
    recipientStakeholderId,
    source,
    createdAt: assertIsoTimestamp('Notification.createdAt', createdAt),
  });
}

/**
 * Acknowledges a notification without mutating or deleting the underlying
 * feedback/verification record it references (AC3; technical spec
 * §"Notification acknowledgement is non-destructive").
 *
 * Idempotent: acknowledging an already-acknowledged notification returns the
 * same `acknowledgedAt` it already had rather than overwriting it with a
 * later timestamp — acknowledgement is a fact that happened once, not a
 * "last acknowledged" timestamp.
 */
export function acknowledgeNotification(
  notification: Notification,
  acknowledgedAt: string,
): Notification {
  if (notification.acknowledgedAt !== undefined) {
    return notification;
  }
  return Object.freeze({
    ...notification,
    acknowledgedAt: assertIsoTimestamp('Notification.acknowledgedAt', acknowledgedAt),
  });
}

/**
 * Returns true when a notification has not yet been acknowledged (Meridian
 * IDEA-67: "unread notification objects").
 */
export function isUnread(notification: Notification): boolean {
  return notification.acknowledgedAt === undefined;
}

/**
 * Resolves the deduplicated set of stakeholder ids that should receive a
 * notification for one feedback/verification-signal ingestion event, given
 * the branch's durably recorded author (resolved by the persistence
 * adapter — never caller-claimed) and any additional relevant stakeholders.
 * The author is always included, first, satisfying AC2's "at minimum" rule
 * structurally rather than by convention.
 */
export function resolveNotificationRecipients(
  authorStakeholderId: StakeholderId,
  additionalStakeholderIds: readonly StakeholderId[] = [],
): StakeholderId[] {
  const seen = new Set<StakeholderId>([authorStakeholderId]);
  const recipients = [authorStakeholderId];
  for (const stakeholderId of additionalStakeholderIds) {
    if (!seen.has(stakeholderId)) {
      seen.add(stakeholderId);
      recipients.push(stakeholderId);
    }
  }
  return recipients;
}
