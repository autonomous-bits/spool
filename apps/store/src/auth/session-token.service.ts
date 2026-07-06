import { Inject, Injectable } from '@nestjs/common';
import { AUTH_CONFIG } from './auth-config.token.js';
import type { AuthConfig } from './auth-config.js';
import { InvalidHmacTokenError, signHmacToken, verifyHmacToken } from './hmac-token.js';

export class InvalidSessionTokenError extends Error {
  constructor(reason: string) {
    super(`Invalid session token: ${reason}`);
    this.name = 'InvalidSessionTokenError';
  }
}

/**
 * Claims carried by a store-issued session token, per Meridian IDEA-81. `discipline` is
 * nullable because not every stakeholder has one assigned. `authTime` (epoch seconds) records
 * when the interactive GitHub OAuth login completed, per IDEA-81's auth_time-equivalent claim.
 */
export interface SessionTokenClaims {
  stakeholderId: string;
  discipline: string | null;
  authTime: number;
}

function isValidClaims(claims: Record<string, unknown>): claims is SessionTokenClaims & Record<string, unknown> {
  const stakeholderId = claims.stakeholderId;
  const discipline = claims.discipline;
  const authTime = claims.authTime;

  return (
    typeof stakeholderId === 'string' &&
    (discipline === null || typeof discipline === 'string') &&
    typeof authTime === 'number'
  );
}

/**
 * Mints and verifies short-lived, store-issued session tokens (Meridian IDEA-81). Only a
 * completed GitHub OAuth login (via `AuthService`) ever produces claims for `sign`; callers
 * must never construct `SessionTokenClaims` from a client-supplied body field.
 */
@Injectable()
export class SessionTokenService {
  constructor(@Inject(AUTH_CONFIG) private readonly config: AuthConfig) {}

  sign(claims: SessionTokenClaims): string {
    return signHmacToken(
      { stakeholderId: claims.stakeholderId, discipline: claims.discipline, authTime: claims.authTime },
      this.config.sessionTokenSecret,
      this.config.sessionTokenMaxAgeSeconds,
    );
  }

  /**
   * Verifies signature and expiry, returning typed claims. Throws `InvalidSessionTokenError` on
   * expiry, tampering, or malformed tokens.
   */
  verify(token: string): SessionTokenClaims {
    let claims: Record<string, unknown>;
    try {
      claims = verifyHmacToken(token, this.config.sessionTokenSecret);
    } catch (error) {
      if (error instanceof InvalidHmacTokenError) {
        throw new InvalidSessionTokenError(error.message);
      }
      throw error;
    }

    if (!isValidClaims(claims)) {
      throw new InvalidSessionTokenError('claims shape is invalid');
    }

    return {
      stakeholderId: claims.stakeholderId,
      discipline: claims.discipline,
      authTime: claims.authTime,
    };
  }
}
