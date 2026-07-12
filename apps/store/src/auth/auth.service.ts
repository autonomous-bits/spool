import {
  BadRequestException,
  ForbiddenException,
  Inject,
  Injectable,
  UnauthorizedException,
} from '@nestjs/common';
import { createHash, randomBytes } from 'node:crypto';
import { GITHUB_OAUTH_CLIENT, type GithubOAuthClient } from './github-oauth-client.js';
import {
  InvalidCliRedirectUriError,
  InvalidOAuthStateError,
  OAuthStateService,
} from './oauth-state.service.js';
import { AUTH_CONFIG } from './auth-config.token.js';
import type { AuthConfig } from './auth-config.js';
import { InvalidRefreshTokenError, RefreshTokenService } from './refresh-token.service.js';
import { SessionTokenService } from './session-token.service.js';
import { PairingCodeRepository } from '../persistence/pairing-code.repository.js';
import { StakeholderRepository } from '../persistence/stakeholder.repository.js';
import { WorkspaceRepository } from '../persistence/workspace.repository.js';

function parseRequiredString(value: unknown, fieldName: string): string {
  if (typeof value !== 'string' || value.trim().length === 0) {
    throw new BadRequestException(`Missing or invalid ${fieldName}`);
  }
  return value;
}

function nowSeconds(): number {
  return Math.floor(Date.now() / 1000);
}

export interface AuthCallbackResult {
  kind: 'tokens';
  sessionToken: string;
  refreshToken: string;
  expiresAt: number;
}

export interface AuthCallbackRedirectResult {
  kind: 'redirect';
  redirectUrl: string;
}

export interface PairingExchangeResult {
  sessionToken: string;
  refreshToken: string;
}

export interface RefreshedSessionResult {
  sessionToken: string;
  refreshToken: string;
  expiresAt: number;
}

export type CallbackResult = AuthCallbackResult | AuthCallbackRedirectResult;

/**
 * Orchestrates the GitHub OAuth login/callback flow (Meridian IDEA-81), sitting between the
 * HTTP controller and the OAuth state/session-token/stakeholder-lookup collaborators.
 */
@Injectable()
export class AuthService {
  constructor(
    @Inject(AUTH_CONFIG) private readonly config: AuthConfig,
    @Inject(GITHUB_OAUTH_CLIENT) private readonly githubOAuthClient: GithubOAuthClient,
    private readonly oauthStateService: OAuthStateService,
    private readonly sessionTokenService: SessionTokenService,
    private readonly refreshTokenService: RefreshTokenService,
    private readonly pairingCodeRepository: PairingCodeRepository,
    private readonly stakeholderRepository: StakeholderRepository,
    private readonly workspaceRepository: WorkspaceRepository,
  ) {}

  /**
   * `workspaceId` is optional (Meridian IDEA-101's bootstrap path): a stakeholder with zero
   * workspace memberships may omit it entirely and later mint a workspace-less bootstrap token.
   */
  buildLoginRedirectUrl(workspaceId?: string, cliRedirectUri?: string): string {
    let state: string;
    try {
      state = this.oauthStateService.issue(workspaceId, cliRedirectUri);
    } catch (error) {
      if (error instanceof InvalidCliRedirectUriError) {
        throw new BadRequestException(error.message);
      }
      throw error;
    }

    return this.githubOAuthClient.buildAuthorizeUrl(state);
  }

  /**
   * Validates `state`, exchanges `code` for a GitHub access token, resolves the caller's GitHub
   * identity, maps it to an existing stakeholder, and mints a session/refresh token pair. Throws
   * `BadRequestException` for malformed/expired state or an unknown GitHub login, per SG0's
   * acceptance criteria ("400/401 if no matching stakeholder").
   *
   * G11 SG2 (Meridian IDEA-92/IDEA-100/IDEA-101): the `workspaceId` round-tripped through `state`
   * governs which kind of token gets minted:
   *   - `workspaceId` present: the stakeholder must be a member of that workspace
   *     (`WorkspaceRepository.isMember`), or the callback is rejected with `ForbiddenException`.
   *     Otherwise a workspace-bound token is minted.
   *   - `workspaceId` absent: only a stakeholder with zero memberships may proceed — they get a
   *     workspace-less bootstrap token (`workspaceId: null`), usable only for `POST /workspaces`.
   *     A stakeholder who already has memberships and omits `workspaceId` is rejected with
   *     `BadRequestException` (400), forcing them to pick one of their existing workspaces.
   *
   * When `cliRedirectUri` is present in the signed state, the token pair is persisted behind a
   * short-lived, single-use pairing code and the caller receives a loopback redirect instead of the
   * raw tokens directly.
   */
  async handleCallback(rawCode: unknown, rawState: unknown): Promise<CallbackResult> {
    const code = parseRequiredString(rawCode, 'code');
    const state = typeof rawState === 'string' ? rawState : undefined;

    let workspaceId: string | null;
    let cliRedirectUri: string | null;
    try {
      ({ workspaceId, cliRedirectUri } = this.oauthStateService.verify(state));
    } catch (error) {
      if (error instanceof InvalidOAuthStateError) {
        throw new BadRequestException(error.message);
      }
      throw error;
    }

    const accessToken = await this.githubOAuthClient.exchangeCodeForAccessToken(code);
    const githubUser = await this.githubOAuthClient.fetchGithubUser(accessToken);

    const stakeholder = await this.stakeholderRepository.findByGithubLogin(githubUser.login);
    if (stakeholder === undefined) {
      throw new UnauthorizedException(`No stakeholder mapped to GitHub login: ${githubUser.login}`);
    }

    if (workspaceId !== null) {
      const isMember = await this.workspaceRepository.isMember(workspaceId, stakeholder.id);
      if (!isMember) {
        throw new ForbiddenException(
          `Stakeholder ${stakeholder.id} is not a member of workspace ${workspaceId}`,
        );
      }
    } else {
      const hasAnyMembership = await this.workspaceRepository.hasAnyMembership(stakeholder.id);
      if (hasAnyMembership) {
        throw new BadRequestException(
          'workspaceId is required for stakeholders who already belong to a workspace',
        );
      }
    }

    const authTime = nowSeconds();
    const sessionToken = this.sessionTokenService.sign({
      stakeholderId: stakeholder.id,
      discipline: stakeholder.discipline,
      authTime,
      workspaceId,
    });
    const refreshToken = await this.refreshTokenService.issue({
      stakeholderId: stakeholder.id,
      workspaceId,
    });
    const callbackTokens = {
      kind: 'tokens',
      sessionToken,
      refreshToken: refreshToken.token,
      expiresAt: this.getSessionTokenExpiresAt(authTime),
    } satisfies AuthCallbackResult;

    if (cliRedirectUri === null) {
      return callbackTokens;
    }

    const pairingCode = randomBytes(32).toString('base64url');
    const pairingCodeExpiresAt = nowSeconds() + this.config.pairingCodeMaxAgeSeconds;
    await this.pairingCodeRepository.create({
      codeHash: this.hashOpaqueToken(pairingCode),
      sessionToken,
      refreshToken: refreshToken.token,
      expiresAt: new Date(pairingCodeExpiresAt * 1000),
    });

    const redirectUrl = new URL(cliRedirectUri);
    redirectUrl.searchParams.set('code', pairingCode);

    return {
      kind: 'redirect',
      redirectUrl: redirectUrl.toString(),
    } satisfies AuthCallbackRedirectResult;
  }

  async exchangePairingCode(code: string): Promise<PairingExchangeResult> {
    const pairingCode = parseRequiredString(code, 'code');
    const tokens = await this.pairingCodeRepository.consume(this.hashOpaqueToken(pairingCode));
    if (tokens === undefined) {
      throw new BadRequestException('Invalid or expired pairing code');
    }

    return tokens;
  }

  async refreshSession(rawRefreshToken: unknown): Promise<RefreshedSessionResult> {
    const refreshToken = parseRequiredString(rawRefreshToken, 'refreshToken');

    try {
      const rotated = await this.refreshTokenService.verifyAndRotate({ token: refreshToken });
      const stakeholder = await this.stakeholderRepository.findById(rotated.stakeholderId);
      if (stakeholder === undefined) {
        throw new InvalidRefreshTokenError('stakeholder not found');
      }

      const authTime = nowSeconds();
      const sessionToken = this.sessionTokenService.sign({
        stakeholderId: rotated.stakeholderId,
        discipline: stakeholder.discipline,
        authTime,
        workspaceId: rotated.workspaceId,
      });

      return {
        sessionToken,
        refreshToken: rotated.newToken,
        expiresAt: this.getSessionTokenExpiresAt(authTime),
      };
    } catch (error) {
      if (error instanceof InvalidRefreshTokenError) {
        throw new UnauthorizedException(error.message);
      }
      throw error;
    }
  }

  private hashOpaqueToken(token: string): string {
    return createHash('sha256').update(token).digest('hex');
  }

  private getSessionTokenExpiresAt(authTime: number): number {
    return authTime + this.config.sessionTokenMaxAgeSeconds;
  }
}
