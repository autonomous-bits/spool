import type { INestApplication } from '@nestjs/common';
import { Test, type TestingModule } from '@nestjs/testing';
import request from 'supertest';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { AppModule } from '../src/app.module.js';
import { BOOTSTRAP_STAKEHOLDER_ID } from '../src/persistence/bootstrap-stakeholder.js';
import { runMigrations } from '../src/persistence/migrator.js';
import { loadDatabaseConfig } from '../src/persistence/database-config.js';
import { Pool } from 'pg';

describe('Chunks HTTP API (containerized Postgres)', () => {
  let app: INestApplication;
  let migrationPool: Pool;

  beforeAll(async () => {
    migrationPool = new Pool(loadDatabaseConfig());
    await runMigrations(migrationPool);

    const moduleRef: TestingModule = await Test.createTestingModule({
      imports: [AppModule],
    }).compile();

    app = moduleRef.createNestApplication();
    await app.init();
  });

  afterAll(async () => {
    await app.close();
    await migrationPool.end();
  });

  it('POST /chunks creates a chunk and GET /chunks/:id retrieves it', async () => {
    const createResponse = await request(app.getHttpServer())
      .post('/chunks')
      .send({
        label: `e2e-${Math.random().toString(36).slice(2, 10)}`,
        content: 'An atomic idea captured over HTTP.',
        discipline: 'engineering',
        chunkType: 'feature',
        contextKind: 'permanent',
        stakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
      });

    expect(createResponse.status).toBe(201);
    expect(createResponse.body.id).toBeTruthy();
    expect(createResponse.body.status).toBe('draft');

    const getResponse = await request(app.getHttpServer()).get(
      `/chunks/${createResponse.body.id as string}`,
    );

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
      .send({
        label: 'bad-vocab',
        content: 'content',
        discipline: 'bogus',
        chunkType: 'feature',
        contextKind: 'permanent',
        stakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
      });

    expect(response.status).toBe(400);
  });

  it('POST /chunks returns 400 for a missing required field', async () => {
    const response = await request(app.getHttpServer()).post('/chunks').send({
      content: 'content',
      discipline: 'engineering',
      chunkType: 'feature',
      contextKind: 'permanent',
      stakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
    });

    expect(response.status).toBe(400);
  });

  it('POST /chunks returns 400 for an unknown stakeholderId', async () => {
    const response = await request(app.getHttpServer())
      .post('/chunks')
      .send({
        label: `e2e-unknown-stakeholder-${Math.random().toString(36).slice(2, 10)}`,
        content: 'content',
        discipline: 'engineering',
        chunkType: 'feature',
        contextKind: 'permanent',
        stakeholderId: '00000000-0000-0000-0000-0000000000ff',
      });

    expect(response.status).toBe(400);
  });

  it('GET /chunks/:id returns 404 for an unknown id', async () => {
    const response = await request(app.getHttpServer()).get(
      '/chunks/00000000-0000-0000-0000-00000000dead',
    );

    expect(response.status).toBe(404);
  });
});
