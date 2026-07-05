/**
 * HTTP-facing shape returned by `GET /auth/github/callback` (Meridian IDEA-81): the minted,
 * store-issued session token that human-only endpoints (e.g. branch submit) require in an
 * `Authorization: Bearer <token>` header.
 */
export interface SessionTokenResponse {
  sessionToken: string;
}

export function toSessionTokenResponse(sessionToken: string): SessionTokenResponse {
  return { sessionToken } satisfies SessionTokenResponse;
}
