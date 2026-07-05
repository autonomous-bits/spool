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
 */
@Injectable()
export class OAuthStateService {
  constructor(@Inject(AUTH_CONFIG) private readonly config: AuthConfig) {}

  issue(): string {
    return signHmacToken({}, this.config.oauthStateSecret, this.config.oauthStateMaxAgeSeconds);
  }

  /**
   * Throws `InvalidOAuthStateError` if `state` is missing, malformed, tampered with, or expired.
   */
  verify(state: string | undefined): void {
    if (state === undefined || state.trim().length === 0) {
      throw new InvalidOAuthStateError('missing state parameter');
    }

    try {
      verifyHmacToken(state, this.config.oauthStateSecret);
    } catch (error) {
      if (error instanceof InvalidHmacTokenError) {
        throw new InvalidOAuthStateError(error.message);
      }
      throw error;
    }
  }
}
