import type { IssuedArtifactDownloadToken } from './artifact-download-token.service.js';

/**
 * HTTP-facing shape of `GET /artifacts/:id/download-token` (Meridian IDEA-85). `expiresAt` is
 * serialized as an ISO-8601 string by Nest's default JSON serialization of `Date`.
 */
export interface DownloadTokenResponse {
  token: string;
  expiresAt: Date;
}

export function toDownloadTokenResponse(
  issued: IssuedArtifactDownloadToken,
): DownloadTokenResponse {
  return { token: issued.token, expiresAt: issued.expiresAt } satisfies DownloadTokenResponse;
}
