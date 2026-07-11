import { BadRequestException } from '@nestjs/common';
import type { VerificationSignalStatus } from '../domain/types/vocabulary/verification-signal-status.js';
import { isVerificationSignalStatus } from '../domain/types/vocabulary/verification-signal-status.js';

/**
 * Validated shape of a `POST /branches/:id/verification-signals` request body. `verifierName`
 * remains untrusted free text per Meridian IDEA-21, while authenticated reporter identity is
 * derived from verified session-token claims per Meridian IDEA-139, so this interface
 * intentionally has no `reportedByStakeholderId` field. `reason` is optional free text.
 */
export interface CreateVerificationSignalRequest {
  verifierName: string;
  status: VerificationSignalStatus;
  reason?: string;
}

function isNonEmptyString(value: unknown): value is string {
  return typeof value === 'string' && value.trim().length > 0;
}

/**
 * Parses and validates an untrusted HTTP request body into a `CreateVerificationSignalRequest`,
 * throwing `BadRequestException` (HTTP 400) for a blank/missing `verifierName`, an invalid
 * `status`, or a non-string `reason`.
 */
export function parseCreateVerificationSignalRequest(
  body: unknown,
): CreateVerificationSignalRequest {
  if (typeof body !== 'object' || body === null) {
    throw new BadRequestException('Request body must be a JSON object');
  }

  const record = body as Record<string, unknown>;

  const verifierName = record.verifierName;
  if (!isNonEmptyString(verifierName)) {
    throw new BadRequestException('verifierName must be a non-empty string');
  }

  const status = record.status;
  if (!isVerificationSignalStatus(status)) {
    throw new BadRequestException(`Invalid status: ${JSON.stringify(status)}`);
  }

  const reason = record.reason;
  if (reason !== undefined && typeof reason !== 'string') {
    throw new BadRequestException('reason must be a string when provided');
  }

  return {
    verifierName,
    status,
    ...(reason === undefined ? {} : { reason }),
  } satisfies CreateVerificationSignalRequest;
}
