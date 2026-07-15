import { randomUUID } from 'node:crypto';
import type { INestApplication } from '@nestjs/common';
import type { Server } from 'node:http';
import { Test, type TestingModule } from '@nestjs/testing';
import request from 'supertest';
import { afterAll, beforeAll, describe, expect, it, vi } from 'vitest';
import { AppModule } from '../src/app.module.js';
import { GITHUB_OAUTH_CLIENT, type GithubOAuthClient } from '../src/auth/github-oauth-client.js';
import { SessionTokenService } from '../src/auth/session-token.service.js';
import { Workspace } from '../src/domain/workspace.js';
import { WorkspaceRepository } from '../src/persistence/workspace.repository.js';
import { setUpTestDatabase, type TestDatabase } from './support/test-database.js';

const KNOWN_GITHUB_LOGIN = `known-octocat-${randomUUID()}`;
const UNKNOWN_GITHUB_LOGIN = 'unmapped-octocat';
const MEMBER_GITHUB_LOGIN = `member-octocat-${randomUUID()}`;

/** Asserts a supertest response's `location` header is present and returns it as a string. */
function requireLocationHeader(headers: Record<string, unknown>): string {
  const location = headers.location;
  if (typeof location !== 'string') {
    throw new Error('expected a location header');
  }
  return location;
}

const BEARER_PREFIX = 'Bearer ';

function authHeader(token: string): string {
  return BEARER_PREFIX.concat(token);
}

/**
 * Deterministic fake standing in for github.com's token-exchange/user endpoints, per SG0's
 * "injectable GithubOAuthClient" requirement. Maps a single fixed authorization code to a fixed
 * GitHub login so this e2e test never makes a real network call.
 */
class FakeGithubOAuthClient implements GithubOAuthClient {
  buildAuthorizeUrl(state: string): string {
    return `https://github.com/login/oauth/authorize?client_id=fake-client-id&redirect_uri=http%3A%2F%2Flocalhost%3A3000%2Fauth%2Fgithub%2Fcallback&state=${state}`;
  }

  exchangeCodeForAccessToken(code: string): Promise<string> {
    if (code !== 'valid-code') {
      throw new Error('unexpected code in FakeGithubOAuthClient');
    }
    return Promise.resolve('fake-access-token');
  }

  fetchGithubUser(accessToken: string): Promise<{ login: string }> {
    if (accessToken !== 'fake-access-token') {
      throw new Error('unexpected access token in FakeGithubOAuthClient');
    }
    return Promise.resolve({ login: KNOWN_GITHUB_LOGIN });
 }
}

describe('GitHub OAuth login/callback HTTP API (containerized Postgres)', () => {
  let app: INestApplication<Server>;
  let database: TestDatabase;
  let sessionTokenService: SessionTokenService;
  let stakeholderId: string;
  let memberStakeholderId: string;
  let memberWorkspaceId: string;

  beforeAll(async () => {
    database = await setUpTestDatabase();

    stakeholderId = randomUUID();
    await database.pool.query(
      `INSERT INTO stakeholders (id, name, email, role, discipline, github_login)
       VALUES ($1, 'E2E Stakeholder', $2, 'stakeholder', 'engineering', $3)`,
      [stakeholderId, `e2e-auth-${stakeholderId}@spool.local`, KNOWN_GITHUB_LOGIN],
    );

    memberStakeholderId = randomUUID();
    await database.pool.query(
      `INSERT INTO stakeholders (id, name, email, role, discipline, github_login)
       VALUES ($1, 'E2E Member Stakeholder', $2, 'stakeholder', 'engineering', $3)`,
      [memberStakeholderId, `e2e-auth-member-${memberStakeholderId}@spool.local`, MEMBER_GITHUB_LOGIN],
    );
    const workspaceRepository = new WorkspaceRepository(database.pool);
    const memberWorkspace = await workspaceRepository.createWithFirstMember(
      new Workspace({ name: `e2e-auth-workspace-${randomUUID()}`, createdByStakeholderId: memberStakeholderId }),
    );
    memberWorkspaceId = memberWorkspace.id;

    const moduleRef: TestingModule = await Test.createTestingModule({
      imports: [AppModule],
    })
      .overrideProvider(GITHUB_OAUTH_CLIENT)
      .useValue(new FakeGithubOAuthClient())
      .compile();

    app = moduleRef.createNestApplication();
    await app.init();
    sessionTokenService = moduleRef.get(SessionTokenService);
  });

  afterAll(async () => {
    await app.close();
    await database.close();
  });

  it('GET /auth/github/login redirects to github.com/login/oauth/authorize with client_id/redirect_uri/state', async () => {
    const response = await request(app.getHttpServer()).get('/auth/github/login');

    expect(response.status).toBe(302);
    const location = requireLocationHeader(response.headers);
    expect(location).toContain('github.com/login/oauth/authorize');
    expect(location).toContain('client_id=');
    expect(location).toContain('redirect_uri=');
    expect(location).toContain('state=');
  });

  it('GET /auth/github/login returns 400 for a non-loopback cliRedirectUri', async () => {
    const response = await request(app.getHttpServer())
      .get('/auth/github/login')
      .query({ cliRedirectUri: 'http://evil.example.com/callback' });

    expect(response.status).toBe(400);
  });

  it('GET /auth/github/login returns 400 for an https loopback cliRedirectUri', async () => {
    const response = await request(app.getHttpServer())
      .get('/auth/github/login')
      .query({ cliRedirectUri: 'https://127.0.0.1/callback' });

    expect(response.status).toBe(400);
  });

  it('GET /auth/github/callback mints a verifiable session token for a known GitHub login', async () => {
    const loginResponse = await request(app.getHttpServer()).get('/auth/github/login');
    const location = new URL(requireLocationHeader(loginResponse.headers));
    const state = location.searchParams.get('state');

    const callbackResponse = await request(app.getHttpServer())
      .get('/auth/github/callback')
      .query({ code: 'valid-code', state });

    expect(callbackResponse.status).toBe(200);
    expect(typeof callbackResponse.body.sessionToken).toBe('string');
    expect(typeof callbackResponse.body.refreshToken).toBe('string');
    expect(typeof callbackResponse.body.expiresAt).toBe('number');

    const claims = sessionTokenService.verify(callbackResponse.body.sessionToken as string);
    expect(claims.stakeholderId).toBe(stakeholderId);
  });

  it('GET /auth/github/callback returns 401 for an unknown GitHub login', async () => {
    const loginResponse = await request(app.getHttpServer()).get('/auth/github/login');
    const location = new URL(requireLocationHeader(loginResponse.headers));
    const state = location.searchParams.get('state');

    class UnknownLoginClient extends FakeGithubOAuthClient {
      override fetchGithubUser(): Promise<{ login: string }> {
        return Promise.resolve({ login: UNKNOWN_GITHUB_LOGIN });
      }
    }

    const moduleRef: TestingModule = await Test.createTestingModule({
      imports: [AppModule],
    })
      .overrideProvider(GITHUB_OAUTH_CLIENT)
      .useValue(new UnknownLoginClient())
      .compile();
    const unknownApp = moduleRef.createNestApplication();
    await unknownApp.init();

    try {
      const response = await request(unknownApp.getHttpServer())
        .get('/auth/github/callback')
        .query({ code: 'valid-code', state });

      expect(response.status).toBe(401);
    } finally {
      await unknownApp.close();
    }
  });

  it('GET /auth/github/callback returns 400 for a missing state', async () => {
    const response = await request(app.getHttpServer())
      .get('/auth/github/callback')
      .query({ code: 'valid-code' });

    expect(response.status).toBe(400);
  });

  it('GET /auth/github/callback returns 400 for an invalid state', async () => {
    const response = await request(app.getHttpServer())
      .get('/auth/github/callback')
      .query({ code: 'valid-code', state: 'not-a-real-state' });

    expect(response.status).toBe(400);
  });

  describe('G11 SG2: workspaceId-aware login/callback', () => {
    class MemberLoginClient extends FakeGithubOAuthClient {
      override fetchGithubUser(): Promise<{ login: string }> {
        return Promise.resolve({ login: MEMBER_GITHUB_LOGIN });
      }
    }

    async function buildMemberApp(): Promise<INestApplication<Server>> {
      const moduleRef: TestingModule = await Test.createTestingModule({
        imports: [AppModule],
      })
        .overrideProvider(GITHUB_OAUTH_CLIENT)
        .useValue(new MemberLoginClient())
        .compile();
      const memberApp = moduleRef.createNestApplication();
      await memberApp.init();
      return memberApp;
    }

    it('mints a workspace-bound token when the stakeholder is a member of the requested workspaceId', async () => {
      const memberApp = await buildMemberApp();
      try {
        const loginResponse = await request(memberApp.getHttpServer())
          .get('/auth/github/login')
          .query({ workspaceId: memberWorkspaceId });
        const location = new URL(requireLocationHeader(loginResponse.headers));
        const state = location.searchParams.get('state');

        const callbackResponse = await request(memberApp.getHttpServer())
          .get('/auth/github/callback')
          .query({ code: 'valid-code', state });

        expect(callbackResponse.status).toBe(200);
        expect(typeof callbackResponse.body.refreshToken).toBe('string');
        expect(typeof callbackResponse.body.expiresAt).toBe('number');
        const claims = sessionTokenService.verify(callbackResponse.body.sessionToken as string);
        expect(claims.stakeholderId).toBe(memberStakeholderId);
        expect(claims.workspaceId).toBe(memberWorkspaceId);
      } finally {
        await memberApp.close();
      }
    });

    it('returns 403 when the stakeholder is not a member of the requested workspaceId', async () => {
      const memberApp = await buildMemberApp();
      try {
        const loginResponse = await request(memberApp.getHttpServer())
          .get('/auth/github/login')
          .query({ workspaceId: '00000000-0000-0000-0000-00000000dead' });
        const location = new URL(requireLocationHeader(loginResponse.headers));
        const state = location.searchParams.get('state');

        const callbackResponse = await request(memberApp.getHttpServer())
          .get('/auth/github/callback')
          .query({ code: 'valid-code', state });

        expect(callbackResponse.status).toBe(403);
      } finally {
        await memberApp.close();
      }
    });

    it('returns 400 when workspaceId is omitted for a stakeholder who already has memberships', async () => {
      const memberApp = await buildMemberApp();
      try {
        const loginResponse = await request(memberApp.getHttpServer()).get('/auth/github/login');
        const location = new URL(requireLocationHeader(loginResponse.headers));
        const state = location.searchParams.get('state');

        const callbackResponse = await request(memberApp.getHttpServer())
          .get('/auth/github/callback')
          .query({ code: 'valid-code', state });

        expect(callbackResponse.status).toBe(400);
      } finally {
        await memberApp.close();
      }
    });

    it('mints a workspace-less bootstrap token when workspaceId is omitted for a stakeholder with zero memberships', async () => {
      const loginResponse = await request(app.getHttpServer()).get('/auth/github/login');
      const location = new URL(requireLocationHeader(loginResponse.headers));
      const state = location.searchParams.get('state');

      const callbackResponse = await request(app.getHttpServer())
        .get('/auth/github/callback')
        .query({ code: 'valid-code', state });

      expect(callbackResponse.status).toBe(200);
      expect(typeof callbackResponse.body.refreshToken).toBe('string');
      expect(typeof callbackResponse.body.expiresAt).toBe('number');
      const claims = sessionTokenService.verify(callbackResponse.body.sessionToken as string);
      expect(claims.stakeholderId).toBe(stakeholderId);
      expect(claims.workspaceId).toBeNull();
    });

    it('redirects loopback logins to cliRedirectUri, exchanges the pairing code, and refreshes the returned token pair', async () => {
      vi.useFakeTimers();
      try {
        vi.setSystemTime(new Date(Date.now() + 5 * 60_000));

        const cliRedirectUri = 'http://127.0.0.1:4318/callback?source=cli';
        const cliLoginResponse = await request(app.getHttpServer())
          .get('/auth/github/login')
          .query({ cliRedirectUri });
        const cliState = new URL(requireLocationHeader(cliLoginResponse.headers)).searchParams.get('state');

        const cliCallbackResponse = await request(app.getHttpServer())
          .get('/auth/github/callback')
          .query({ code: 'valid-code', state: cliState });

        expect(cliCallbackResponse.status).toBe(302);
        const redirectLocation = new URL(requireLocationHeader(cliCallbackResponse.headers));
        expect(`${redirectLocation.origin}${redirectLocation.pathname}`).toBe(
          'http://127.0.0.1:4318/callback',
        );
        expect(redirectLocation.searchParams.get('source')).toBe('cli');

        const pairingCode = redirectLocation.searchParams.get('code');
        expect(typeof pairingCode).toBe('string');
        expect(pairingCode).not.toBeNull();

        const exchangeResponse = await request(app.getHttpServer())
          .post('/auth/github/pairing/exchange')
          .send({ code: pairingCode });

        expect(exchangeResponse.status).toBe(200);
        expect(typeof exchangeResponse.body.sessionToken).toBe('string');
        expect(typeof exchangeResponse.body.refreshToken).toBe('string');

        const exchangedClaims = sessionTokenService.verify(exchangeResponse.body.sessionToken as string);
        expect(exchangedClaims.stakeholderId).toBe(stakeholderId);
        expect(exchangedClaims.workspaceId).toBeNull();

        const workspaceResponse = await request(app.getHttpServer())
          .post('/workspaces')
          .set('Authorization', authHeader(exchangeResponse.body.sessionToken as string))
          .send({ name: `paired-auth-workspace-${randomUUID()}` });
        expect(workspaceResponse.status).toBe(201);

        const refreshResponse = await request(app.getHttpServer())
          .post('/auth/github/refresh')
          .send({ refreshToken: exchangeResponse.body.refreshToken as string });
        expect(refreshResponse.status).toBe(200);
        expect(typeof refreshResponse.body.sessionToken).toBe('string');
        expect(typeof refreshResponse.body.refreshToken).toBe('string');
        expect(refreshResponse.body.refreshToken).not.toBe(exchangeResponse.body.refreshToken);

        const refreshedClaims = sessionTokenService.verify(refreshResponse.body.sessionToken as string);
        expect(refreshedClaims.stakeholderId).toBe(stakeholderId);
        expect(refreshedClaims.workspaceId).toBeNull();

        const refreshedWorkspaceResponse = await request(app.getHttpServer())
          .post('/workspaces')
          .set('Authorization', authHeader(refreshResponse.body.sessionToken as string))
          .send({ name: `paired-auth-refreshed-workspace-${randomUUID()}` });
        expect(refreshedWorkspaceResponse.status).toBe(201);

        const secondExchangeResponse = await request(app.getHttpServer())
          .post('/auth/github/pairing/exchange')
          .send({ code: pairingCode });
        expect(secondExchangeResponse.status).toBe(400);

        const unknownExchangeResponse = await request(app.getHttpServer())
          .post('/auth/github/pairing/exchange')
          .send({ code: 'not-a-real-code' });
        expect(unknownExchangeResponse.status).toBe(400);
      } finally {
        vi.useRealTimers();
      }
    });

    it('rotates refresh tokens and rejects reuse of the original token', async () => {
      const memberApp = await buildMemberApp();
      try {
        const loginResponse = await request(memberApp.getHttpServer())
          .get('/auth/github/login')
          .query({ workspaceId: memberWorkspaceId });
        const state = new URL(requireLocationHeader(loginResponse.headers)).searchParams.get('state');

        const callbackResponse = await request(memberApp.getHttpServer())
          .get('/auth/github/callback')
          .query({ code: 'valid-code', state });

        expect(callbackResponse.status).toBe(200);
        expect(typeof callbackResponse.body.sessionToken).toBe('string');
        expect(typeof callbackResponse.body.refreshToken).toBe('string');
        expect(typeof callbackResponse.body.expiresAt).toBe('number');

        const firstRefreshToken = callbackResponse.body.refreshToken as string;
        const refreshResponse = await request(memberApp.getHttpServer())
          .post('/auth/github/refresh')
          .send({ refreshToken: firstRefreshToken });

        expect(refreshResponse.status).toBe(200);
        expect(typeof refreshResponse.body.sessionToken).toBe('string');
        expect(typeof refreshResponse.body.refreshToken).toBe('string');
        expect(typeof refreshResponse.body.expiresAt).toBe('number');
        expect(refreshResponse.body.refreshToken).not.toBe(firstRefreshToken);

        const refreshedClaims = sessionTokenService.verify(refreshResponse.body.sessionToken as string);
        expect(refreshedClaims.stakeholderId).toBe(memberStakeholderId);
        expect(refreshedClaims.workspaceId).toBe(memberWorkspaceId);

        const workspaceResponse = await request(memberApp.getHttpServer())
          .post('/workspaces')
          .set('Authorization', `Bearer ${refreshResponse.body.sessionToken as string}`)
          .send({ name: `refreshed-auth-workspace-${randomUUID()}` });
        expect(workspaceResponse.status).toBe(201);

        const replayResponse = await request(memberApp.getHttpServer())
          .post('/auth/github/refresh')
          .send({ refreshToken: firstRefreshToken });
        expect(replayResponse.status).toBe(401);
      } finally {
        await memberApp.close();
      }
    });
  });
});
