import type { INestApplication } from '@nestjs/common';
import type { Server } from 'node:http';
import { Test, type TestingModule } from '@nestjs/testing';
import request from 'supertest';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { AppModule } from '../src/app.module.js';
import { setUpTestDatabase, type TestDatabase } from './support/test-database.js';

const WORKSPACE_ID = '00000000-0000-0000-0000-00000000d0fa';
const SOME_UUID = '00000000-0000-0000-0000-00000000dead';

/**
 * G16 SG3 regression coverage: locks in that every route G16 SG2 already migrated onto real
 * session-token claims (Meridian IDEA-139) rejects a request with no `Authorization` header at
 * all with 401, before any body/param validation or business logic runs. See
 * `docs/goals/G16-store-single-tier-auth-foundation/route-auth-matrix.md` for the full per-route
 * tier inventory and the rationale for the two routes deliberately excluded below:
 *
 * - `GET /artifacts/content/:token` is an intentional unauthenticated capability-link exception
 *   (the opaque signed download token is itself the credential) — not covered here.
 * - `POST/GET /branches/:id/verification-signals` have no session-token check yet at all; that
 *   migration is explicitly G18 SG2's open acceptance criterion, not this sub-goal's — not
 *   covered here (would require implementing production code that belongs to a different goal).
 *
 * `POST /workspaces` IS covered: the Meridian IDEA-101 bootstrap exception only waives the
 * `X-Workspace-Id`/membership check, not the bearer-token requirement itself.
 */
describe('Route auth: 401 without a bearer token (containerized Postgres)', () => {
  let app: INestApplication<Server>;
  let database: TestDatabase;

  beforeAll(async () => {
    database = await setUpTestDatabase();

    const moduleRef: TestingModule = await Test.createTestingModule({
      imports: [AppModule],
    }).compile();

    app = moduleRef.createNestApplication();
    await app.init();
  });

  afterAll(async () => {
    await app.close();
    await database.close();
  });

  function server(): Server {
    return app.getHttpServer();
  }

  const routes: { method: 'get' | 'post' | 'delete'; path: string; body?: unknown }[] = [
    // Chunks
    { method: 'get', path: '/chunks' },
    { method: 'post', path: '/chunks', body: {} },
    { method: 'get', path: `/chunks/${SOME_UUID}` },
    { method: 'get', path: `/chunks/${SOME_UUID}/neighbourhood` },
    // Edges
    { method: 'post', path: '/edges', body: {} },
    { method: 'get', path: `/edges/${SOME_UUID}` },
    // Branches
    { method: 'post', path: '/branches', body: {} },
    { method: 'post', path: `/branches/${SOME_UUID}/submit` },
    { method: 'post', path: `/branches/${SOME_UUID}/verify` },
    { method: 'post', path: `/branches/${SOME_UUID}/reject` },
    { method: 'post', path: `/branches/${SOME_UUID}/merge` },
    { method: 'get', path: `/branches/${SOME_UUID}` },
    // Suggestions
    { method: 'post', path: '/suggestions', body: {} },
    { method: 'post', path: `/suggestions/${SOME_UUID}/accept`, body: {} },
    { method: 'post', path: `/suggestions/${SOME_UUID}/reject` },
    { method: 'get', path: '/suggestions' },
    { method: 'get', path: `/suggestions/${SOME_UUID}` },
    // Artifacts (excluding GET /artifacts/content/:token — deliberate capability-link exception)
    { method: 'post', path: '/artifacts', body: {} },
    { method: 'post', path: `/chunks/${SOME_UUID}/artifacts`, body: {} },
    { method: 'delete', path: `/chunks/${SOME_UUID}/artifacts/${SOME_UUID}?branchId=${SOME_UUID}` },
    { method: 'get', path: `/chunks/${SOME_UUID}/artifacts` },
    { method: 'get', path: `/artifacts/${SOME_UUID}/download-token` },
    // Workspaces
    { method: 'post', path: '/workspaces', body: {} },
    { method: 'post', path: `/workspaces/${SOME_UUID}/members`, body: {} },
    // Notifications
    { method: 'get', path: '/notifications' },
    { method: 'post', path: `/notifications/${SOME_UUID}/read` },
    // Delivery subscriptions
    { method: 'post', path: `/workspaces/${SOME_UUID}/delivery-subscriptions`, body: {} },
    { method: 'get', path: `/workspaces/${SOME_UUID}/delivery-subscriptions` },
    { method: 'delete', path: `/workspaces/${SOME_UUID}/delivery-subscriptions/${SOME_UUID}` },
  ];

  it.each(routes)('$method $path returns 401 with no Authorization header', async ({ method, path, body }) => {
    let req = request(server())[method](path).set('X-Workspace-Id', WORKSPACE_ID);
    if (body !== undefined) {
      req = req.send(body);
    }
    const response = await req;
    expect(response.status).toBe(401);
  });
});
