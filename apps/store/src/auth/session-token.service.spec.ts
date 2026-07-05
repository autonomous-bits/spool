import { describe, expect, it, vi } from 'vitest';
import type { AuthConfig } from './auth-config.js';
import { InvalidSessionTokenError, SessionTokenService } from './session-token.service.js';

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
    oauthStateSecret: 'state-secret',
    oauthStateMaxAgeSeconds: 600,
    ...overrides,
  } satisfies AuthConfig;
}

describe('SessionTokenService', () => {
  it('mints a token whose verify() returns the signed claims', () => {
    const service = new SessionTokenService(buildConfig());
    const token = service.sign({ stakeholderId: 'stakeholder-1', discipline: 'engineering', authTime: 100 });

    const claims = service.verify(token);

    expect(claims).toEqual({ stakeholderId: 'stakeholder-1', discipline: 'engineering', authTime: 100 });
  });

  it('mints a verifiable token for a null-discipline stakeholder', () => {
    const service = new SessionTokenService(buildConfig());
    const token = service.sign({ stakeholderId: 'stakeholder-1', discipline: null, authTime: 100 });

    expect(service.verify(token)).toEqual({
      stakeholderId: 'stakeholder-1',
      discipline: null,
      authTime: 100,
    });
  });

  it('throws InvalidSessionTokenError for a token signed with a different secret', () => {
    const service = new SessionTokenService(buildConfig());
    const otherService = new SessionTokenService(buildConfig({ sessionTokenSecret: 'other-secret' }));
    const token = otherService.sign({ stakeholderId: 'stakeholder-1', discipline: null, authTime: 100 });

    expect(() => service.verify(token)).toThrow(InvalidSessionTokenError);
  });

  it('throws InvalidSessionTokenError once the token has expired', () => {
    vi.useFakeTimers();
    try {
      vi.setSystemTime(0);
      const service = new SessionTokenService(buildConfig({ sessionTokenMaxAgeSeconds: 60 }));
      const token = service.sign({ stakeholderId: 'stakeholder-1', discipline: null, authTime: 0 });

      vi.setSystemTime(61_000);

      expect(() => service.verify(token)).toThrow(InvalidSessionTokenError);
    } finally {
      vi.useRealTimers();
    }
  });

  it('throws InvalidSessionTokenError for a structurally invalid token', () => {
    const service = new SessionTokenService(buildConfig());
    expect(() => service.verify('garbage')).toThrow(InvalidSessionTokenError);
  });
});
