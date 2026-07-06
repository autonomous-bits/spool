import type { VerificationSignal } from '../domain/verification-signal.js';
import type { VerificationSignalStatus } from '../domain/types/vocabulary/verification-signal-status.js';

/**
 * HTTP-facing shape of a persisted VerificationSignal, per Meridian IDEA-21/IDEA-31. Kept as an
 * explicit interface (rather than returning the domain entity directly) so the API response
 * contract is typed independently of the domain entity's internal shape.
 */
export interface VerificationSignalResponse {
  id: string;
  branchId: string;
  verifierName: string;
  status: VerificationSignalStatus;
  reason: string | null;
  createdAt: Date;
}

export function toVerificationSignalResponse(
  signal: VerificationSignal,
): VerificationSignalResponse {
  return {
    id: signal.id,
    branchId: signal.branchId,
    verifierName: signal.verifierName,
    status: signal.status,
    reason: signal.reason ?? null,
    createdAt: signal.createdAt,
  } satisfies VerificationSignalResponse;
}
