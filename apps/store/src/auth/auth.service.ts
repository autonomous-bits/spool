import {
  BadRequestException,
  ForbiddenException,
  Inject,
  Injectable,
  UnauthorizedException,
} from '@nestjs/common';
import { GITHUB_OAUTH_CLIENT, type GithubOAuthClient } from './github-oauth-client.js';
import { InvalidOAuthStateError, OAuthStateService } from './oauth-state.service.js';
import { SessionTokenService } from './session-token.service.js';
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

/**
 * Orchestrates the GitHub OAuth login/callback flow (Meridian IDEA-81), sitting between the
 * HTTP controller and the OAuth state/session-token/stakeholder-lookup collaborators.
 */
@Injectable()
export class AuthService {
  constructor(
    @Inject(GITHUB_OAUTH_CLIENT) private readonly githubOAuthClient: GithubOAuthClient,
    private readonly oauthStateService: OAuthStateService,
    private readonly sessionTokenService: SessionTokenService,
    private readonly stakeholderRepository: StakeholderRepository,
    private readonly workspaceRepository: WorkspaceRepository,
  ) {}

  /**
   * `workspaceId` is optional (Meridian IDEA-101's bootstrap path): a stakeholder with zero
   * workspace memberships may omit it entirely and later mint a workspace-less bootstrap token.
   */
  buildLoginRedirectUrl(workspaceId?: string): string {
    const state = this.oauthStateService.issue(workspaceId);
    return this.githubOAuthClient.buildAuthorizeUrl(state);
  }

  /**
   * Validates `state`, exchanges `code` for a GitHub access token, resolves the caller's GitHub
   * identity, maps it to an existing stakeholder, and mints a session token. Throws
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
   */
  async handleCallback(rawCode: unknown, rawState: unknown): Promise<string> {
    const code = parseRequiredString(rawCode, 'code');
    const state = typeof rawState === 'string' ? rawState : undefined;

    let workspaceId: string | null;
    try {
      ({ workspaceId } = this.oauthStateService.verify(state));
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

    return this.sessionTokenService.sign({
      stakeholderId: stakeholder.id,
      discipline: stakeholder.discipline,
      authTime: nowSeconds(),
      workspaceId,
    });
  }
}
