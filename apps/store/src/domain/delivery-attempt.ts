import type { DeliveryAttemptStatus } from './types/vocabulary/delivery-attempt-status.js';
import { parseDeliveryAttemptStatus } from './types/vocabulary/delivery-attempt-status.js';

export interface DeliveryAttemptProps {
  id: string;
  subscriptionId: string;
  mergeEventId: string;
  branchId: string;
  mergedAt: Date;
  status: DeliveryAttemptStatus;
  attemptCount: number;
  lastAttemptedAt: Date | null;
  nextRetryAt: Date | null;
  createdAt: Date;
  updatedAt: Date;
}

/**
 * DeliveryAttempt entity: one queued/claimed/terminal outbound delivery of a single merge event
 * to a single subscription (Meridian IDEA-127/IDEA-129/IDEA-132, G14 SG1). Always constructed
 * from a persisted row -- like `FeedbackNotification`, there is no client-driven creation path
 * (rows are only ever created by `BranchRepository.merge()`'s fan-out, G14 SG2, and mutated by
 * `DeliveryAttemptRepository`'s claim/complete methods), so this constructor requires every
 * field rather than defaulting `id`/`createdAt`.
 */
export class DeliveryAttempt {
  readonly id: string;
  readonly subscriptionId: string;
  readonly mergeEventId: string;
  readonly branchId: string;
  readonly mergedAt: Date;
  readonly status: DeliveryAttemptStatus;
  readonly attemptCount: number;
  readonly lastAttemptedAt: Date | null;
  readonly nextRetryAt: Date | null;
  readonly createdAt: Date;
  readonly updatedAt: Date;

  constructor(props: DeliveryAttemptProps) {
    this.id = props.id;
    this.subscriptionId = props.subscriptionId;
    this.mergeEventId = props.mergeEventId;
    this.branchId = props.branchId;
    this.mergedAt = props.mergedAt;
    this.status = parseDeliveryAttemptStatus(props.status);
    this.attemptCount = props.attemptCount;
    this.lastAttemptedAt = props.lastAttemptedAt;
    this.nextRetryAt = props.nextRetryAt;
    this.createdAt = props.createdAt;
    this.updatedAt = props.updatedAt;
  }
}
