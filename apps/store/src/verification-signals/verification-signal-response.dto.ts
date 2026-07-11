import type { VerificationSignal } from '../domain/verification-signal.js';
import type { VerificationSignalStatus } from '../domain/types/vocabulary/verification-signal-status.js';

/**
 * HTTP-facing shape of a persisted VerificationSignal. Responses surface both the untrusted
 * free-text `verifierName` and the authenticated claims-derived `reportedByStakeholderId`.
 */
export interface VerificationSignalResponse {
  id: string;
  branchId: string;
  reportedByStakeholderId: string;
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
    reportedByStakeholderId: signal.reportedByStakeholderId,
    verifierName: signal.verifierName,
    status: signal.status,
    reason: signal.reason ?? null,
    createdAt: signal.createdAt,
  } satisfies VerificationSignalResponse;
}
