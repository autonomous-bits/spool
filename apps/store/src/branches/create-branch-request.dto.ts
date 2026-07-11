import { BadRequestException } from '@nestjs/common';
import type { Discipline } from '../domain/types/vocabulary/discipline.js';
import { isDiscipline } from '../domain/types/vocabulary/discipline.js';

/**
 * Validated shape of a `POST /branches` request body, per Meridian IDEA-52/IDEA-34 (branch
 * creation API). Stakeholder registration is out of scope for G02 (mirrors G01's precedent):
 * stakeholderId must already exist as a row in `stakeholders`.
 *
 * G17 SG2 (Meridian IDEA-24/IDEA-17, mirroring G16 SG5's IDEA-139 pattern): authorship
 * attribution (`createdByStakeholderId`) is derived from the verified session token's
 * `stakeholderId` claim, not a client-supplied body field — this interface intentionally has no
 * `stakeholderId` field.
 */
export interface CreateBranchRequest {
  name: string;
  discipline: Discipline;
}

function requireStringField(body: Record<string, unknown>, field: string): string {
  const value = body[field];
  if (typeof value !== 'string' || value.trim().length === 0) {
    throw new BadRequestException(`${field} must be a non-empty string`);
  }
  return value;
}

/**
 * Parses and validates an untrusted HTTP request body into a `CreateBranchRequest`, throwing
 * `BadRequestException` (HTTP 400) for missing fields or invalid vocabulary values. This is the
 * IO boundary guard required for unknown request bodies (typescript-quality: explicit guards at
 * IO boundaries).
 */
export function parseCreateBranchRequest(body: unknown): CreateBranchRequest {
  if (typeof body !== 'object' || body === null) {
    throw new BadRequestException('Request body must be a JSON object');
  }

  const record = body as Record<string, unknown>;

  const name = requireStringField(record, 'name');

  const discipline = record.discipline;
  if (!isDiscipline(discipline)) {
    throw new BadRequestException(`Invalid discipline: ${JSON.stringify(discipline)}`);
  }

  return {
    name,
    discipline,
  } satisfies CreateBranchRequest;
}
