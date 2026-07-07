import { randomUUID } from 'node:crypto';
import type { INestApplication } from '@nestjs/common';
import type { Server } from 'node:http';
import { Test, type TestingModule } from '@nestjs/testing';
import request from 'supertest';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { AppModule } from '../src/app.module.js';
import { SessionTokenService } from '../src/auth/session-token.service.js';
import { setUpTestDatabase, type TestDatabase } from './support/test-database.js';

function uniqueName(prefix: string): string {
  return `${prefix}-${randomUUID()}`;
}

describe('Workspaces HTTP API (containerized Postgres)', () => {
  let app: INestApplication<Server>;
  let database: TestDatabase;
  let sessionTokenService: SessionTokenService;
  let creatorStakeholderId: string;
  let otherStakeholderId: string;
  let nonMemberStakeholderId: string;

  beforeAll(async () => {
    database = await setUpTestDatabase();

    creatorStakeholderId = randomUUID();
    otherStakeholderId = randomUUID();
    nonMemberStakeholderId = randomUUID();

    await database.pool.query(
      `INSERT INTO stakeholders (id, name, email, role, discipline, github_login)
       VALUES
         ($1, 'Creator Stakeholder', $2, 'stakeholder', 'engineering', $3),
         ($4, 'Other Stakeholder', $5, 'stakeholder', 'product', $6),
         ($7, 'Non-Member Stakeholder', $8, 'stakeholder', NULL, $9)`,
      [
        creatorStakeholderId,
        `creator-${creatorStakeholderId}@spool.local`,
        `creator-${creatorStakeholderId}`,
        otherStakeholderId,
        `other-${otherStakeholderId}@spool.local`,
        `other-${otherStakeholderId}`,
        nonMemberStakeholderId,
        `non-member-${nonMemberStakeholderId}@spool.local`,
        `non-member-${nonMemberStakeholderId}`,
      ],
    );

    const moduleRef: TestingModule = await Test.createTestingModule({
      imports: [AppModule],
    }).compile();

    app = moduleRef.createNestApplication();
    await app.init();
    sessionTokenService = moduleRef.get(SessionTokenService);
  });

  afterAll(async () => {
    await app.close();
    await database.close();
  });

  function mintSessionToken(stakeholderId: string, discipline: string | null, workspaceId: string | null = null): string {
    return sessionTokenService.sign({
      stakeholderId,
      discipline,
      authTime: Math.floor(Date.now() / 1000),
      workspaceId,
    });
  }

  async function createWorkspace(stakeholderId: string): Promise<string> {
    const token = mintSessionToken(stakeholderId, 'engineering');
    const response = await request(app.getHttpServer())
      .post('/workspaces')
      .set('Authorization', `Bearer ${token}`)
      .send({ name: uniqueName('e2e-workspace') });

    expect(response.status).toBe(201);
    return response.body.id as string;
  }

  it('POST /workspaces creates a workspace with the caller as sole initial member', async () => {
    const token = mintSessionToken(creatorStakeholderId, 'engineering');
    const name = uniqueName('e2e-workspace');

    const response = await request(app.getHttpServer())
      .post('/workspaces')
      .set('Authorization', `Bearer ${token}`)
      .send({ name });

    expect(response.status).toBe(201);
    expect(response.body).toMatchObject({
      name,
      createdByStakeholderId: creatorStakeholderId,
    });
    expect(response.body.id).toBeTruthy();
    expect(response.body.createdAt).toBeTruthy();

    const membershipRows = await database.pool.query(
      'SELECT stakeholder_id FROM workspace_memberships WHERE workspace_id = $1',
      [response.body.id],
    );
    expect(membershipRows.rows).toEqual([{ stakeholder_id: creatorStakeholderId }]);
  });

  it('POST /workspaces returns 401 when Authorization is missing', async () => {
    const response = await request(app.getHttpServer())
      .post('/workspaces')
      .send({ name: uniqueName('e2e-workspace') });

    expect(response.status).toBe(401);
  });

  it('POST /workspaces returns 401 for a malformed Authorization header', async () => {
    const response = await request(app.getHttpServer())
      .post('/workspaces')
      .set('Authorization', 'Token not-a-bearer-token')
      .send({ name: uniqueName('e2e-workspace') });

    expect(response.status).toBe(401);
  });

  it('POST /workspaces/:id/members adds an existing member as a new member', async () => {
    const workspaceId = await createWorkspace(creatorStakeholderId);
    const token = mintSessionToken(creatorStakeholderId, 'engineering', workspaceId);

    const response = await request(app.getHttpServer())
      .post(`/workspaces/${workspaceId}/members`)
      .set('Authorization', `Bearer ${token}`)
      .set('X-Workspace-Id', workspaceId)
      .send({ stakeholderId: otherStakeholderId });

    expect(response.status).toBe(201);
    expect(response.body).toMatchObject({
      workspaceId,
      stakeholderId: otherStakeholderId,
    });

    const membershipRows = await database.pool.query(
      'SELECT stakeholder_id FROM workspace_memberships WHERE workspace_id = $1 AND stakeholder_id = $2',
      [workspaceId, otherStakeholderId],
    );
    expect(membershipRows.rows).toHaveLength(1);
  });

  it('POST /workspaces/:id/members returns 401 when Authorization is missing', async () => {
    const workspaceId = await createWorkspace(creatorStakeholderId);

    const response = await request(app.getHttpServer())
      .post(`/workspaces/${workspaceId}/members`)
      .set('X-Workspace-Id', workspaceId)
      .send({ stakeholderId: otherStakeholderId });

    expect(response.status).toBe(401);
  });

  it('POST /workspaces/:id/members returns 404 for an unknown workspace id', async () => {
    const token = mintSessionToken(creatorStakeholderId, 'engineering', '00000000-0000-0000-0000-00000000dead');

    const response = await request(app.getHttpServer())
      .post('/workspaces/00000000-0000-0000-0000-00000000dead/members')
      .set('Authorization', `Bearer ${token}`)
      .set('X-Workspace-Id', '00000000-0000-0000-0000-00000000dead')
      .send({ stakeholderId: otherStakeholderId });

    expect(response.status).toBe(404);
  });

  it('POST /workspaces/:id/members returns 403 when the caller is not a member', async () => {
    const workspaceId = await createWorkspace(creatorStakeholderId);
    const token = mintSessionToken(nonMemberStakeholderId, null, workspaceId);

    const response = await request(app.getHttpServer())
      .post(`/workspaces/${workspaceId}/members`)
      .set('Authorization', `Bearer ${token}`)
      .set('X-Workspace-Id', workspaceId)
      .send({ stakeholderId: otherStakeholderId });

    expect(response.status).toBe(403);
  });

  it('POST /workspaces/:id/members returns 404 for an unknown target stakeholderId', async () => {
    const workspaceId = await createWorkspace(creatorStakeholderId);
    const token = mintSessionToken(creatorStakeholderId, 'engineering', workspaceId);

    const response = await request(app.getHttpServer())
      .post(`/workspaces/${workspaceId}/members`)
      .set('Authorization', `Bearer ${token}`)
      .set('X-Workspace-Id', workspaceId)
      .send({ stakeholderId: '00000000-0000-0000-0000-00000000beef' });

    expect(response.status).toBe(404);
  });

  it('POST /workspaces/:id/members returns 409 when the target is already a member', async () => {
    const workspaceId = await createWorkspace(creatorStakeholderId);
    const token = mintSessionToken(creatorStakeholderId, 'engineering', workspaceId);

    const firstResponse = await request(app.getHttpServer())
      .post(`/workspaces/${workspaceId}/members`)
      .set('Authorization', `Bearer ${token}`)
      .set('X-Workspace-Id', workspaceId)
      .send({ stakeholderId: otherStakeholderId });
    expect(firstResponse.status).toBe(201);

    const repeatedResponse = await request(app.getHttpServer())
      .post(`/workspaces/${workspaceId}/members`)
      .set('Authorization', `Bearer ${token}`)
      .set('X-Workspace-Id', workspaceId)
      .send({ stakeholderId: otherStakeholderId });

    expect(repeatedResponse.status).toBe(409);
  });

  it('POST /workspaces/:id/members returns 400 for a missing stakeholderId', async () => {
    const workspaceId = await createWorkspace(creatorStakeholderId);
    const token = mintSessionToken(creatorStakeholderId, 'engineering', workspaceId);

    const response = await request(app.getHttpServer())
      .post(`/workspaces/${workspaceId}/members`)
      .set('Authorization', `Bearer ${token}`)
      .set('X-Workspace-Id', workspaceId)
      .send({});

    expect(response.status).toBe(400);
  });

  it('POST /workspaces returns 400 for a missing name', async () => {
    const token = mintSessionToken(creatorStakeholderId, 'engineering');

    const response = await request(app.getHttpServer())
      .post('/workspaces')
      .set('Authorization', `Bearer ${token}`)
      .send({});

    expect(response.status).toBe(400);
  });
});
