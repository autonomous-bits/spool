/**
 * Vocabulary: DeliveryAttemptStatus enum, per Meridian IDEA-127 (promoted: base
 * pending/succeeded/failed) as amended by IDEA-129 (promoted: adds `in_progress`, the
 * claim-queue state set atomically by `DeliveryAttemptRepository.claimBatch` while a worker
 * owns the outbound delivery). `pending` also covers "due for retry" -- a failed attempt that
 * hasn't exhausted its 3 tries is set back to `pending` with `next_retry_at` populated, not a
 * separate `retrying` state (G14 SG1).
 */
export type DeliveryAttemptStatus = 'pending' | 'in_progress' | 'succeeded' | 'failed';

const DELIVERY_ATTEMPT_STATUSES: readonly DeliveryAttemptStatus[] = [
  'pending',
  'in_progress',
  'succeeded',
  'failed',
];

export function isDeliveryAttemptStatus(value: unknown): value is DeliveryAttemptStatus {
  return typeof value === 'string' && (DELIVERY_ATTEMPT_STATUSES as readonly string[]).includes(value);
}

export function parseDeliveryAttemptStatus(value: unknown): DeliveryAttemptStatus {
  if (!isDeliveryAttemptStatus(value)) {
    throw new TypeError(`Invalid DeliveryAttemptStatus: ${JSON.stringify(value)}`);
  }
  return value;
}
