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

export class InvalidCliRedirectUriError extends Error {
  constructor(reason: string) {
    super(`Invalid cliRedirectUri: ${reason}`);
    this.name = 'InvalidCliRedirectUriError';
  }
}

function validateCliRedirectUri(cliRedirectUri: string): string {
  let url: URL;
  try {
    url = new URL(cliRedirectUri);
  } catch {
    throw new InvalidCliRedirectUriError('must be a valid absolute URL');
  }

  if (url.protocol !== 'http:') {
    throw new InvalidCliRedirectUriError('must use the http:// scheme');
  }

  if (url.hostname !== '127.0.0.1' && url.hostname !== 'localhost') {
    throw new InvalidCliRedirectUriError('must target localhost or 127.0.0.1');
  }

  return cliRedirectUri;
}

/**
 * Issues and verifies the GitHub OAuth `state` CSRF parameter as a self-contained, HMAC-signed,
 * short-lived token (Meridian IDEA-81: "validate state"). The store has no server-side session
 * store to look `state` up against, so `state` carries its own integrity and expiry instead of
 * being a bare random value checked against stored server state.
 *
 * G11 SG2 (Meridian IDEA-92/IDEA-101): `state` also round-trips the optional `workspaceId` and
 * loopback-only `cliRedirectUri` query params a caller supplied to `GET /auth/github/login`, so
 * the callback can recover them after the GitHub redirect round-trip without a server-side session
 * store.
 */
@Injectable()
export class OAuthStateService {
  constructor(@Inject(AUTH_CONFIG) private readonly config: AuthConfig) {}

  issue(workspaceId?: string, cliRedirectUri?: string): string {
    return signHmacToken(
      {
        workspaceId: workspaceId ?? null,
        cliRedirectUri:
          cliRedirectUri === undefined ? null : validateCliRedirectUri(cliRedirectUri),
      },
      this.config.oauthStateSecret,
      this.config.oauthStateMaxAgeSeconds,
    );
  }

  /**
   * Throws `InvalidOAuthStateError` if `state` is missing, malformed, tampered with, or expired.
   * Returns the claims embedded at `issue()` time (`null` if omitted).
   */
  verify(state: string | undefined): { workspaceId: string | null; cliRedirectUri: string | null } {
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

    const cliRedirectUriClaim = claims.cliRedirectUri;
    if (cliRedirectUriClaim !== null && typeof cliRedirectUriClaim !== 'string') {
      throw new InvalidOAuthStateError('state payload has an invalid cliRedirectUri claim');
    }

    let cliRedirectUri: string | null = cliRedirectUriClaim;
    if (cliRedirectUri !== null) {
      try {
        cliRedirectUri = validateCliRedirectUri(cliRedirectUri);
      } catch (error) {
        if (error instanceof InvalidCliRedirectUriError) {
          throw new InvalidOAuthStateError('state payload has an invalid cliRedirectUri claim');
        }
        throw error;
      }
    }

    return { workspaceId, cliRedirectUri };
  }
}
