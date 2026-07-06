import { randomUUID } from 'node:crypto';
import type { INestApplication } from '@nestjs/common';
import { Test, type TestingModule } from '@nestjs/testing';
import request from 'supertest';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { AppModule } from '../src/app.module.js';
import { SessionTokenService } from '../src/auth/session-token.service.js';
import { BOOTSTRAP_STAKEHOLDER_ID } from '../src/persistence/bootstrap-stakeholder.js';
import { setUpTestDatabase, type TestDatabase } from './support/test-database.js';

function uniqueName(prefix: string): string {
  return `${prefix}-${randomUUID()}`;
}

describe('Branches HTTP API (containerized Postgres)', () => {
  let app: INestApplication;
  let database: TestDatabase;
  let sessionTokenService: SessionTokenService;
  let engineeringStakeholderId: string;
  let productStakeholderId: string;
  let nullDisciplineStakeholderId: string;

  beforeAll(async () => {
    database = await setUpTestDatabase();

    engineeringStakeholderId = randomUUID();
    productStakeholderId = randomUUID();
    nullDisciplineStakeholderId = randomUUID();

    await database.pool.query(
      `INSERT INTO stakeholders (id, name, email, role, discipline, github_login)
       VALUES
         ($1, 'Engineering Stakeholder', $2, 'stakeholder', 'engineering', $3),
         ($4, 'Product Stakeholder', $5, 'stakeholder', 'product', $6),
         ($7, 'No Discipline Stakeholder', $8, 'stakeholder', NULL, $9)`,
      [
        engineeringStakeholderId,
        `engineering-${engineeringStakeholderId}@spool.local`,
        `engineering-${engineeringStakeholderId}`,
        productStakeholderId,
        `product-${productStakeholderId}@spool.local`,
        `product-${productStakeholderId}`,
        nullDisciplineStakeholderId,
        `null-discipline-${nullDisciplineStakeholderId}@spool.local`,
        `null-discipline-${nullDisciplineStakeholderId}`,
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
    await app?.close();
    await database?.close();
  });

  function mintSessionToken(stakeholderId: string, discipline: string | null): string {
    return sessionTokenService.sign({
      stakeholderId,
      discipline,
      authTime: Math.floor(Date.now() / 1000),
    });
  }

  async function createBranch(discipline: 'engineering' | 'product', stakeholderId: string): Promise<string> {
    const createResponse = await request(app.getHttpServer())
      .post('/branches')
      .send({
        name: uniqueName('e2e-branch'),
        discipline,
        stakeholderId,
      });

    expect(createResponse.status).toBe(201);
    return createResponse.body.id as string;
  }

  async function createSubmittedBranch(
    discipline: 'engineering' | 'product',
    stakeholderId: string,
  ): Promise<string> {
    const branchId = await createBranch(discipline, stakeholderId);
    const token = mintSessionToken(stakeholderId, discipline);

    const submitResponse = await request(app.getHttpServer())
      .post(`/branches/${branchId}/submit`)
      .set('Authorization', `Bearer ${token}`);

    expect(submitResponse.status).toBe(201);
    return branchId;
  }

  async function createVerifiedBranch(
    discipline: 'engineering' | 'product',
    stakeholderId: string,
  ): Promise<string> {
    const branchId = await createSubmittedBranch(discipline, stakeholderId);
    const verifyToken = mintSessionToken(productStakeholderId, 'product');

    const verifyResponse = await request(app.getHttpServer())
      .post(`/branches/${branchId}/verify`)
      .set('Authorization', `Bearer ${verifyToken}`);

    expect(verifyResponse.status).toBe(201);
    return branchId;
  }

  it('POST /branches creates a draft branch and GET /branches/:id retrieves it', async () => {
    const createResponse = await request(app.getHttpServer())
      .post('/branches')
      .send({
        name: uniqueName('e2e-branch'),
        discipline: 'engineering',
        stakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
      });

    expect(createResponse.status).toBe(201);
    expect(createResponse.body.id).toBeTruthy();
    expect(createResponse.body.status).toBe('draft');
    expect(createResponse.body.submittedAt).toBeNull();
    expect(createResponse.body.createdByStakeholderId).toBe(BOOTSTRAP_STAKEHOLDER_ID);

    const getResponse = await request(app.getHttpServer()).get(
      `/branches/${createResponse.body.id as string}`,
    );

    expect(getResponse.status).toBe(200);
    expect(getResponse.body).toMatchObject({
      id: createResponse.body.id,
      discipline: 'engineering',
      status: 'draft',
      submittedAt: null,
    });
  });

  it('POST /branches/:id/submit submits a draft branch using only the bearer token actor context and surfaces submittedAt on POST + GET', async () => {
    const branchId = await createBranch('engineering', engineeringStakeholderId);
    const token = mintSessionToken(engineeringStakeholderId, 'engineering');

    const submitResponse = await request(app.getHttpServer())
      .post(`/branches/${branchId}/submit`)
      .set('Authorization', `Bearer ${token}`)
      .send({ actorKind: 'delegated', discipline: 'security' });

    expect(submitResponse.status).toBe(201);
    expect(submitResponse.body).toMatchObject({
      id: branchId,
      discipline: 'engineering',
      status: 'submitted',
    });
    expect(typeof submitResponse.body.submittedAt).toBe('string');

    const getResponse = await request(app.getHttpServer()).get(`/branches/${branchId}`);

    expect(getResponse.status).toBe(200);
    expect(getResponse.body.status).toBe('submitted');
    expect(getResponse.body.submittedAt).toBe(submitResponse.body.submittedAt);
  });

  it('POST /branches/:id/submit returns 401 when Authorization is missing', async () => {
    const branchId = await createBranch('engineering', engineeringStakeholderId);

    const response = await request(app.getHttpServer()).post(`/branches/${branchId}/submit`);

    expect(response.status).toBe(401);
  });

  it('POST /branches/:id/submit returns 401 for a malformed Authorization header', async () => {
    const branchId = await createBranch('engineering', engineeringStakeholderId);

    const response = await request(app.getHttpServer())
      .post(`/branches/${branchId}/submit`)
      .set('Authorization', 'Token not-a-bearer-token');

    expect(response.status).toBe(401);
  });

  it('POST /branches/:id/submit returns 401 when session-token verification rejects the bearer token', async () => {
    const branchId = await createBranch('engineering', engineeringStakeholderId);

    const response = await request(app.getHttpServer())
      .post(`/branches/${branchId}/submit`)
      .set('Authorization', 'Bearer definitely-not-a-valid-token');

    expect(response.status).toBe(401);
  });

  it('POST /branches/:id/submit returns 404 for an unknown branch id', async () => {
    const token = mintSessionToken(engineeringStakeholderId, 'engineering');

    const response = await request(app.getHttpServer())
      .post('/branches/00000000-0000-0000-0000-00000000dead/submit')
      .set('Authorization', `Bearer ${token}`);

    expect(response.status).toBe(404);
  });

  it('POST /branches/:id/submit returns 400 when the token stakeholder does not resolve', async () => {
    const branchId = await createBranch('engineering', engineeringStakeholderId);
    const token = mintSessionToken('00000000-0000-0000-0000-00000000beef', 'engineering');

    const response = await request(app.getHttpServer())
      .post(`/branches/${branchId}/submit`)
      .set('Authorization', `Bearer ${token}`);

    expect(response.status).toBe(400);
  });

  it('POST /branches/:id/submit returns 400 when the resolved stakeholder discipline is null', async () => {
    const branchId = await createBranch('engineering', engineeringStakeholderId);
    const token = mintSessionToken(nullDisciplineStakeholderId, null);

    const response = await request(app.getHttpServer())
      .post(`/branches/${branchId}/submit`)
      .set('Authorization', `Bearer ${token}`);

    expect(response.status).toBe(400);
  });

  it('POST /branches/:id/submit returns 409 when the stakeholder discipline mismatches the branch discipline', async () => {
    const branchId = await createBranch('engineering', engineeringStakeholderId);
    const token = mintSessionToken(productStakeholderId, 'product');

    const response = await request(app.getHttpServer())
      .post(`/branches/${branchId}/submit`)
      .set('Authorization', `Bearer ${token}`);

    expect(response.status).toBe(409);
  });

  it('POST /branches/:id/submit returns 409 when the branch is no longer in draft status', async () => {
    const branchId = await createBranch('engineering', engineeringStakeholderId);
    const token = mintSessionToken(engineeringStakeholderId, 'engineering');

    const firstResponse = await request(app.getHttpServer())
      .post(`/branches/${branchId}/submit`)
      .set('Authorization', `Bearer ${token}`);
    const repeatedResponse = await request(app.getHttpServer())
      .post(`/branches/${branchId}/submit`)
      .set('Authorization', `Bearer ${token}`);

    expect(firstResponse.status).toBe(201);
    expect(repeatedResponse.status).toBe(409);
  });

  it('POST /branches/:id/verify verifies a submitted branch with a cross-discipline stakeholder token and surfaces verifiedAt', async () => {
    const branchId = await createSubmittedBranch('engineering', engineeringStakeholderId);
    const token = mintSessionToken(productStakeholderId, 'product');

    const verifyResponse = await request(app.getHttpServer())
      .post(`/branches/${branchId}/verify`)
      .set('Authorization', `Bearer ${token}`);

    expect(verifyResponse.status).toBe(201);
    expect(verifyResponse.body.status).toBe('verified');
    expect(typeof verifyResponse.body.verifiedAt).toBe('string');

    const getResponse = await request(app.getHttpServer()).get(`/branches/${branchId}`);
    expect(getResponse.body.status).toBe('verified');
    expect(getResponse.body.verifiedAt).toBe(verifyResponse.body.verifiedAt);
  });

  it('POST /branches/:id/verify succeeds for a stakeholder with a null discipline', async () => {
    const branchId = await createSubmittedBranch('engineering', engineeringStakeholderId);
    const token = mintSessionToken(nullDisciplineStakeholderId, null);

    const verifyResponse = await request(app.getHttpServer())
      .post(`/branches/${branchId}/verify`)
      .set('Authorization', `Bearer ${token}`);

    expect(verifyResponse.status).toBe(201);
    expect(verifyResponse.body.status).toBe('verified');
  });

  it('POST /branches/:id/verify returns 401 when Authorization is missing', async () => {
    const branchId = await createSubmittedBranch('engineering', engineeringStakeholderId);

    const response = await request(app.getHttpServer()).post(`/branches/${branchId}/verify`);

    expect(response.status).toBe(401);
  });

  it('POST /branches/:id/verify returns 404 for an unknown branch id', async () => {
    const token = mintSessionToken(engineeringStakeholderId, 'engineering');

    const response = await request(app.getHttpServer())
      .post('/branches/00000000-0000-0000-0000-00000000dead/verify')
      .set('Authorization', `Bearer ${token}`);

    expect(response.status).toBe(404);
  });

  it('POST /branches/:id/verify returns 400 when the token stakeholder does not resolve', async () => {
    const branchId = await createSubmittedBranch('engineering', engineeringStakeholderId);
    const token = mintSessionToken('00000000-0000-0000-0000-00000000beef', 'engineering');

    const response = await request(app.getHttpServer())
      .post(`/branches/${branchId}/verify`)
      .set('Authorization', `Bearer ${token}`);

    expect(response.status).toBe(400);
  });

  it('POST /branches/:id/verify returns 409 when the branch is not submitted', async () => {
    const branchId = await createBranch('engineering', engineeringStakeholderId);
    const token = mintSessionToken(productStakeholderId, 'product');

    const response = await request(app.getHttpServer())
      .post(`/branches/${branchId}/verify`)
      .set('Authorization', `Bearer ${token}`);

    expect(response.status).toBe(409);
  });

  it('POST /branches/:id/verify returns 409 when the branch was already verified', async () => {
    const branchId = await createSubmittedBranch('engineering', engineeringStakeholderId);
    const token = mintSessionToken(productStakeholderId, 'product');

    const firstResponse = await request(app.getHttpServer())
      .post(`/branches/${branchId}/verify`)
      .set('Authorization', `Bearer ${token}`);
    const repeatedResponse = await request(app.getHttpServer())
      .post(`/branches/${branchId}/verify`)
      .set('Authorization', `Bearer ${token}`);

    expect(firstResponse.status).toBe(201);
    expect(repeatedResponse.status).toBe(409);
  });

  it('POST /branches/:id/reject resets a submitted branch to draft, clearing verifiedAt and submittedAt', async () => {
    const branchId = await createSubmittedBranch('engineering', engineeringStakeholderId);
    const token = mintSessionToken(productStakeholderId, 'product');

    const rejectResponse = await request(app.getHttpServer())
      .post(`/branches/${branchId}/reject`)
      .set('Authorization', `Bearer ${token}`);

    expect(rejectResponse.status).toBe(201);
    expect(rejectResponse.body.status).toBe('draft');
    expect(rejectResponse.body.verifiedAt).toBeNull();
    expect(rejectResponse.body.submittedAt).toBeNull();

    const getResponse = await request(app.getHttpServer()).get(`/branches/${branchId}`);
    expect(getResponse.body.status).toBe('draft');
    expect(getResponse.body.verifiedAt).toBeNull();
    expect(getResponse.body.submittedAt).toBeNull();
  });

  it('POST /branches/:id/reject resets a verified branch to draft', async () => {
    const branchId = await createSubmittedBranch('engineering', engineeringStakeholderId);
    const verifyToken = mintSessionToken(productStakeholderId, 'product');
    await request(app.getHttpServer())
      .post(`/branches/${branchId}/verify`)
      .set('Authorization', `Bearer ${verifyToken}`);

    const rejectResponse = await request(app.getHttpServer())
      .post(`/branches/${branchId}/reject`)
      .set('Authorization', `Bearer ${verifyToken}`);

    expect(rejectResponse.status).toBe(201);
    expect(rejectResponse.body.status).toBe('draft');
    expect(rejectResponse.body.verifiedAt).toBeNull();
    expect(rejectResponse.body.submittedAt).toBeNull();
  });

  it('POST /branches/:id/reject returns 409 when the branch is a draft', async () => {
    const branchId = await createBranch('engineering', engineeringStakeholderId);
    const token = mintSessionToken(productStakeholderId, 'product');

    const response = await request(app.getHttpServer())
      .post(`/branches/${branchId}/reject`)
      .set('Authorization', `Bearer ${token}`);

    expect(response.status).toBe(409);
  });

  it('POST /branches/:id/reject returns 401 when Authorization is missing', async () => {
    const branchId = await createSubmittedBranch('engineering', engineeringStakeholderId);

    const response = await request(app.getHttpServer()).post(`/branches/${branchId}/reject`);

    expect(response.status).toBe(401);
  });

  it('POST /branches/:id/reject returns 404 for an unknown branch id', async () => {
    const token = mintSessionToken(engineeringStakeholderId, 'engineering');

    const response = await request(app.getHttpServer())
      .post('/branches/00000000-0000-0000-0000-00000000dead/reject')
      .set('Authorization', `Bearer ${token}`);

    expect(response.status).toBe(404);
  });

  it('POST /branches/:id/merge merges a verified branch and returns mergedAt', async () => {
    const branchId = await createVerifiedBranch('engineering', engineeringStakeholderId);
    const token = mintSessionToken(productStakeholderId, 'product');

    const mergeResponse = await request(app.getHttpServer())
      .post(`/branches/${branchId}/merge`)
      .set('Authorization', `Bearer ${token}`);

    expect(mergeResponse.status).toBe(201);
    expect(mergeResponse.body.status).toBe('merged');
    expect(typeof mergeResponse.body.mergedAt).toBe('string');

    const getResponse = await request(app.getHttpServer()).get(`/branches/${branchId}`);
    expect(getResponse.body.status).toBe('merged');
    expect(getResponse.body.mergedAt).toBe(mergeResponse.body.mergedAt);
  });

  it('POST /branches/:id/merge is discipline-agnostic (any human stakeholder can merge)', async () => {
    const branchId = await createVerifiedBranch('engineering', engineeringStakeholderId);
    const token = mintSessionToken(nullDisciplineStakeholderId, null);

    const mergeResponse = await request(app.getHttpServer())
      .post(`/branches/${branchId}/merge`)
      .set('Authorization', `Bearer ${token}`);

    expect(mergeResponse.status).toBe(201);
    expect(mergeResponse.body.status).toBe('merged');
  });

  it('POST /branches/:id/merge returns 409 when the branch is not verified', async () => {
    const branchId = await createSubmittedBranch('engineering', engineeringStakeholderId);
    const token = mintSessionToken(productStakeholderId, 'product');

    const response = await request(app.getHttpServer())
      .post(`/branches/${branchId}/merge`)
      .set('Authorization', `Bearer ${token}`);

    expect(response.status).toBe(409);
  });

  it('POST /branches/:id/merge returns 401 when Authorization is missing', async () => {
    const branchId = await createVerifiedBranch('engineering', engineeringStakeholderId);

    const response = await request(app.getHttpServer()).post(`/branches/${branchId}/merge`);

    expect(response.status).toBe(401);
  });

  it('POST /branches/:id/merge returns 404 for an unknown branch id', async () => {
    const token = mintSessionToken(engineeringStakeholderId, 'engineering');

    const response = await request(app.getHttpServer())
      .post('/branches/00000000-0000-0000-0000-00000000dead/merge')
      .set('Authorization', `Bearer ${token}`);

    expect(response.status).toBe(404);
  });

  it('POST /branches returns 400 for an invalid discipline', async () => {
    const response = await request(app.getHttpServer())
      .post('/branches')
      .send({
        name: 'bad-vocab-branch',
        discipline: 'bogus',
        stakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
      });

    expect(response.status).toBe(400);
  });

  it('POST /branches returns 400 for a missing required field', async () => {
    const response = await request(app.getHttpServer()).post('/branches').send({
      discipline: 'engineering',
      stakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
    });

    expect(response.status).toBe(400);
  });

  it('POST /branches returns 400 for an unknown stakeholderId', async () => {
    const response = await request(app.getHttpServer())
      .post('/branches')
      .send({
        name: uniqueName('e2e-unknown-stakeholder'),
        discipline: 'engineering',
        stakeholderId: '00000000-0000-0000-0000-0000000000ff',
      });

    expect(response.status).toBe(400);
  });

  it('GET /branches/:id returns 404 for an unknown id', async () => {
    const response = await request(app.getHttpServer()).get(
      '/branches/00000000-0000-0000-0000-00000000dead',
    );

    expect(response.status).toBe(404);
  });
});
