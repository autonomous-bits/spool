import { BadRequestException, UnauthorizedException } from '@nestjs/common';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { AuthService } from './auth.service.js';
import type { GithubOAuthClient } from './github-oauth-client.js';
import type { OAuthStateService } from './oauth-state.service.js';
import { InvalidOAuthStateError } from './oauth-state.service.js';
import type { SessionTokenService } from './session-token.service.js';
import type { StakeholderRepository } from '../persistence/stakeholder.repository.js';

describe('AuthService', () => {
  let githubOAuthClient: GithubOAuthClient;
  let oauthStateService: Pick<OAuthStateService, 'issue' | 'verify'>;
  let sessionTokenService: Pick<SessionTokenService, 'sign'>;
  let stakeholderRepository: Pick<StakeholderRepository, 'findByGithubLogin'>;
  let service: AuthService;

  // Kept as standalone typed mock references (rather than reading them back off
  // `githubOAuthClient.*`) so assertions below don't trip `unbound-method`: `GithubOAuthClient`
  // declares its members with method-shorthand syntax, and accessing a method-shorthand member
  // without calling it looks like an unbound `this` reference to that rule, even though these
  // are plain `vi.fn()` mocks with no `this` usage.
  let buildAuthorizeUrl: ReturnType<typeof vi.fn>;
  let exchangeCodeForAccessToken: ReturnType<typeof vi.fn>;
  let fetchGithubUser: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    buildAuthorizeUrl = vi.fn().mockReturnValue('https://github.com/login/oauth/authorize?state=abc');
    exchangeCodeForAccessToken = vi.fn().mockResolvedValue('gh-access-token');
    fetchGithubUser = vi.fn().mockResolvedValue({ login: 'octocat' });
    githubOAuthClient = {
      buildAuthorizeUrl,
      exchangeCodeForAccessToken,
      fetchGithubUser,
    };
    oauthStateService = {
      issue: vi.fn().mockReturnValue('signed-state'),
      verify: vi.fn(),
    };
    sessionTokenService = {
      sign: vi.fn().mockReturnValue('signed-session-token'),
    };
    stakeholderRepository = {
      findByGithubLogin: vi.fn().mockResolvedValue({ id: 'stakeholder-1', discipline: 'engineering' }),
    };

    service = new AuthService(
      githubOAuthClient,
      oauthStateService as OAuthStateService,
      sessionTokenService as SessionTokenService,
      stakeholderRepository as StakeholderRepository,
    );
  });

  it('buildLoginRedirectUrl issues a state and asks the client for the authorize URL', () => {
    const url = service.buildLoginRedirectUrl();

    expect(oauthStateService.issue).toHaveBeenCalledOnce();
    expect(buildAuthorizeUrl).toHaveBeenCalledWith('signed-state');
    expect(url).toBe('https://github.com/login/oauth/authorize?state=abc');
  });

  it('handleCallback mints a session token for a known GitHub login', async () => {
    const token = await service.handleCallback('a-code', 'signed-state');

    expect(oauthStateService.verify).toHaveBeenCalledWith('signed-state');
    expect(exchangeCodeForAccessToken).toHaveBeenCalledWith('a-code');
    expect(fetchGithubUser).toHaveBeenCalledWith('gh-access-token');
    expect(stakeholderRepository.findByGithubLogin).toHaveBeenCalledWith('octocat');
    expect(sessionTokenService.sign).toHaveBeenCalledWith(
      expect.objectContaining({ stakeholderId: 'stakeholder-1', discipline: 'engineering' }),
    );
    expect(token).toBe('signed-session-token');
  });

  it('rejects a missing code with BadRequestException', async () => {
    await expect(service.handleCallback(undefined, 'signed-state')).rejects.toThrow(
      BadRequestException,
    );
  });

  it('rejects an invalid/expired state with BadRequestException', async () => {
    vi.mocked(oauthStateService.verify).mockImplementation(() => {
      throw new InvalidOAuthStateError('token expired');
    });

    await expect(service.handleCallback('a-code', 'signed-state')).rejects.toThrow(
      BadRequestException,
    );
  });

  it('rejects an unknown GitHub login with UnauthorizedException', async () => {
    vi.mocked(stakeholderRepository.findByGithubLogin).mockResolvedValue(undefined);

    await expect(service.handleCallback('a-code', 'signed-state')).rejects.toThrow(
      UnauthorizedException,
    );
  });
});
