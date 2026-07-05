import type { INestApplication } from '@nestjs/common';
import { Test, type TestingModule } from '@nestjs/testing';
import request from 'supertest';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { AppModule } from '../src/app.module.js';
import { BOOTSTRAP_STAKEHOLDER_ID } from '../src/persistence/bootstrap-stakeholder.js';
import { setUpTestDatabase, type TestDatabase } from './support/test-database.js';

describe('Branches HTTP API (containerized Postgres)', () => {
  let app: INestApplication;
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
    await app?.close();
    await database?.close();
  });

  it('POST /branches creates a draft branch and GET /branches/:id retrieves it', async () => {
    const createResponse = await request(app.getHttpServer())
      .post('/branches')
      .send({
        name: `e2e-branch-${Math.random().toString(36).slice(2, 10)}`,
        discipline: 'engineering',
        stakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
      });

    expect(createResponse.status).toBe(201);
    expect(createResponse.body.id).toBeTruthy();
    expect(createResponse.body.status).toBe('draft');
    expect(createResponse.body.createdByStakeholderId).toBe(BOOTSTRAP_STAKEHOLDER_ID);

    const getResponse = await request(app.getHttpServer()).get(
      `/branches/${createResponse.body.id as string}`,
    );

    expect(getResponse.status).toBe(200);
    expect(getResponse.body).toMatchObject({
      id: createResponse.body.id,
      discipline: 'engineering',
      status: 'draft',
    });
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
        name: `e2e-unknown-stakeholder-${Math.random().toString(36).slice(2, 10)}`,
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
