import { Inject, Injectable } from '@nestjs/common';
import { AUTH_CONFIG } from './auth-config.token.js';
import type { AuthConfig } from './auth-config.js';
import type { GithubOAuthClient } from './github-oauth-client.js';

export class GithubOAuthError extends Error {
  constructor(reason: string) {
    super(`GitHub OAuth request failed: ${reason}`);
    this.name = 'GithubOAuthError';
  }
}

function parseAccessTokenResponse(body: unknown): string {
  if (typeof body !== 'object' || body === null) {
    throw new GithubOAuthError('token exchange response was not an object');
  }
  const accessToken = (body as Record<string, unknown>)['access_token'];
  if (typeof accessToken !== 'string' || accessToken.length === 0) {
    throw new GithubOAuthError('token exchange response missing access_token');
  }
  return accessToken;
}

function parseGithubUserResponse(body: unknown): { login: string } {
  if (typeof body !== 'object' || body === null) {
    throw new GithubOAuthError('user response was not an object');
  }
  const login = (body as Record<string, unknown>)['login'];
  if (typeof login !== 'string' || login.length === 0) {
    throw new GithubOAuthError('user response missing login');
  }
  return { login };
}

/**
 * Real `GithubOAuthClient` implementation, talking to github.com over `fetch` (Meridian
 * IDEA-81). Base URLs come from `AuthConfig` rather than being hardcoded, so Docker Compose can
 * redirect them at a local stub for containerized e2e without swapping this class out.
 */
@Injectable()
export class HttpGithubOAuthClient implements GithubOAuthClient {
  constructor(@Inject(AUTH_CONFIG) private readonly config: AuthConfig) {}

  buildAuthorizeUrl(state: string): string {
    const url = new URL(this.config.githubAuthorizeUrl);
    url.searchParams.set('client_id', this.config.githubClientId);
    url.searchParams.set('redirect_uri', this.config.githubRedirectUri);
    url.searchParams.set('state', state);
    return url.toString();
  }

  async exchangeCodeForAccessToken(code: string): Promise<string> {
    const response = await fetch(this.config.githubTokenExchangeUrl, {
      method: 'POST',
      headers: {
        Accept: 'application/json',
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({
        client_id: this.config.githubClientId,
        client_secret: this.config.githubClientSecret,
        code,
        redirect_uri: this.config.githubRedirectUri,
      }),
    });

    if (!response.ok) {
      throw new GithubOAuthError(`token exchange returned HTTP ${response.status}`);
    }

    const body: unknown = await response.json();
    return parseAccessTokenResponse(body);
  }

  async fetchGithubUser(accessToken: string): Promise<{ login: string }> {
    const response = await fetch(this.config.githubUserApiUrl, {
      headers: {
        Accept: 'application/vnd.github+json',
        Authorization: `Bearer ${accessToken}`,
        'User-Agent': 'spool-store',
      },
    });

    if (!response.ok) {
      throw new GithubOAuthError(`user lookup returned HTTP ${response.status}`);
    }

    const body: unknown = await response.json();
    return parseGithubUserResponse(body);
  }
}
