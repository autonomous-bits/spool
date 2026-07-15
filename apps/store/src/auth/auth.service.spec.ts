import { createHash } from 'node:crypto';
import { BadRequestException, ForbiddenException, UnauthorizedException } from '@nestjs/common';
import { Test, type TestingModule } from '@nestjs/testing';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { AuthConfig } from './auth-config.js';
import { AUTH_CONFIG } from './auth-config.token.js';
import { AuthService } from './auth.service.js';
import type { GithubOAuthClient } from './github-oauth-client.js';
import { GITHUB_OAUTH_CLIENT } from './github-oauth-client.js';
import type { OAuthStateService } from './oauth-state.service.js';
import {
  InvalidCliRedirectUriError,
  InvalidOAuthStateError,
  OAuthStateService as OAuthStateServiceToken,
} from './oauth-state.service.js';
import type { RefreshTokenService } from './refresh-token.service.js';
import {
  InvalidRefreshTokenError,
  RefreshTokenService as RefreshTokenServiceToken,
} from './refresh-token.service.js';
import type { SessionTokenService } from './session-token.service.js';
import { SessionTokenService as SessionTokenServiceToken } from './session-token.service.js';
import type { PairingCodeRepository } from '../persistence/pairing-code.repository.js';
import { PairingCodeRepository as PairingCodeRepositoryToken } from '../persistence/pairing-code.repository.js';
import type { StakeholderRepository } from '../persistence/stakeholder.repository.js';
import { StakeholderRepository as StakeholderRepositoryToken } from '../persistence/stakeholder.repository.js';
import type { WorkspaceRepository } from '../persistence/workspace.repository.js';
import { WorkspaceRepository as WorkspaceRepositoryToken } from '../persistence/workspace.repository.js';

function buildConfig(overrides: Partial<AuthConfig> = {}): AuthConfig {
  return {
    githubClientId: 'client-id',
    githubClientSecret: 'client-secret',
    githubRedirectUri: 'http://localhost:3000/auth/github/callback',
    githubAuthorizeUrl: 'https://github.com/login/oauth/authorize',
    githubTokenExchangeUrl: 'https://github.com/login/oauth/access_token',
    githubUserApiUrl: 'https://api.github.com/user',
    sessionTokenSecret: 'session-secret',
    sessionTokenMaxAgeSeconds: 900,
    refreshTokenMaxAgeSeconds: 2_592_000,
    pairingCodeMaxAgeSeconds: 120,
    oauthStateSecret: 'state-secret',
    oauthStateMaxAgeSeconds: 600,
    ...overrides,
  } satisfies AuthConfig;
}

describe('AuthService', () => {
  let githubOAuthClient: Pick<
    GithubOAuthClient,
    'buildAuthorizeUrl' | 'exchangeCodeForAccessToken' | 'fetchGithubUser'
  >;
  let oauthStateService: Pick<OAuthStateService, 'issue' | 'verify'>;
  let sessionTokenService: Pick<SessionTokenService, 'sign'>;
  let refreshTokenService: Pick<RefreshTokenService, 'issue' | 'verifyAndRotate'>;
  let pairingCodeRepository: Pick<PairingCodeRepository, 'create' | 'consume'>;
  let stakeholderRepository: Pick<StakeholderRepository, 'findByGithubLogin' | 'findById'>;
  let workspaceRepository: Pick<WorkspaceRepository, 'isMember' | 'hasAnyMembership'>;
  let service: AuthService;

  let buildAuthorizeUrl: ReturnType<typeof vi.fn>;
  let exchangeCodeForAccessToken: ReturnType<typeof vi.fn>;
  let fetchGithubUser: ReturnType<typeof vi.fn>;

  beforeEach(async () => {
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
      verify: vi.fn().mockReturnValue({ workspaceId: null, cliRedirectUri: null }),
    };
    sessionTokenService = {
      sign: vi.fn().mockReturnValue('signed-session-token'),
    };
    refreshTokenService = {
      issue: vi.fn().mockResolvedValue({
        token: 'refresh-token-1',
        expiresAt: 1_756_360_200 + 2_592_000,
      }),
      verifyAndRotate: vi.fn().mockResolvedValue({
        stakeholderId: 'stakeholder-1',
        workspaceId: 'workspace-1',
        newToken: 'refresh-token-2',
        newExpiresAt: 1_756_360_200 + 2_592_000,
      }),
    };
    pairingCodeRepository = {
      create: vi.fn().mockResolvedValue({ id: 'pairing-code-1' }),
      consume: vi.fn(),
    } satisfies Pick<PairingCodeRepository, 'create' | 'consume'>;
    stakeholderRepository = {
      findByGithubLogin: vi.fn().mockResolvedValue({ id: 'stakeholder-1', discipline: 'engineering' }),
      findById: vi.fn().mockResolvedValue({ id: 'stakeholder-1', discipline: 'engineering' }),
    };
    workspaceRepository = {
      isMember: vi.fn().mockResolvedValue(true),
      hasAnyMembership: vi.fn().mockResolvedValue(false),
    };

    const module: TestingModule = await Test.createTestingModule({
      providers: [
        AuthService,
        { provide: AUTH_CONFIG, useValue: buildConfig() },
        { provide: GITHUB_OAUTH_CLIENT, useValue: githubOAuthClient },
        { provide: OAuthStateServiceToken, useValue: oauthStateService },
        { provide: SessionTokenServiceToken, useValue: sessionTokenService },
        { provide: RefreshTokenServiceToken, useValue: refreshTokenService },
        { provide: PairingCodeRepositoryToken, useValue: pairingCodeRepository },
        { provide: StakeholderRepositoryToken, useValue: stakeholderRepository },
        { provide: WorkspaceRepositoryToken, useValue: workspaceRepository },
      ],
    }).compile();

    service = module.get(AuthService);
  });

  it('buildLoginRedirectUrl issues a state and asks the client for the authorize URL', () => {
    const url = service.buildLoginRedirectUrl();

    expect(oauthStateService.issue).toHaveBeenCalledWith(undefined, undefined);
    expect(buildAuthorizeUrl).toHaveBeenCalledWith('signed-state');
    expect(url).toBe('https://github.com/login/oauth/authorize?state=abc');
  });

  it('buildLoginRedirectUrl passes workspaceId and cliRedirectUri through to state issuance', () => {
    service.buildLoginRedirectUrl('workspace-1', 'http://127.0.0.1:4318/callback');

    expect(oauthStateService.issue).toHaveBeenCalledWith(
      'workspace-1',
      'http://127.0.0.1:4318/callback',
    );
  });

  it('buildLoginRedirectUrl translates an invalid cliRedirectUri into BadRequestException', () => {
    vi.mocked(oauthStateService.issue).mockImplementation(() => {
      throw new InvalidCliRedirectUriError('must target localhost or 127.0.0.1');
    });

    expect(() => {
      service.buildLoginRedirectUrl(undefined, 'http://example.com/callback');
    }).toThrow(BadRequestException);
  });

  it('handleCallback mints both tokens for a workspace-less bootstrap login when the stakeholder has zero memberships and omitted workspaceId', async () => {
    const tokenSet = await service.handleCallback('a-code', 'signed-state');

    expect(oauthStateService.verify).toHaveBeenCalledWith('signed-state');
    expect(exchangeCodeForAccessToken).toHaveBeenCalledWith('a-code');
    expect(fetchGithubUser).toHaveBeenCalledWith('gh-access-token');
    expect(stakeholderRepository.findByGithubLogin).toHaveBeenCalledWith('octocat');
    expect(workspaceRepository.hasAnyMembership).toHaveBeenCalledWith('stakeholder-1');
    expect(sessionTokenService.sign).toHaveBeenCalledWith(
      expect.objectContaining({
        stakeholderId: 'stakeholder-1',
        workspaceId: null,
      }),
    );
    expect(refreshTokenService.issue).toHaveBeenCalledWith({
      stakeholderId: 'stakeholder-1',
      workspaceId: null,
    });
    expect(tokenSet).toEqual({
      kind: 'tokens',
      sessionToken: 'signed-session-token',
      refreshToken: 'refresh-token-1',
      expiresAt: expect.any(Number),
    });
  });

  it('handleCallback rejects with BadRequestException when workspaceId is omitted but the stakeholder already has memberships', async () => {
    vi.mocked(workspaceRepository.hasAnyMembership).mockResolvedValue(true);

    await expect(service.handleCallback('a-code', 'signed-state')).rejects.toThrow(BadRequestException);
  });

  it('handleCallback mints both tokens for a workspace-bound login when the stakeholder is a member of the requested workspace', async () => {
    vi.mocked(oauthStateService.verify).mockReturnValue({
      workspaceId: 'workspace-1',
      cliRedirectUri: null,
    });

    const tokenSet = await service.handleCallback('a-code', 'signed-state');

    expect(workspaceRepository.isMember).toHaveBeenCalledWith('workspace-1', 'stakeholder-1');
    expect(sessionTokenService.sign).toHaveBeenCalledWith(
      expect.objectContaining({ stakeholderId: 'stakeholder-1', workspaceId: 'workspace-1' }),
    );
    expect(refreshTokenService.issue).toHaveBeenCalledWith({
      stakeholderId: 'stakeholder-1',
      workspaceId: 'workspace-1',
    });
    expect(tokenSet).toEqual({
      kind: 'tokens',
      sessionToken: 'signed-session-token',
      refreshToken: 'refresh-token-1',
      expiresAt: expect.any(Number),
    });
  });

  it('handleCallback creates a pairing code and returns a redirect when cliRedirectUri is present', async () => {
    vi.useFakeTimers();
    try {
      vi.setSystemTime(new Date('2026-07-11T22:30:00.000Z'));
      vi.mocked(oauthStateService.verify).mockReturnValue({
        workspaceId: 'workspace-1',
        cliRedirectUri: 'http://127.0.0.1:4318/callback?source=cli',
      });

      const result = await service.handleCallback('a-code', 'signed-state');

      expect(result.kind).toBe('redirect');
      if (result.kind !== 'redirect') {
        throw new Error('expected redirect callback result');
      }

      const redirectUrl = new URL(result.redirectUrl);
      const pairingCode = redirectUrl.searchParams.get('code');
      expect(`${redirectUrl.origin}${redirectUrl.pathname}`).toBe('http://127.0.0.1:4318/callback');
      expect(redirectUrl.searchParams.get('source')).toBe('cli');
      expect(typeof pairingCode).toBe('string');
      expect(pairingCode).toHaveLength(43);
      expect(pairingCodeRepository.create).toHaveBeenCalledWith(
        {
          codeHash: createHash('sha256').update(pairingCode ?? '').digest('hex'),
          sessionToken: 'signed-session-token',
          refreshToken: 'refresh-token-1',
          expiresAt: new Date('2026-07-11T22:32:00.000Z'),
        },
      );
    } finally {
      vi.useRealTimers();
    }
  });

  it('handleCallback rejects with ForbiddenException when the stakeholder is not a member of the requested workspace', async () => {
    vi.mocked(oauthStateService.verify).mockReturnValue({
      workspaceId: 'workspace-1',
      cliRedirectUri: null,
    });
    vi.mocked(workspaceRepository.isMember).mockResolvedValue(false);

    await expect(service.handleCallback('a-code', 'signed-state')).rejects.toThrow(ForbiddenException);
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

  it('exchangePairingCode hashes the code and returns the stored tokens', async () => {
    vi.mocked(pairingCodeRepository.consume).mockResolvedValue({
      sessionToken: 'signed-session-token',
      refreshToken: 'refresh-token-1',
    });

    await expect(service.exchangePairingCode('pairing-code-1')).resolves.toEqual({
      sessionToken: 'signed-session-token',
      refreshToken: 'refresh-token-1',
    });

    expect(pairingCodeRepository.consume).toHaveBeenCalledWith(
      createHash('sha256').update('pairing-code-1').digest('hex'),
    );
  });

  it('exchangePairingCode rejects an unknown, expired, or already consumed pairing code', async () => {
    vi.mocked(pairingCodeRepository.consume).mockResolvedValue(undefined);

    await expect(service.exchangePairingCode('missing-code')).rejects.toThrow(BadRequestException);
  });

  it('refreshSession rotates the refresh token and mints a fresh session token', async () => {
    vi.useFakeTimers();
    try {
      const now = new Date('2026-07-11T22:30:00.000Z');
      const authTime = Math.floor(now.getTime() / 1000);
      vi.setSystemTime(now);

      await expect(service.refreshSession('refresh-token-1')).resolves.toEqual({
        sessionToken: 'signed-session-token',
        refreshToken: 'refresh-token-2',
        expiresAt: authTime + 900,
      });

      expect(refreshTokenService.verifyAndRotate).toHaveBeenCalledWith({ token: 'refresh-token-1' });
      expect(stakeholderRepository.findById).toHaveBeenCalledWith('stakeholder-1');
      expect(sessionTokenService.sign).toHaveBeenCalledWith({
        stakeholderId: 'stakeholder-1',
        authTime,
        workspaceId: 'workspace-1',
      });
    } finally {
      vi.useRealTimers();
    }
  });

  it('refreshSession rejects a missing refresh token with BadRequestException', async () => {
    await expect(service.refreshSession(undefined)).rejects.toThrow(BadRequestException);
    expect(refreshTokenService.verifyAndRotate).not.toHaveBeenCalled();
  });

  it('refreshSession rejects a stale already-rotated refresh token with UnauthorizedException', async () => {
    vi.mocked(refreshTokenService.verifyAndRotate).mockRejectedValue(
      new InvalidRefreshTokenError('token revoked'),
    );

    await expect(service.refreshSession('rotated-token')).rejects.toThrow(UnauthorizedException);
  });

  it('refreshSession rejects an expired refresh token with UnauthorizedException', async () => {
    vi.mocked(refreshTokenService.verifyAndRotate).mockRejectedValue(
      new InvalidRefreshTokenError('token expired'),
    );

    await expect(service.refreshSession('expired-token')).rejects.toThrow(UnauthorizedException);
  });

  it('refreshSession rejects a refresh token whose stakeholder no longer exists with UnauthorizedException', async () => {
    vi.mocked(stakeholderRepository.findById).mockResolvedValue(undefined);

    await expect(service.refreshSession('refresh-token-1')).rejects.toThrow(UnauthorizedException);
  });
});
