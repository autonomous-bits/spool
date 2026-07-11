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

describe('Verification Signals HTTP API (containerized Postgres)', () => {
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

  function mintSessionToken(stakeholderId: string, discipline: string | null): string {
    return sessionTokenService.sign({
      stakeholderId,
      discipline,
      authTime: Math.floor(Date.now() / 1000),
      workspaceId: WORKSPACE_ID,
    });
  }

  const BEARER_PREFIX = 'Bearer ';

  function authHeader(token: string): string {
    return BEARER_PREFIX.concat(token);
  }

  async function createBranch(): Promise<string> {
    const token = mintSessionToken(engineeringStakeholderId, 'engineering');
    const createResponse = await request(app.getHttpServer())
      .post('/branches')
      .set('X-Workspace-Id', WORKSPACE_ID)
      .set('Authorization', authHeader(token))
      .send({
        name: uniqueName('e2e-signal-branch'),
        discipline: 'engineering',
        stakeholderId: engineeringStakeholderId,
      });

    expect(createResponse.status).toBe(201);
    return createResponse.body.id as string;
  }

  async function createSubmittedBranch(): Promise<string> {
    const branchId = await createBranch();
    const token = mintSessionToken(engineeringStakeholderId, 'engineering');

    const submitResponse = await request(app.getHttpServer())
      .post(`/branches/${branchId}/submit`)
      .set('X-Workspace-Id', WORKSPACE_ID)
      .set('Authorization', authHeader(token));

    expect(submitResponse.status).toBe(201);
    return branchId;
  }

  async function createVerifiedBranch(): Promise<string> {
    const branchId = await createSubmittedBranch();
    const token = mintSessionToken(engineeringStakeholderId, 'engineering');

    const verifyResponse = await request(app.getHttpServer())
      .post(`/branches/${branchId}/verify`)
      .set('X-Workspace-Id', WORKSPACE_ID)
      .set('Authorization', authHeader(token));

    expect(verifyResponse.status).toBe(201);
    return branchId;
  }

  it('POST /branches/:id/verification-signals persists one row for a submitted branch and does not change branch status', async () => {
    const branchId = await createSubmittedBranch();

    const response = await request(app.getHttpServer())
      .post(`/branches/${branchId}/verification-signals`)
      .set('X-Workspace-Id', WORKSPACE_ID)
      .send({ verifierName: 'ci-evaluator', status: 'pass', reason: 'all checks green' });

    expect(response.status).toBe(201);
    expect(response.body).toMatchObject({
      branchId,
      verifierName: 'ci-evaluator',
      status: 'pass',
      reason: 'all checks green',
    });

    const row = await database.pool.query(
      'SELECT verifier_name, status, reason FROM verification_signals WHERE branch_id = $1',
      [branchId],
    );
    expect(row.rows).toHaveLength(1);
    expect(row.rows[0]).toMatchObject({
      verifier_name: 'ci-evaluator',
      status: 'pass',
      reason: 'all checks green',
    });

    const token = mintSessionToken(engineeringStakeholderId, 'engineering');
    const branchResponse = await request(app.getHttpServer())
      .get(`/branches/${branchId}`)
      .set('X-Workspace-Id', WORKSPACE_ID)
      .set('Authorization', authHeader(token))
      .query({ stakeholderId: engineeringStakeholderId });
    expect(branchResponse.body.status).toBe('submitted');
  });

  it('POST /branches/:id/verification-signals persists one row for a verified branch', async () => {
    const branchId = await createVerifiedBranch();

    const response = await request(app.getHttpServer())
      .post(`/branches/${branchId}/verification-signals`)
      .set('X-Workspace-Id', WORKSPACE_ID)
      .send({ verifierName: 'human-reviewer', status: 'fail', reason: 'needs rework' });

    expect(response.status).toBe(201);
    expect(response.body.status).toBe('fail');

    const token = mintSessionToken(engineeringStakeholderId, 'engineering');
    const branchResponse = await request(app.getHttpServer())
      .get(`/branches/${branchId}`)
      .set('X-Workspace-Id', WORKSPACE_ID)
      .set('Authorization', authHeader(token))
      .query({ stakeholderId: engineeringStakeholderId });
    expect(branchResponse.body.status).toBe('verified');
  });

  it('POST /branches/:id/verification-signals returns 404 for an unknown branchId, no row persisted', async () => {
    const response = await request(app.getHttpServer())
      .post('/branches/00000000-0000-0000-0000-00000000dead/verification-signals')
      .set('X-Workspace-Id', WORKSPACE_ID)
      .send({ verifierName: 'ci-evaluator', status: 'pass' });

    expect(response.status).toBe(404);
  });

  it('POST /branches/:id/verification-signals returns 409 for a draft branch, no row persisted', async () => {
    const branchId = await createBranch();

    const response = await request(app.getHttpServer())
      .post(`/branches/${branchId}/verification-signals`)
      .set('X-Workspace-Id', WORKSPACE_ID)
      .send({ verifierName: 'ci-evaluator', status: 'pass' });

    expect(response.status).toBe(409);

    const row = await database.pool.query(
      'SELECT id FROM verification_signals WHERE branch_id = $1',
      [branchId],
    );
    expect(row.rows).toHaveLength(0);
  });

  it('POST /branches/:id/verification-signals returns 409 for a merged branch, no row persisted', async () => {
    const branchId = await createVerifiedBranch();
    const token = mintSessionToken(engineeringStakeholderId, 'engineering');
    const mergeResponse = await request(app.getHttpServer())
      .post(`/branches/${branchId}/merge`)
      .set('X-Workspace-Id', WORKSPACE_ID)
      .set('Authorization', authHeader(token));
    expect(mergeResponse.status).toBe(201);

    const response = await request(app.getHttpServer())
      .post(`/branches/${branchId}/verification-signals`)
      .set('X-Workspace-Id', WORKSPACE_ID)
      .send({ verifierName: 'ci-evaluator', status: 'pass' });

    expect(response.status).toBe(409);

    const row = await database.pool.query(
      'SELECT id FROM verification_signals WHERE branch_id = $1',
      [branchId],
    );
    expect(row.rows).toHaveLength(0);
  });

  it('POST /branches/:id/verification-signals returns 400 for an invalid status', async () => {
    const branchId = await createSubmittedBranch();

    const response = await request(app.getHttpServer())
      .post(`/branches/${branchId}/verification-signals`)
      .set('X-Workspace-Id', WORKSPACE_ID)
      .send({ verifierName: 'ci-evaluator', status: 'unknown' });

    expect(response.status).toBe(400);
  });

  it('POST /branches/:id/verification-signals returns 400 for a blank verifierName', async () => {
    const branchId = await createSubmittedBranch();

    const response = await request(app.getHttpServer())
      .post(`/branches/${branchId}/verification-signals`)
      .set('X-Workspace-Id', WORKSPACE_ID)
      .send({ verifierName: '   ', status: 'pass' });

    expect(response.status).toBe(400);
  });

  it('GET /branches/:id/verification-signals returns all signals oldest-first with a valid X-Workspace-Id header', async () => {
    const branchId = await createSubmittedBranch();

    await request(app.getHttpServer())
      .post(`/branches/${branchId}/verification-signals`)
      .set('X-Workspace-Id', WORKSPACE_ID)
      .send({ verifierName: 'first-evaluator', status: 'pass' });
    await request(app.getHttpServer())
      .post(`/branches/${branchId}/verification-signals`)
      .set('X-Workspace-Id', WORKSPACE_ID)
      .send({ verifierName: 'second-evaluator', status: 'fail' });

    const response = await request(app.getHttpServer())
      .get(`/branches/${branchId}/verification-signals`)
      .set('X-Workspace-Id', WORKSPACE_ID);

    expect(response.status).toBe(200);
    const names = (response.body as { verifierName: string }[]).map((s) => s.verifierName);
    expect(names).toEqual(['first-evaluator', 'second-evaluator']);
  });

  it('POST /branches/:id/verification-signals returns 403 when the X-Workspace-Id header is missing', async () => {
    const branchId = await createSubmittedBranch();

    const response = await request(app.getHttpServer())
      .post(`/branches/${branchId}/verification-signals`)
      .send({ verifierName: 'ci-evaluator', status: 'pass' });

    expect(response.status).toBe(403);

    const row = await database.pool.query(
      'SELECT id FROM verification_signals WHERE branch_id = $1',
      [branchId],
    );
    expect(row.rows).toHaveLength(0);
  });

  it('POST /branches/:id/verification-signals returns 404 when X-Workspace-Id does not match the branch workspace', async () => {
    const branchId = await createSubmittedBranch();

    const response = await request(app.getHttpServer())
      .post(`/branches/${branchId}/verification-signals`)
      .set('X-Workspace-Id', '00000000-0000-0000-0000-00000000beef')
      .send({ verifierName: 'ci-evaluator', status: 'pass' });

    expect(response.status).toBe(404);

    const row = await database.pool.query(
      'SELECT id FROM verification_signals WHERE branch_id = $1',
      [branchId],
    );
    expect(row.rows).toHaveLength(0);
  });

  it('GET /branches/:id/verification-signals returns 403 when the X-Workspace-Id header is missing', async () => {
    const branchId = await createSubmittedBranch();

    const response = await request(app.getHttpServer()).get(
      `/branches/${branchId}/verification-signals`,
    );

    expect(response.status).toBe(403);
  });
});
