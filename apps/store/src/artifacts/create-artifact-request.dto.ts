import { BadRequestException } from '@nestjs/common';

/**
 * Validated shape of a `POST /artifacts` request body, per Meridian IDEA-52/IDEA-34 (API gateway
 * boundary) and IDEA-58/IDEA-59 (artifacts as standalone immutable blobs). `content` is the
 * artifact's raw bytes, base64-encoded for JSON transport (mirrors the MCP `upload-artifact` tool
 * planned for SG6, which forwards base64 inline content the same way).
 */
export interface CreateArtifactRequest {
  content: string;
  mimeType: string;
  stakeholderId: string;
}

function requireStringField(body: Record<string, unknown>, field: string): string {
  const value = body[field];
  if (typeof value !== 'string' || value.trim().length === 0) {
    throw new BadRequestException(`${field} must be a non-empty string`);
  }
  return value;
}

const BASE64_PATTERN = /^[A-Za-z0-9+/]*={0,2}$/;

/**
 * Parses and validates an untrusted HTTP request body into a `CreateArtifactRequest`, throwing
 * `BadRequestException` (HTTP 400) for missing fields or malformed base64 content. This is the IO
 * boundary guard required for unknown request bodies (typescript-quality: explicit guards at IO
 * boundaries).
 */
export function parseCreateArtifactRequest(body: unknown): CreateArtifactRequest {
  if (typeof body !== 'object' || body === null) {
    throw new BadRequestException('Request body must be a JSON object');
  }

  const record = body as Record<string, unknown>;

  const content = requireStringField(record, 'content');
  const mimeType = requireStringField(record, 'mimeType');
  const stakeholderId = requireStringField(record, 'stakeholderId');

  if (!BASE64_PATTERN.test(content) || content.length % 4 !== 0) {
    throw new BadRequestException('content must be base64-encoded');
  }

  return { content, mimeType, stakeholderId } satisfies CreateArtifactRequest;
}
