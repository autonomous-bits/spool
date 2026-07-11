import { UnauthorizedException } from '@nestjs/common';
import type { SessionTokenService} from './session-token.service.js';
import { InvalidSessionTokenError, type SessionTokenClaims } from './session-token.service.js';

/**
 * Extracts and verifies a bearer session token from an `Authorization` header, shared by every
 * human-only route (branches submit/verify/reject/merge, suggestions accept/reject, G09 SG3
 * notifications). Previously duplicated verbatim in `BranchesController` and
 * `SuggestionsController`; centralized here so a third copy isn't added for
 * `NotificationsController`.
 */
export function extractBearerToken(authorizationHeader: unknown): string {
  if (typeof authorizationHeader !== 'string') {
    throw new UnauthorizedException('Missing Authorization header');
  }

  const [scheme, token, ...rest] = authorizationHeader.split(' ');
  if (scheme !== 'Bearer' || token === undefined || token.trim().length === 0 || rest.length > 0) {
    throw new UnauthorizedException('Authorization header must use ****** format');
  }

  return token;
}

export function verifySessionClaims(
  authorizationHeader: unknown,
  sessionTokenService: SessionTokenService,
): SessionTokenClaims {
  const token = extractBearerToken(authorizationHeader);

  try {
    return sessionTokenService.verify(token);
  } catch (error) {
    if (error instanceof InvalidSessionTokenError) {
      throw new UnauthorizedException(error.message);
    }
    throw error;
  }
}
