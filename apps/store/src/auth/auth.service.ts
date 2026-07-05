import { BadRequestException, Inject, Injectable, UnauthorizedException } from '@nestjs/common';
import { GITHUB_OAUTH_CLIENT, type GithubOAuthClient } from './github-oauth-client.js';
import { InvalidOAuthStateError, OAuthStateService } from './oauth-state.service.js';
import { SessionTokenService } from './session-token.service.js';
import { StakeholderRepository } from '../persistence/stakeholder.repository.js';

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
  ) {}

  buildLoginRedirectUrl(): string {
    const state = this.oauthStateService.issue();
    return this.githubOAuthClient.buildAuthorizeUrl(state);
  }

  /**
   * Validates `state`, exchanges `code` for a GitHub access token, resolves the caller's GitHub
   * identity, maps it to an existing stakeholder, and mints a session token. Throws
   * `BadRequestException` for malformed/expired state or an unknown GitHub login, per SG0's
   * acceptance criteria ("400/401 if no matching stakeholder").
   */
  async handleCallback(rawCode: unknown, rawState: unknown): Promise<string> {
    const code = parseRequiredString(rawCode, 'code');
    const state = typeof rawState === 'string' ? rawState : undefined;

    try {
      this.oauthStateService.verify(state);
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

    return this.sessionTokenService.sign({
      stakeholderId: stakeholder.id,
      discipline: stakeholder.discipline,
      authTime: nowSeconds(),
    });
  }
}
