import { randomUUID } from 'node:crypto';
import type { INestApplication } from '@nestjs/common';
import type { Server } from 'node:http';
import { Test, type TestingModule } from '@nestjs/testing';
import request from 'supertest';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { AppModule } from '../src/app.module.js';
import { SessionTokenService } from '../src/auth/session-token.service.js';
import { BOOTSTRAP_STAKEHOLDER_ID } from '../src/persistence/bootstrap-stakeholder.js';
import { setUpTestDatabase, type TestDatabase } from './support/test-database.js';

const WORKSPACE_ID = '00000000-0000-0000-0000-00000000d0fa';

describe('Chunks HTTP API (containerized Postgres)', () => {
  let app: INestApplication<Server>;
  let database: TestDatabase;
  let sessionTokenService: SessionTokenService;
  let defaultToken: string;

  beforeAll(async () => {
    database = await setUpTestDatabase();

    const moduleRef: TestingModule = await Test.createTestingModule({
      imports: [AppModule],
    }).compile();

    app = moduleRef.createNestApplication();
    await app.init();
    sessionTokenService = moduleRef.get(SessionTokenService);
    defaultToken = sessionTokenService.sign({
      stakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
      authTime: Math.floor(Date.now() / 1000),
      workspaceId: WORKSPACE_ID,
    });
    // G21 SG3: branch-scoped search/getNeighbourhood gating now checks the stakeholder_disciplines
    // allow-list (sourced from the branch's own discipline) instead of a per-token discipline
    // claim. Grant the bootstrap stakeholder 'engineering' so existing branch-scoped fixtures
    // (which create 'engineering' branches) keep working.
    await database.pool.query(
      `INSERT INTO stakeholder_disciplines (workspace_id, stakeholder_id, discipline)
       VALUES ($1, $2, 'engineering')
       ON CONFLICT DO NOTHING`,
      [WORKSPACE_ID, BOOTSTRAP_STAKEHOLDER_ID],
    );
  });

  function tokenFor(workspaceId: string): string {
    return sessionTokenService.sign({
      stakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
      authTime: Math.floor(Date.now() / 1000),
      workspaceId,
    });
  }

  afterAll(async () => {
    await app.close();
    await database.close();
  });

  async function createWorkspaceFor(stakeholderId: string): Promise<string> {
    const token = sessionTokenService.sign({
      stakeholderId,
      authTime: Math.floor(Date.now() / 1000),
      workspaceId: null,
    });
    const response = await request(app.getHttpServer())
      .post('/workspaces')
      .set('Authorization', `Bearer ${token}`)
      .send({ name: `e2e-chunks-workspace-${randomUUID()}` });

    expect(response.status).toBe(201);
    return response.body.id as string;
  }

  it('POST /chunks creates a chunk and GET /chunks/:id retrieves it', async () => {
    const createResponse = await request(app.getHttpServer())
      .post('/chunks')
      .set('Authorization', `Bearer ${defaultToken}`)
      .set('X-Workspace-Id', WORKSPACE_ID)
      .send({
        label: `e2e-${Math.random().toString(36).slice(2, 10)}`,
        content: 'An atomic idea captured over HTTP.',
        discipline: 'engineering',
        chunkType: 'feature',
        contextKind: 'permanent',
      });

    expect(createResponse.status).toBe(201);
    expect(createResponse.body.id).toBeTruthy();
    expect(createResponse.body.status).toBe('draft');

    const getResponse = await request(app.getHttpServer())
      .get(`/chunks/${createResponse.body.id as string}`)
      .set('Authorization', `Bearer ${defaultToken}`)
      .set('X-Workspace-Id', WORKSPACE_ID)
      .query({ stakeholderId: BOOTSTRAP_STAKEHOLDER_ID });

    expect(getResponse.status).toBe(200);
    expect(getResponse.body).toMatchObject({
      id: createResponse.body.id,
      chunkType: 'feature',
      contextKind: 'permanent',
      discipline: 'engineering',
    });
  });

  it('POST /chunks returns 400 for an invalid vocab value', async () => {
    const response = await request(app.getHttpServer())
      .post('/chunks')
      .set('Authorization', `Bearer ${defaultToken}`)
      .set('X-Workspace-Id', WORKSPACE_ID)
      .send({
        label: 'bad-vocab',
        content: 'content',
        discipline: 'bogus',
        chunkType: 'feature',
        contextKind: 'permanent',
      });

    expect(response.status).toBe(400);
  });

  it('POST /chunks returns 400 for a missing required field', async () => {
    const response = await request(app.getHttpServer())
      .post('/chunks')
      .set('Authorization', `Bearer ${defaultToken}`)
      .send({
      content: 'content',
      discipline: 'engineering',
      chunkType: 'feature',
      contextKind: 'permanent',
    });

    expect(response.status).toBe(400);
  });

  it('POST /chunks returns 403 when the stakeholder is not a current member of the workspace (authorship is claim-derived, not client-supplied)', async () => {
    const nonMemberToken = sessionTokenService.sign({
      stakeholderId: randomUUID(),
      authTime: Math.floor(Date.now() / 1000),
      workspaceId: WORKSPACE_ID,
    });

    const response = await request(app.getHttpServer())
      .post('/chunks')
      .set('Authorization', `Bearer ${nonMemberToken}`)
      .set('X-Workspace-Id', WORKSPACE_ID)
      .send({
        label: `e2e-non-member-${Math.random().toString(36).slice(2, 10)}`,
        content: 'content',
        discipline: 'engineering',
        chunkType: 'feature',
        contextKind: 'permanent',
      });

    expect(response.status).toBe(403);
  });

  it('GET /chunks/:id returns 404 for an unknown id', async () => {
    const response = await request(app.getHttpServer())
      .get('/chunks/00000000-0000-0000-0000-00000000dead')
      .set('Authorization', `Bearer ${defaultToken}`)
      .set('X-Workspace-Id', WORKSPACE_ID);

    expect(response.status).toBe(404);
  });

  it('GET /chunks/:id returns 403 when the stakeholder is not a current member of the workspace', async () => {
    const createResponse = await request(app.getHttpServer())
      .post('/chunks')
      .set('Authorization', `Bearer ${defaultToken}`)
      .set('X-Workspace-Id', WORKSPACE_ID)
      .send({
        label: `e2e-get-non-member-${Math.random().toString(36).slice(2, 10)}`,
        content: 'content',
        discipline: 'engineering',
        chunkType: 'feature',
        contextKind: 'permanent',
      });
    expect(createResponse.status).toBe(201);

    const nonMemberToken = sessionTokenService.sign({
      stakeholderId: randomUUID(),
      authTime: Math.floor(Date.now() / 1000),
      workspaceId: WORKSPACE_ID,
    });

    const response = await request(app.getHttpServer())
      .get(`/chunks/${createResponse.body.id as string}`)
      .set('Authorization', `Bearer ${nonMemberToken}`)
      .set('X-Workspace-Id', WORKSPACE_ID);

    expect(response.status).toBe(403);
  });

  it('POST /chunks attaches a chunk to an existing draft branch', async () => {
    const branchResponse = await request(app.getHttpServer())
      .post('/branches')
      .set('Authorization', `Bearer ${defaultToken}`)
      .set('X-Workspace-Id', WORKSPACE_ID)
      .send({
        name: `e2e-branch-${Math.random().toString(36).slice(2, 10)}`,
        discipline: 'engineering',
        stakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
      });
    expect(branchResponse.status).toBe(201);
    const branchId = branchResponse.body.id as string;

    const createResponse = await request(app.getHttpServer())
      .post('/chunks')
      .set('Authorization', `Bearer ${defaultToken}`)
      .set('X-Workspace-Id', WORKSPACE_ID)
      .send({
        label: `e2e-branch-scoped-${Math.random().toString(36).slice(2, 10)}`,
        content: 'A branch-scoped idea.',
        discipline: 'engineering',
        chunkType: 'feature',
        contextKind: 'permanent',
        branchId,
      });

    expect(createResponse.status).toBe(201);
    expect(createResponse.body.branchId).toBe(branchId);
    expect(createResponse.body.originBranchId).toBe(branchId);

    const getResponse = await request(app.getHttpServer())
      .get(`/chunks/${createResponse.body.id as string}`)
      .set('Authorization', `Bearer ${defaultToken}`)
      .set('X-Workspace-Id', WORKSPACE_ID)
      .query({ stakeholderId: BOOTSTRAP_STAKEHOLDER_ID });

    expect(getResponse.status).toBe(200);
    expect(getResponse.body.branchId).toBe(branchId);
    expect(getResponse.body.originBranchId).toBe(branchId);
  });

  it('POST /chunks returns 404 when branchId does not exist', async () => {
    const response = await request(app.getHttpServer())
      .post('/chunks')
      .set('Authorization', `Bearer ${defaultToken}`)
      .set('X-Workspace-Id', WORKSPACE_ID)
      .send({
        label: `e2e-missing-branch-${Math.random().toString(36).slice(2, 10)}`,
        content: 'content',
        discipline: 'engineering',
        chunkType: 'feature',
        contextKind: 'permanent',
        branchId: '00000000-0000-0000-0000-00000000dead',
      });

    expect(response.status).toBe(404);
  });

  it('POST /chunks returns 409 when branch discipline does not match the request', async () => {
    const branchResponse = await request(app.getHttpServer())
      .post('/branches')
      .set('Authorization', `Bearer ${defaultToken}`)
      .set('X-Workspace-Id', WORKSPACE_ID)
      .send({
        name: `e2e-branch-${Math.random().toString(36).slice(2, 10)}`,
        discipline: 'design',
        stakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
      });
    expect(branchResponse.status).toBe(201);

    const response = await request(app.getHttpServer())
      .post('/chunks')
      .set('Authorization', `Bearer ${defaultToken}`)
      .set('X-Workspace-Id', WORKSPACE_ID)
      .send({
        label: `e2e-wrong-discipline-${Math.random().toString(36).slice(2, 10)}`,
        content: 'content',
        discipline: 'engineering',
        chunkType: 'feature',
        contextKind: 'permanent',
        branchId: branchResponse.body.id,
      });

    expect(response.status).toBe(409);
  });

  it('allows the same chunk label to be created independently in two different workspaces', async () => {
    const otherWorkspaceId = await createWorkspaceFor(BOOTSTRAP_STAKEHOLDER_ID);
    const label = `e2e-cross-workspace-${Math.random().toString(36).slice(2, 10)}`;

    const firstResponse = await request(app.getHttpServer())
      .post('/chunks')
      .set('Authorization', `Bearer ${defaultToken}`)
      .set('X-Workspace-Id', WORKSPACE_ID)
      .send({
        label,
        content: 'An idea captured in the default workspace.',
        discipline: 'engineering',
        chunkType: 'feature',
        contextKind: 'permanent',
      });
    expect(firstResponse.status).toBe(201);

    const secondResponse = await request(app.getHttpServer())
      .post('/chunks')
      .set('Authorization', `Bearer ${tokenFor(otherWorkspaceId)}`)
      .set('X-Workspace-Id', otherWorkspaceId)
      .send({
        label,
        content: 'The same label captured in a different workspace.',
        discipline: 'engineering',
        chunkType: 'feature',
        contextKind: 'permanent',
      });
    expect(secondResponse.status).toBe(201);
    expect(secondResponse.body.id).not.toBe(firstResponse.body.id);

    const firstGet = await request(app.getHttpServer())
      .get(`/chunks/${firstResponse.body.id as string}`)
      .set('Authorization', `Bearer ${defaultToken}`)
      .set('X-Workspace-Id', WORKSPACE_ID)
      .query({ stakeholderId: BOOTSTRAP_STAKEHOLDER_ID });
    expect(firstGet.status).toBe(200);
    expect(firstGet.body.label).toBe(label);

    const secondGet = await request(app.getHttpServer())
      .get(`/chunks/${secondResponse.body.id as string}`)
      .set('Authorization', `Bearer ${tokenFor(otherWorkspaceId)}`)
      .set('X-Workspace-Id', otherWorkspaceId)
      .query({ stakeholderId: BOOTSTRAP_STAKEHOLDER_ID });
    expect(secondGet.status).toBe(200);
    expect(secondGet.body.label).toBe(label);

    // Cross-workspace reads must not resolve: fetching the other workspace's chunk id under this
    // workspace's header is indistinguishable from "doesn't exist" (never leaks existence).
    const crossWorkspaceGet = await request(app.getHttpServer())
      .get(`/chunks/${secondResponse.body.id as string}`)
      .set('Authorization', `Bearer ${defaultToken}`)
      .set('X-Workspace-Id', WORKSPACE_ID)
      .query({ stakeholderId: BOOTSTRAP_STAKEHOLDER_ID });
    expect(crossWorkspaceGet.status).toBe(404);
  });

  describe('GET /chunks (Search & Pagination)', () => {
    let validToken: string;

    beforeAll(() => {
      validToken = sessionTokenService.sign({
        stakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
        authTime: Math.floor(Date.now() / 1000),
        workspaceId: WORKSPACE_ID,
      });
    });

    it('returns 401 when Authorization header is missing', async () => {
      const response = await request(app.getHttpServer())
        .get('/chunks')
        .set('X-Workspace-Id', WORKSPACE_ID);

      expect(response.status).toBe(401);
    });

    it('returns 403 when X-Workspace-Id header is missing', async () => {
      const response = await request(app.getHttpServer())
        .get('/chunks')
        .set('Authorization', `Bearer ${validToken}`);

      expect(response.status).toBe(403);
    });

    it('returns 403 when token workspaceId does not match X-Workspace-Id', async () => {
      const mismatchedToken = sessionTokenService.sign({
        stakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
        authTime: Math.floor(Date.now() / 1000),
        workspaceId: '00000000-0000-0000-0000-0000000000ff',
      });
      const response = await request(app.getHttpServer())
        .get('/chunks')
        .set('Authorization', `Bearer ${mismatchedToken}`)
        .set('X-Workspace-Id', WORKSPACE_ID);

      expect(response.status).toBe(403);
    });

    it('returns 400 when filtering by branchId and activeDiscipline is missing (G21 SG4)', async () => {
      const branchResponse = await request(app.getHttpServer())
        .post('/branches')
        .set('Authorization', `Bearer ${validToken}`)
        .set('X-Workspace-Id', WORKSPACE_ID)
        .send({
          name: `e2e-branch-${Math.random().toString(36).slice(2, 10)}`,
          discipline: 'engineering',
          stakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
        });
      const branchId = branchResponse.body.id as string;

      const response = await request(app.getHttpServer())
        .get('/chunks')
        .set('Authorization', `Bearer ${validToken}`)
        .set('X-Workspace-Id', WORKSPACE_ID)
        .query({ branchId });

      expect(response.status).toBe(400);
    });

    it('returns 403 when filtering by branchId and activeDiscipline is a valid vocabulary value but disallowed (G21 SG4)', async () => {
      const branchResponse = await request(app.getHttpServer())
        .post('/branches')
        .set('Authorization', `Bearer ${validToken}`)
        .set('X-Workspace-Id', WORKSPACE_ID)
        .send({
          name: `e2e-branch-${Math.random().toString(36).slice(2, 10)}`,
          discipline: 'design',
          stakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
        });
      const branchId = branchResponse.body.id as string;

      // validToken's stakeholder is only allowed 'engineering' in this workspace (seeded in the
      // outer beforeAll), not 'design'.
      const response = await request(app.getHttpServer())
        .get('/chunks')
        .set('Authorization', `Bearer ${validToken}`)
        .set('X-Workspace-Id', WORKSPACE_ID)
        .query({ branchId, activeDiscipline: 'design' });

      expect(response.status).toBe(403);
    });

    it('returns 200 when filtering by branchId and activeDiscipline is allowed (G21 SG4)', async () => {
      const branchResponse = await request(app.getHttpServer())
        .post('/branches')
        .set('Authorization', `Bearer ${validToken}`)
        .set('X-Workspace-Id', WORKSPACE_ID)
        .send({
          name: `e2e-branch-${Math.random().toString(36).slice(2, 10)}`,
          discipline: 'engineering',
          stakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
        });
      const branchId = branchResponse.body.id as string;

      const response = await request(app.getHttpServer())
        .get('/chunks')
        .set('Authorization', `Bearer ${validToken}`)
        .set('X-Workspace-Id', WORKSPACE_ID)
        .query({ branchId, activeDiscipline: 'engineering' });

      expect(response.status).toBe(200);
    });

    it('successfully searches and paginates chunks', async () => {
      // Create some chunks for searching
      const uniqueSuffix = Math.random().toString(36).slice(2, 10);
      for (let i = 0; i < 3; i++) {
        await request(app.getHttpServer())
          .post('/chunks')
          .set('Authorization', `Bearer ${validToken}`)
          .set('X-Workspace-Id', WORKSPACE_ID)
          .send({
            label: `e2e-search-${uniqueSuffix}-${String(i)}`,
            content: `Searchable content ${uniqueSuffix} number ${String(i)}`,
            discipline: 'engineering',
            chunkType: 'feature',
            contextKind: 'permanent',
          });
      }

      // 1. Search with full text 'q'
      const searchRes = await request(app.getHttpServer())
        .get('/chunks')
        .set('Authorization', `Bearer ${validToken}`)
        .set('X-Workspace-Id', WORKSPACE_ID)
        .query({ q: uniqueSuffix, limit: 2 });

      expect(searchRes.status).toBe(200);
      expect(searchRes.body.chunks).toHaveLength(2);
      expect(searchRes.body.nextCursor).toBeTruthy();

      // 2. Fetch next page
      const nextRes = await request(app.getHttpServer())
        .get('/chunks')
        .set('Authorization', `Bearer ${validToken}`)
        .set('X-Workspace-Id', WORKSPACE_ID)
        .query({ q: uniqueSuffix, limit: 2, cursor: searchRes.body.nextCursor });

      expect(nextRes.status).toBe(200);
      expect(nextRes.body.chunks).toHaveLength(1); // 3 total, got 2, 1 left
      expect(nextRes.body.nextCursor).toBeNull();
    });
  describe('GET /chunks/:id/neighbourhood', () => {
    let validToken: string;

    beforeAll(() => {
      validToken = sessionTokenService.sign({
        stakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
        authTime: Math.floor(Date.now() / 1000),
        workspaceId: WORKSPACE_ID,
      });
    });

    it('returns 400 if depth is invalid or exceeds 5', async () => {
      const response1 = await request(app.getHttpServer())
        .get('/chunks/chunk-123/neighbourhood')
        .set('Authorization', `Bearer ${validToken}`)
        .set('X-Workspace-Id', WORKSPACE_ID)
        .query({ depth: 6 });
      expect(response1.status).toBe(400);

      const response2 = await request(app.getHttpServer())
        .get('/chunks/chunk-123/neighbourhood')
        .set('Authorization', `Bearer ${validToken}`)
        .set('X-Workspace-Id', WORKSPACE_ID)
        .query({ depth: -1 });
      expect(response2.status).toBe(400);

      const response3 = await request(app.getHttpServer())
        .get('/chunks/chunk-123/neighbourhood')
        .set('Authorization', `Bearer ${validToken}`)
        .set('X-Workspace-Id', WORKSPACE_ID)
        .query({ depth: 'abc' });
      expect(response3.status).toBe(400);
    });

    it('returns 404 for an unknown id', async () => {
      const response = await request(app.getHttpServer())
        .get('/chunks/00000000-0000-0000-0000-00000000dead/neighbourhood')
        .set('Authorization', `Bearer ${validToken}`)
        .set('X-Workspace-Id', WORKSPACE_ID);
      expect(response.status).toBe(404);
    });

    it('traverses the neighbourhood, avoids cycles, and respects depth', async () => {
      // Create chunks A, B, C, D
      const suffix = Math.random().toString(36).slice(2, 10);
      const labels = ['a', 'b', 'c', 'd'].map(l => `nh-${l}-${suffix}`);
      const chunkIds: Record<string, string> = {};

      for (const label of labels) {
        const res = await request(app.getHttpServer())
          .post('/chunks')
          .set('Authorization', `Bearer ${validToken}`)
          .set('X-Workspace-Id', WORKSPACE_ID)
          .send({
            label,
            content: `content for ${label}`,
            discipline: 'engineering',
            chunkType: 'feature',
            contextKind: 'permanent',
          });
        expect(res.status).toBe(201);
        chunkIds[label] = res.body.id as string;
      }

      // Create edges: A -> B, B -> C, C -> A (cycle), B -> D
      const edgeDefs = [
        { from: labels[0], to: labels[1] }, // A -> B
        { from: labels[1], to: labels[2] }, // B -> C
        { from: labels[2], to: labels[0] }, // C -> A
        { from: labels[1], to: labels[3] }, // B -> D
      ];

      for (const edge of edgeDefs) {
        const res = await request(app.getHttpServer())
          .post('/edges')
          .set('Authorization', `Bearer ${validToken}`)
          .set('X-Workspace-Id', WORKSPACE_ID)
          .send({
            fromChunkLabel: edge.from,
            toChunkLabel: edge.to,
            type: 'depends-on',
            discipline: 'engineering',
            stakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
          });
        expect(res.status).toBe(201);
      }

      // Query A neighbourhood with depth 1
      const depth1 = await request(app.getHttpServer())
        .get(`/chunks/${String(chunkIds[labels[0]])}/neighbourhood`)
        .set('Authorization', `Bearer ${validToken}`)
        .set('X-Workspace-Id', WORKSPACE_ID)
        .query({ depth: 1 });

      expect(depth1.status).toBe(200);
      expect(depth1.body.chunk.id).toBe(chunkIds[labels[0]]);
      expect(depth1.body.neighbours).toHaveLength(2); // A->B and C->A (incoming)
      const neighbours1 = depth1.body.neighbours.map((n: { label: string }) => n.label).sort();
      expect(neighbours1).toEqual([labels[1], labels[2]].sort());
      
      // Query A neighbourhood with depth 2
      const depth2 = await request(app.getHttpServer())
        .get(`/chunks/${String(chunkIds[labels[0]])}/neighbourhood`)
        .set('Authorization', `Bearer ${validToken}`)
        .set('X-Workspace-Id', WORKSPACE_ID)
        .query({ depth: 2 });

      expect(depth2.status).toBe(200);
      expect(depth2.body.neighbours).toHaveLength(3); // B, C, and now D (from B)
      const neighbours2 = depth2.body.neighbours.map((n: { label: string }) => n.label).sort();
      expect(neighbours2).toEqual([labels[1], labels[2], labels[3]].sort());
    });

    });
  });
});
