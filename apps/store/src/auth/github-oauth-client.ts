/**
 * Injectable boundary between AuthService and GitHub's OAuth endpoints (Meridian IDEA-81),
 * so tests use a deterministic fake and Docker Compose e2e (G04.SG5) can point the real
 * implementation's base URLs at a local stub standing in for github.com, without ever driving
 * a live interactive consent screen.
 */
export interface GithubOAuthClient {
  buildAuthorizeUrl(state: string): string;
  exchangeCodeForAccessToken(code: string): Promise<string>;
  fetchGithubUser(accessToken: string): Promise<{ login: string }>;
}

export const GITHUB_OAUTH_CLIENT = Symbol('GITHUB_OAUTH_CLIENT');
