import type { INestApplication } from '@nestjs/common';
import type { Server } from 'node:http';
import { Test, type TestingModule } from '@nestjs/testing';
import request from 'supertest';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { AppModule } from '../src/app.module.js';
import { BOOTSTRAP_STAKEHOLDER_ID } from '../src/persistence/bootstrap-stakeholder.js';
import { setUpTestDatabase, type TestDatabase } from './support/test-database.js';

describe('Artifacts HTTP API (containerized Postgres)', () => {
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

  function uniqueLabel(prefix: string): string {
    return `${prefix}-${Math.random().toString(36).slice(2, 10)}`;
  }

  async function createChunk(label: string, branchId?: string): Promise<string> {
    const response = await request(app.getHttpServer())
      .post('/chunks')
      .send({
        label,
        content: 'An atomic idea captured over HTTP.',
        discipline: 'engineering',
        chunkType: 'feature',
        contextKind: 'permanent',
        stakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
        ...(branchId === undefined ? {} : { branchId }),
      });
    expect(response.status).toBe(201);
    return response.body.label as string;
  }

  async function createBranch(): Promise<string> {
    const response = await request(app.getHttpServer())
      .post('/branches')
      .send({
        name: `e2e-branch-${Math.random().toString(36).slice(2, 10)}`,
        discipline: 'engineering',
        stakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
      });
    expect(response.status).toBe(201);
    return response.body.id as string;
  }

  async function createArtifact(content = 'hello artifact bytes'): Promise<string> {
    const response = await request(app.getHttpServer())
      .post('/artifacts')
      .send({
        content: Buffer.from(content).toString('base64'),
        mimeType: 'text/plain',
        stakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
      });
    expect(response.status).toBe(201);
    return response.body.id as string;
  }

  it('POST /artifacts returns 201 with artifact id + uri metadata only (no content echoed)', async () => {
    const response = await request(app.getHttpServer())
      .post('/artifacts')
      .send({
        content: Buffer.from('sample bytes').toString('base64'),
        mimeType: 'text/plain',
        stakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
      });

    expect(response.status).toBe(201);
    expect(response.body).toMatchObject({
      mimeType: 'text/plain',
      createdByStakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
    });
    expect(typeof response.body.id).toBe('string');
    expect(typeof response.body.uri).toBe('string');
    expect(response.body.content).toBeUndefined();
  });

  it('POST /artifacts returns 400 for malformed base64 content', async () => {
    const response = await request(app.getHttpServer()).post('/artifacts').send({
      content: 'not base64!!',
      mimeType: 'text/plain',
      stakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
    });

    expect(response.status).toBe(400);
  });

  it('POST /chunks/:label/artifacts returns 201 with the created mainline association', async () => {
    const label = await createChunk(uniqueLabel('e2e-chunk'));
    const artifactId = await createArtifact();

    const response = await request(app.getHttpServer())
      .post(`/chunks/${label}/artifacts`)
      .send({ artifactId, stakeholderId: BOOTSTRAP_STAKEHOLDER_ID });

    expect(response.status).toBe(201);
    expect(response.body).toMatchObject({
      chunkLabel: label,
      artifactId,
      status: 'active',
      branchId: null,
    });
  });

  it('POST /chunks/:label/artifacts returns 404 for an unknown chunk', async () => {
    const artifactId = await createArtifact();

    const response = await request(app.getHttpServer())
      .post(`/chunks/${uniqueLabel('e2e-nonexistent')}/artifacts`)
      .send({ artifactId, stakeholderId: BOOTSTRAP_STAKEHOLDER_ID });

    expect(response.status).toBe(404);
  });

  it('POST /chunks/:label/artifacts returns 404 for an unknown artifact', async () => {
    const label = await createChunk(uniqueLabel('e2e-chunk-unknown-artifact'));

    const response = await request(app.getHttpServer())
      .post(`/chunks/${label}/artifacts`)
      .send({
        artifactId: '00000000-0000-0000-0000-00000000dead',
        stakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
      });

    expect(response.status).toBe(404);
  });

  it('POST /chunks/:label/artifacts creates a branch-scoped association', async () => {
    const branchId = await createBranch();
    const label = await createChunk(uniqueLabel('e2e-branch-chunk'), branchId);
    const artifactId = await createArtifact();

    const response = await request(app.getHttpServer())
      .post(`/chunks/${label}/artifacts`)
      .send({ artifactId, stakeholderId: BOOTSTRAP_STAKEHOLDER_ID, branchId });

    expect(response.status).toBe(201);
    expect(response.body).toMatchObject({
      chunkLabel: label,
      artifactId,
      status: 'active',
      branchId,
      originBranchId: branchId,
    });
  });

  it('DELETE .../artifacts/:artifactId inserts a deactivated row for a branch-scoped association', async () => {
    const branchId = await createBranch();
    const label = await createChunk(uniqueLabel('e2e-detach-chunk'), branchId);
    const artifactId = await createArtifact();

    await request(app.getHttpServer())
      .post(`/chunks/${label}/artifacts`)
      .send({ artifactId, stakeholderId: BOOTSTRAP_STAKEHOLDER_ID, branchId })
      .expect(201);

    const response = await request(app.getHttpServer())
      .delete(`/chunks/${label}/artifacts/${artifactId}`)
      .query({ branchId, stakeholderId: BOOTSTRAP_STAKEHOLDER_ID });

    expect(response.status).toBe(201);
    expect(response.body).toMatchObject({
      chunkLabel: label,
      artifactId,
      status: 'deactivated',
      branchId,
    });
  });

  it('DELETE .../artifacts/:artifactId returns 400 when branchId query parameter is omitted', async () => {
    const label = await createChunk(uniqueLabel('e2e-detach-missing-branch'));
    const artifactId = await createArtifact();

    const response = await request(app.getHttpServer())
      .delete(`/chunks/${label}/artifacts/${artifactId}`)
      .query({ stakeholderId: BOOTSTRAP_STAKEHOLDER_ID });

    expect(response.status).toBe(400);
  });

  it('GET /chunks/:label/artifacts?branchId= returns the mainline-effective tuple with no branch overlay', async () => {
    const label = await createChunk(uniqueLabel('e2e-effective-mainline'));
    const artifactId = await createArtifact();
    await request(app.getHttpServer())
      .post(`/chunks/${label}/artifacts`)
      .send({ artifactId, stakeholderId: BOOTSTRAP_STAKEHOLDER_ID })
      .expect(201);

    const response = await request(app.getHttpServer()).get(`/chunks/${label}/artifacts`);

    expect(response.status).toBe(200);
    expect(response.body).toEqual([{ artifactId, branchId: null, status: 'active' }]);
  });

  it('GET /chunks/:label/artifacts?branchId= returns the branch-overlaid tuple when a branch-scoped association exists', async () => {
    const branchId = await createBranch();
    const label = await createChunk(uniqueLabel('e2e-effective-branch'), branchId);
    const artifactId = await createArtifact();
    await request(app.getHttpServer())
      .post(`/chunks/${label}/artifacts`)
      .send({ artifactId, stakeholderId: BOOTSTRAP_STAKEHOLDER_ID, branchId })
      .expect(201);

    const response = await request(app.getHttpServer())
      .get(`/chunks/${label}/artifacts`)
      .query({ branchId });

    expect(response.status).toBe(200);
    expect(response.body).toEqual([{ artifactId, branchId, status: 'active' }]);
  });

  it('GET /chunks/:label/artifacts returns 404 for an unknown chunk', async () => {
    const response = await request(app.getHttpServer()).get(
      `/chunks/${uniqueLabel('e2e-effective-nonexistent')}/artifacts`,
    );

    expect(response.status).toBe(404);
  });

  it('GET /artifacts/:id/download-token issues a token that expires', async () => {
    const artifactId = await createArtifact();

    const response = await request(app.getHttpServer()).get(
      `/artifacts/${artifactId}/download-token`,
    );

    expect(response.status).toBe(200);
    expect(typeof response.body.token).toBe('string');
    expect(new Date(response.body.expiresAt).getTime()).toBeGreaterThan(Date.now());
  });

  it('GET /artifacts/:id/download-token returns 404 for an unknown artifact', async () => {
    const response = await request(app.getHttpServer()).get(
      '/artifacts/00000000-0000-0000-0000-00000000dead/download-token',
    );

    expect(response.status).toBe(404);
  });

  it('GET /artifacts/content/:token streams back the exact originally-uploaded bytes with the correct Content-Type', async () => {
    const content = 'exact original bytes for signed download';
    const artifactId = await createArtifact(content);
    const tokenResponse = await request(app.getHttpServer())
      .get(`/artifacts/${artifactId}/download-token`)
      .expect(200);

    const response = await request(app.getHttpServer()).get(
      `/artifacts/content/${String(tokenResponse.body.token)}`,
    );

    expect(response.status).toBe(200);
    expect(response.headers['content-type']).toContain('text/plain');
    expect(response.text).toBe(content);
  });

  it('GET /artifacts/content/:token rejects a tampered token', async () => {
    const artifactId = await createArtifact();
    const tokenResponse = await request(app.getHttpServer())
      .get(`/artifacts/${artifactId}/download-token`)
      .expect(200);
    const [payload] = (tokenResponse.body.token as string).split('.');
    const tamperedToken = `${String(payload)}.tampered-signature`;

    const response = await request(app.getHttpServer()).get(
      `/artifacts/content/${tamperedToken}`,
    );

    expect(response.status).toBe(401);
  });

  it('GET /artifacts/content/:token rejects a structurally invalid token', async () => {
    const response = await request(app.getHttpServer()).get('/artifacts/content/not-a-token');

    expect(response.status).toBe(401);
  });
});
