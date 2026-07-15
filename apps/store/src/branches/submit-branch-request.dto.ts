import { BadRequestException } from '@nestjs/common';

/**
 * Validated shape of a `POST /branches/:id/submit` request body (Meridian IDEA-142/IDEA-143,
 * G21 SG4). `activeDiscipline` is the per-request field replacing the old token-baked discipline
 * claim: this parser only guards the IO boundary (must be a string if present at all), not the
 * closed vocabulary or the caller's allow-list — those checks belong to
 * `resolveHumanActorContext` (`requireDiscipline: true`), which throws the precise 400/403 split
 * documented there.
 */
export interface SubmitBranchRequest {
  activeDiscipline: string | undefined;
}

export function parseSubmitBranchRequest(body: unknown): SubmitBranchRequest {
  if (body === undefined || body === null) {
    return { activeDiscipline: undefined };
  }
  if (typeof body !== 'object') {
    throw new BadRequestException('Request body must be a JSON object');
  }

  const record = body as Record<string, unknown>;
  const activeDiscipline = record.activeDiscipline;
  if (activeDiscipline === undefined) {
    return { activeDiscipline: undefined };
  }
  if (typeof activeDiscipline !== 'string') {
    throw new BadRequestException('activeDiscipline must be a string');
  }

  return { activeDiscipline };
}
