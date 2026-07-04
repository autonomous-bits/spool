/**
 * Notification routing domain module (top-level re-export, mirrors
 * `domain/verification-signal.ts`'s convention): feedback/verification
 * notification records and their non-destructive acknowledgement.
 *
 * Sources of authority:
 * - Story:          docs/specifications/feature-02-postgres-persistence/stories/S09-feedback-and-verification-notifications-remembered.md
 * - Technical spec: docs/specifications/feature-02-postgres-persistence/technical-specification.md
 *                   §"Feedback notification routing",
 *                   §"Notification acknowledgement is non-destructive",
 *                   §"Protected operation contracts"
 * - Constitution:   docs/constitution.md (Principle IV — Rich Domain Models)
 * - Meridian:       IDEA-67, IDEA-68
 *
 * Story: S09 — Remember feedback and verification notifications without
 * losing the record.
 */

export {
  recordFeedbackItem,
  type FeedbackItem,
  type FeedbackTargetBranch,
} from './types/index.js';

export {
  routeNotification,
  acknowledgeNotification,
  isUnread,
  resolveNotificationRecipients,
  type Notification,
  type NotificationSource,
} from './types/index.js';

export type { VerificationTargetBranch } from './types/index.js';
