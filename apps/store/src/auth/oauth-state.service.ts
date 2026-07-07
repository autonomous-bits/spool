import { Inject, Injectable } from '@nestjs/common';
import { AUTH_CONFIG } from './auth-config.token.js';
import type { AuthConfig } from './auth-config.js';
import { InvalidHmacTokenError, signHmacToken, verifyHmacToken } from './hmac-token.js';

export class InvalidOAuthStateError extends Error {
  constructor(reason: string) {
    super(`Invalid OAuth state: ${reason}`);
    this.name = 'InvalidOAuthStateError';
  }
}

/**
 * Issues and verifies the GitHub OAuth `state` CSRF parameter as a self-contained, HMAC-signed,
 * short-lived token (Meridian IDEA-81: "validate state"). The store has no server-side session
 * store to look `state` up against, so `state` carries its own integrity and expiry instead of
 * being a bare random value checked against stored server state.
 *
 * G11 SG2 (Meridian IDEA-92/IDEA-101): `state` also round-trips the optional `workspaceId` query
 * param a caller supplied to `GET /auth/github/login`, so the callback can recover it after the
 * GitHub redirect round-trip without a server-side session store.
 */
@Injectable()
export class OAuthStateService {
  constructor(@Inject(AUTH_CONFIG) private readonly config: AuthConfig) {}

  issue(workspaceId?: string): string {
    return signHmacToken(
      { workspaceId: workspaceId ?? null },
      this.config.oauthStateSecret,
      this.config.oauthStateMaxAgeSeconds,
    );
  }

  /**
   * Throws `InvalidOAuthStateError` if `state` is missing, malformed, tampered with, or expired.
   * Returns the `workspaceId` embedded at `issue()` time (`null` if none was supplied).
   */
  verify(state: string | undefined): { workspaceId: string | null } {
    if (state === undefined || state.trim().length === 0) {
      throw new InvalidOAuthStateError('missing state parameter');
    }

    let claims: Record<string, unknown>;
    try {
      claims = verifyHmacToken(state, this.config.oauthStateSecret);
    } catch (error) {
      if (error instanceof InvalidHmacTokenError) {
        throw new InvalidOAuthStateError(error.message);
      }
      throw error;
    }

    const workspaceId = claims.workspaceId;
    if (workspaceId !== null && typeof workspaceId !== 'string') {
      throw new InvalidOAuthStateError('state payload has an invalid workspaceId claim');
    }

    return { workspaceId };
  }
}
