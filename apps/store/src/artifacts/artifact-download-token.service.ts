import { Inject, Injectable } from '@nestjs/common';
import { InvalidHmacTokenError, signHmacToken, verifyHmacToken } from '../auth/hmac-token.js';
import { ARTIFACT_DOWNLOAD_CONFIG } from './artifact-download-config.token.js';
import type { ArtifactDownloadConfig } from './artifact-download-config.js';

export class InvalidArtifactDownloadTokenError extends Error {
  constructor(reason: string) {
    super(`Invalid artifact download token: ${reason}`);
    this.name = 'InvalidArtifactDownloadTokenError';
  }
}

/** Claims carried by a signed artifact-download token: the artifact and workspace it authorizes. */
export interface ArtifactDownloadTokenClaims {
  artifactId: string;
  workspaceId: string;
}

export interface IssuedArtifactDownloadToken {
  token: string;
  expiresAt: Date;
}

function isValidClaims(
  claims: Record<string, unknown>,
): claims is ArtifactDownloadTokenClaims & Record<string, unknown> {
  return typeof claims.artifactId === 'string' && typeof claims.workspaceId === 'string';
}

/**
 * Mints and verifies short-lived, store-issued artifact-download tokens (Meridian IDEA-85's
 * resolution of the IDEA-84 gap report on IDEA-61's "signed URLs" for Docker-volume-backed
 * storage). Reuses the dependency-free HMAC codec in `../auth/hmac-token.js` — the same pattern
 * `SessionTokenService`/`OAuthStateService` use — but with its own secret and claim shape,
 * because this token authorizes downloading one specific blob rather than asserting stakeholder
 * identity. Carries `workspaceId` so IDEA-139's one deliberate exception,
 * `GET /artifacts/content/:token`, can redeem the token without a bearer token or
 * `X-Workspace-Id` header: the capability token is minted from an already-authenticated,
 * already-scoped lookup at issuance time.
 */
@Injectable()
export class ArtifactDownloadTokenService {
  constructor(
    @Inject(ARTIFACT_DOWNLOAD_CONFIG) private readonly config: ArtifactDownloadConfig,
  ) {}

  issue(artifactId: string, workspaceId: string): IssuedArtifactDownloadToken {
    const token = signHmacToken(
      { artifactId, workspaceId },
      this.config.secret,
      this.config.maxAgeSeconds,
    );
    const expiresAt = new Date(Date.now() + this.config.maxAgeSeconds * 1000);
    return { token, expiresAt };
  }

  /**
   * Verifies signature and expiry, returning typed claims. Throws
   * `InvalidArtifactDownloadTokenError` on expiry, tampering, or malformed tokens.
   */
  verify(token: string): ArtifactDownloadTokenClaims {
    let claims: Record<string, unknown>;
    try {
      claims = verifyHmacToken(token, this.config.secret);
    } catch (error) {
      if (error instanceof InvalidHmacTokenError) {
        throw new InvalidArtifactDownloadTokenError(error.message);
      }
      throw error;
    }

    if (!isValidClaims(claims)) {
      throw new InvalidArtifactDownloadTokenError('claims shape is invalid');
    }

    return { artifactId: claims.artifactId, workspaceId: claims.workspaceId };
  }
}
