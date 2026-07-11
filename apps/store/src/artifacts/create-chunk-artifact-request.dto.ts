import { BadRequestException } from '@nestjs/common';

/**
 * Validated shape of a `POST /chunks/:label/artifacts` request body, per Meridian IDEA-52/IDEA-34
 * (API gateway boundary) and IDEA-32/IDEA-60 (branch-scoped delta associations). `chunkLabel`
 * comes from the route param, not the body. `branchId` is optional, mirroring the edges/chunks
 * precedent: when present, the association is scoped to that draft branch; when absent, it is a
 * mainline association.
 *
 * G18 SG3 (Meridian IDEA-139): authorship attribution (`createdByStakeholderId`) is derived from
 * the verified session token's `stakeholderId` claim, not a client-supplied body field — this
 * interface intentionally has no `stakeholderId` field.
 */
export interface CreateChunkArtifactRequest {
  artifactId: string;
  branchId?: string;
}

function requireStringField(body: Record<string, unknown>, field: string): string {
  const value = body[field];
  if (typeof value !== 'string' || value.trim().length === 0) {
    throw new BadRequestException(`${field} must be a non-empty string`);
  }
  return value;
}

/**
 * Parses and validates an untrusted HTTP request body into a `CreateChunkArtifactRequest`,
 * throwing `BadRequestException` (HTTP 400) for missing fields. This is the IO boundary guard
 * required for unknown request bodies (typescript-quality: explicit guards at IO boundaries).
 */
export function parseCreateChunkArtifactRequest(body: unknown): CreateChunkArtifactRequest {
  if (typeof body !== 'object' || body === null) {
    throw new BadRequestException('Request body must be a JSON object');
  }

  const record = body as Record<string, unknown>;

  const artifactId = requireStringField(record, 'artifactId');

  const branchIdValue = record.branchId;
  if (branchIdValue !== undefined) {
    if (typeof branchIdValue !== 'string' || branchIdValue.trim().length === 0) {
      throw new BadRequestException('branchId must be a non-empty string when provided');
    }
    return { artifactId, branchId: branchIdValue } satisfies CreateChunkArtifactRequest;
  }

  return { artifactId } satisfies CreateChunkArtifactRequest;
}
