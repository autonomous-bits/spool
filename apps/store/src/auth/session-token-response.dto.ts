export interface SessionTokenResponseShape {
  sessionToken: string;
  refreshToken: string;
  expiresAt: number;
}

/**
 * HTTP-facing shape returned by the JSON branch of `GET /auth/github/callback` (Meridian IDEA-81):
 * the minted, store-issued session token plus its paired refresh token and session expiry
 * metadata.
 */
export type SessionTokenResponse = SessionTokenResponseShape;

export function toSessionTokenResponse(tokens: SessionTokenResponseShape): SessionTokenResponse {
  return {
    sessionToken: tokens.sessionToken,
    refreshToken: tokens.refreshToken,
    expiresAt: tokens.expiresAt,
  } satisfies SessionTokenResponse;
}
