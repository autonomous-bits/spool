import { randomUUID } from 'node:crypto';
import type { INestApplication } from '@nestjs/common';
import type { Server } from 'node:http';
import { Test, type TestingModule } from '@nestjs/testing';
import request from 'supertest';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { AppModule } from '../src/app.module.js';
import { SessionTokenService } from '../src/auth/session-token.service.js';
import { setUpTestDatabase, type TestDatabase } from './support/test-database.js';

const WORKSPACE_ID = '00000000-0000-0000-0000-00000000d0fa';

function uniqueName(prefix: string): string {
  return `${prefix}-${randomUUID()}`;
}

describe('Notifications HTTP API (containerized Postgres)', () => {
  let app: INestApplication<Server>;
  let database: TestDatabase;
  let sessionTokenService: SessionTokenService;
  let engineeringStakeholderId: string;

  beforeAll(async () => {
    database = await setUpTestDatabase();

    engineeringStakeholderId = randomUUID();

    await database.pool.query(
      `INSERT INTO stakeholders (id, name, email, role, discipline, github_login)
       VALUES ($1, 'Engineering Stakeholder', $2, 'stakeholder', 'engineering', $3)`,
      [
        engineeringStakeholderId,
        `engineering-${engineeringStakeholderId}@spool.local`,
        `engineering-${engineeringStakeholderId}`,
      ],
    );
    await database.pool.query(
      `INSERT INTO workspace_memberships (workspace_id, stakeholder_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
      [WORKSPACE_ID, engineeringStakeholderId],
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

  const BEARER_PREFIX = 'Bearer ';

  function authHeader(token: string): string {
    return BEARER_PREFIX.concat(token);
  }

  function mintSessionToken(stakeholderId: string, discipline: string | null): string {
    return sessionTokenService.sign({
      stakeholderId,
      discipline,
      authTime: Math.floor(Date.now() / 1000),
      workspaceId: WORKSPACE_ID,
    });
  }

  async function seedStakeholder(): Promise<string> {
    const id = randomUUID();
    await database.pool.query(
      `INSERT INTO stakeholders (id, name, email, role, discipline, github_login)
       VALUES ($1, $2, $3, 'stakeholder', 'engineering', $4)`,
      [id, `Stakeholder ${id}`, `${id}@spool.local`, `login-${id}`],
    );
    await database.pool.query(
      `INSERT INTO workspace_memberships (workspace_id, stakeholder_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
      [WORKSPACE_ID, id],
    );
    return id;
  }

  /**
   * Creates a submitted branch and one verification signal against it, which fans out an
   * unread `feedback_notifications` row to every member of the branch's workspace (Meridian
   * IDEA-67/IDEA-98/IDEA-103, G09 SG2), so each call adds one fresh notification per stakeholder
   * that is a member of `WORKSPACE_ID`.
   */
  async function createSignal(): Promise<void> {
    const token = mintSessionToken(engineeringStakeholderId, 'engineering');
    const createResponse = await request(app.getHttpServer())
      .post('/branches')
      .set('X-Workspace-Id', WORKSPACE_ID)
      .set('Authorization', authHeader(token))
      .send({
        name: uniqueName('e2e-notification-branch'),
        discipline: 'engineering',
        stakeholderId: engineeringStakeholderId,
      });
    expect(createResponse.status).toBe(201);
    const branchId = createResponse.body.id as string;

    const submitResponse = await request(app.getHttpServer())
      .post(`/branches/${branchId}/submit`)
      .set('X-Workspace-Id', WORKSPACE_ID)
      .set('Authorization', authHeader(token));
    expect(submitResponse.status).toBe(201);

    const signalResponse = await request(app.getHttpServer())
      .post(`/branches/${branchId}/verification-signals`)
      .set('X-Workspace-Id', WORKSPACE_ID)
      .set('Authorization', authHeader(token))
      .send({ verifierName: 'ci-evaluator', status: 'pass' });
    expect(signalResponse.status).toBe(201);
  }

  it('GET /notifications with a valid session token returns only that stakeholder\'s rows, newest first', async () => {
    const stakeholderId = await seedStakeholder();
    const otherStakeholderId = await seedStakeholder();
    await createSignal();
    await createSignal();

    const token = mintSessionToken(stakeholderId, 'engineering');
    const response = await request(app.getHttpServer())
      .get('/notifications')
      .set('X-Workspace-Id', WORKSPACE_ID)
      .set('Authorization', authHeader(token));

    expect(response.status).toBe(200);
    const body = response.body as { id: string; stakeholderId: string; createdAt: string }[];
    expect(body.length).toBeGreaterThanOrEqual(2);
    expect(body.every((n) => n.stakeholderId === stakeholderId)).toBe(true);
    expect(body.every((n) => n.stakeholderId !== otherStakeholderId)).toBe(true);
    const createdAtTimestamps = body.map((n) => new Date(n.createdAt).getTime());
    expect(createdAtTimestamps).toEqual([...createdAtTimestamps].sort((a, b) => b - a));
  });

  it('GET /notifications?status=unread returns only unread rows for that stakeholder', async () => {
    const stakeholderId = await seedStakeholder();
    await createSignal();
    await createSignal();

    const token = mintSessionToken(stakeholderId, 'engineering');
    const listResponse = await request(app.getHttpServer())
      .get('/notifications')
      .set('X-Workspace-Id', WORKSPACE_ID)
      .set('Authorization', authHeader(token));
    const notificationId = (listResponse.body as { id: string }[])[0]?.id;
    if (notificationId === undefined) {
      throw new Error('expected at least one notification');
    }

    await request(app.getHttpServer())
      .post(`/notifications/${notificationId}/read`)
      .set('X-Workspace-Id', WORKSPACE_ID)
      .set('Authorization', authHeader(token));

    const unreadResponse = await request(app.getHttpServer())
      .get('/notifications?status=unread')
      .set('X-Workspace-Id', WORKSPACE_ID)
      .set('Authorization', authHeader(token));

    expect(unreadResponse.status).toBe(200);
    const unreadBody = unreadResponse.body as { id: string; status: string }[];
    expect(unreadBody.every((n) => n.status === 'unread')).toBe(true);
    expect(unreadBody.map((n) => n.id)).not.toContain(notificationId);
  });

  it('POST /notifications/:id/read on the caller\'s own unread notification returns it with status=read', async () => {
    const stakeholderId = await seedStakeholder();
    await createSignal();

    const token = mintSessionToken(stakeholderId, 'engineering');
    const listResponse = await request(app.getHttpServer())
      .get('/notifications')
      .set('X-Workspace-Id', WORKSPACE_ID)
      .set('Authorization', authHeader(token));
    const notificationId = (listResponse.body as { id: string }[])[0]?.id;
    if (notificationId === undefined) {
      throw new Error('expected at least one notification');
    }

    const readResponse = await request(app.getHttpServer())
      .post(`/notifications/${notificationId}/read`)
      .set('X-Workspace-Id', WORKSPACE_ID)
      .set('Authorization', authHeader(token));

    expect(readResponse.status).toBe(201);
    expect(readResponse.body).toMatchObject({ id: notificationId, status: 'read' });
  });

  it('POST /notifications/:id/read on another stakeholder\'s notification returns 404', async () => {
    const stakeholderId = await seedStakeholder();
    const otherStakeholderId = await seedStakeholder();
    await createSignal();

    const token = mintSessionToken(stakeholderId, 'engineering');
    const listResponse = await request(app.getHttpServer())
      .get('/notifications')
      .set('X-Workspace-Id', WORKSPACE_ID)
      .set('Authorization', authHeader(token));
    const notificationId = (listResponse.body as { id: string }[])[0]?.id;
    if (notificationId === undefined) {
      throw new Error('expected at least one notification');
    }

    const otherToken = mintSessionToken(otherStakeholderId, 'engineering');
    const response = await request(app.getHttpServer())
      .post(`/notifications/${notificationId}/read`)
      .set('X-Workspace-Id', WORKSPACE_ID)
      .set('Authorization', authHeader(otherToken));

    expect(response.status).toBe(404);
  });

  it('GET /notifications without a valid session token returns 401', async () => {
    const response = await request(app.getHttpServer())
      .get('/notifications')
      .set('X-Workspace-Id', WORKSPACE_ID);
    expect(response.status).toBe(401);
  });

  it('GET /notifications with a valid session token but missing X-Workspace-Id header returns 403', async () => {
    const stakeholderId = await seedStakeholder();
    const token = mintSessionToken(stakeholderId, 'engineering');

    const response = await request(app.getHttpServer())
      .get('/notifications')
      .set('Authorization', authHeader(token));

    expect(response.status).toBe(403);
  });

  it('GET /notifications with a valid session token but mismatched X-Workspace-Id header returns 403', async () => {
    const stakeholderId = await seedStakeholder();
    const token = mintSessionToken(stakeholderId, 'engineering');

    const response = await request(app.getHttpServer())
      .get('/notifications')
      .set('X-Workspace-Id', '00000000-0000-0000-0000-00000000beef')
      .set('Authorization', authHeader(token));

    expect(response.status).toBe(403);
  });

  it('POST /notifications/:id/read without a valid session token returns 401', async () => {
    const response = await request(app.getHttpServer())
      .post(`/notifications/${randomUUID()}/read`)
      .set('X-Workspace-Id', WORKSPACE_ID);
    expect(response.status).toBe(401);
  });

  it('POST /notifications/:id/read with a valid session token but missing X-Workspace-Id header returns 403', async () => {
    const stakeholderId = await seedStakeholder();
    const token = mintSessionToken(stakeholderId, 'engineering');

    const response = await request(app.getHttpServer())
      .post(`/notifications/${randomUUID()}/read`)
      .set('Authorization', authHeader(token));

    expect(response.status).toBe(403);
  });
});
