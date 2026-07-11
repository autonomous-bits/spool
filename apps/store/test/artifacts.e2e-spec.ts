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

describe('Artifacts HTTP API (containerized Postgres)', () => {
  let app: INestApplication<Server>;
  let database: TestDatabase;
  let sessionTokenService: SessionTokenService;

  beforeAll(async () => {
    database = await setUpTestDatabase();

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

  function uniqueLabel(prefix: string): string {
    return `${prefix}-${Math.random().toString(36).slice(2, 10)}`;
  }

  function mintSessionToken(
    stakeholderId = BOOTSTRAP_STAKEHOLDER_ID,
    discipline: string | null = 'engineering',
  ): string {
    return sessionTokenService.sign({
      stakeholderId,
      discipline,
      authTime: Math.floor(Date.now() / 1000),
      workspaceId: WORKSPACE_ID,
    });
  }

  async function createChunk(label: string, branchId?: string): Promise<string> {
    const token = mintSessionToken();
    const response = await request(app.getHttpServer())
      .post('/chunks')
      .set('Authorization', `Bearer ${token}`)
      .set('X-Workspace-Id', WORKSPACE_ID)
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
    const token = mintSessionToken();
    const response = await request(app.getHttpServer())
      .post('/branches')
      .set('Authorization', `Bearer ${token}`)
      .set('X-Workspace-Id', WORKSPACE_ID)
      .send({
        name: `e2e-branch-${Math.random().toString(36).slice(2, 10)}`,
        discipline: 'engineering',
        stakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
      });
    expect(response.status).toBe(201);
    return response.body.id as string;
  }

  async function createArtifact(content = 'hello artifact bytes'): Promise<string> {
    const token = mintSessionToken();
    const response = await request(app.getHttpServer())
      .post('/artifacts')
      .set('Authorization', `Bearer ${token}`)
      .set('X-Workspace-Id', WORKSPACE_ID)
      .send({
        content: Buffer.from(content).toString('base64'),
        mimeType: 'text/plain',
        stakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
      });
    expect(response.status).toBe(201);
    return response.body.id as string;
  }

  it('POST /artifacts returns 201 with artifact id + uri metadata only (no content echoed)', async () => {
    const token = mintSessionToken();
    const response = await request(app.getHttpServer())
      .post('/artifacts')
      .set('Authorization', `Bearer ${token}`)
      .set('X-Workspace-Id', WORKSPACE_ID)
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
    const token = mintSessionToken();
    const response = await request(app.getHttpServer())
      .post('/artifacts')
      .set('Authorization', `Bearer ${token}`)
      .set('X-Workspace-Id', WORKSPACE_ID)
      .send({
        content: 'not base64!!',
        mimeType: 'text/plain',
        stakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
      });

    expect(response.status).toBe(400);
  });

  it('POST /artifacts returns 403 when the X-Workspace-Id header is missing', async () => {
    const token = mintSessionToken();
    const response = await request(app.getHttpServer())
      .post('/artifacts')
      .set('Authorization', `Bearer ${token}`)
      .send({
        content: Buffer.from('sample bytes').toString('base64'),
        mimeType: 'text/plain',
        stakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
      });

    expect(response.status).toBe(403);
  });

  it('POST /chunks/:label/artifacts returns 201 with the created mainline association', async () => {
    const label = await createChunk(uniqueLabel('e2e-chunk'));
    const artifactId = await createArtifact();
    const token = mintSessionToken();

    const response = await request(app.getHttpServer())
      .post(`/chunks/${label}/artifacts`)
      .set('Authorization', `Bearer ${token}`)
      .set('X-Workspace-Id', WORKSPACE_ID)
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
    const token = mintSessionToken();

    const response = await request(app.getHttpServer())
      .post(`/chunks/${uniqueLabel('e2e-nonexistent')}/artifacts`)
      .set('Authorization', `Bearer ${token}`)
      .set('X-Workspace-Id', WORKSPACE_ID)
      .send({ artifactId, stakeholderId: BOOTSTRAP_STAKEHOLDER_ID });

    expect(response.status).toBe(404);
  });

  it('POST /chunks/:label/artifacts returns 404 for an unknown artifact', async () => {
    const label = await createChunk(uniqueLabel('e2e-chunk-unknown-artifact'));
    const token = mintSessionToken();

    const response = await request(app.getHttpServer())
      .post(`/chunks/${label}/artifacts`)
      .set('Authorization', `Bearer ${token}`)
      .set('X-Workspace-Id', WORKSPACE_ID)
      .send({
        artifactId: '00000000-0000-0000-0000-00000000dead',
        stakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
      });

    expect(response.status).toBe(404);
  });

  it('POST /chunks/:label/artifacts returns 403 when the X-Workspace-Id header is missing', async () => {
    const label = await createChunk(uniqueLabel('e2e-chunk-no-header'));
    const artifactId = await createArtifact();
    const token = mintSessionToken();

    const response = await request(app.getHttpServer())
      .post(`/chunks/${label}/artifacts`)
      .set('Authorization', `Bearer ${token}`)
      .send({ artifactId, stakeholderId: BOOTSTRAP_STAKEHOLDER_ID });

    expect(response.status).toBe(403);
  });

  it('POST /chunks/:label/artifacts creates a branch-scoped association', async () => {
    const branchId = await createBranch();
    const label = await createChunk(uniqueLabel('e2e-branch-chunk'), branchId);
    const artifactId = await createArtifact();
    const token = mintSessionToken();

    const response = await request(app.getHttpServer())
      .post(`/chunks/${label}/artifacts`)
      .set('Authorization', `Bearer ${token}`)
      .set('X-Workspace-Id', WORKSPACE_ID)
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
    const token = mintSessionToken();

    await request(app.getHttpServer())
      .post(`/chunks/${label}/artifacts`)
      .set('Authorization', `Bearer ${token}`)
      .set('X-Workspace-Id', WORKSPACE_ID)
      .send({ artifactId, stakeholderId: BOOTSTRAP_STAKEHOLDER_ID, branchId })
      .expect(201);

    const response = await request(app.getHttpServer())
      .delete(`/chunks/${label}/artifacts/${artifactId}`)
      .set('Authorization', `Bearer ${token}`)
      .set('X-Workspace-Id', WORKSPACE_ID)
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
    const token = mintSessionToken();

    const response = await request(app.getHttpServer())
      .delete(`/chunks/${label}/artifacts/${artifactId}`)
      .set('Authorization', `Bearer ${token}`)
      .set('X-Workspace-Id', WORKSPACE_ID)
      .query({ stakeholderId: BOOTSTRAP_STAKEHOLDER_ID });

    expect(response.status).toBe(400);
  });

  it('DELETE .../artifacts/:artifactId returns 403 when the X-Workspace-Id header is missing', async () => {
    const branchId = await createBranch();
    const label = await createChunk(uniqueLabel('e2e-detach-no-header'), branchId);
    const artifactId = await createArtifact();
    const token = mintSessionToken();
    await request(app.getHttpServer())
      .post(`/chunks/${label}/artifacts`)
      .set('Authorization', `Bearer ${token}`)
      .set('X-Workspace-Id', WORKSPACE_ID)
      .send({ artifactId, stakeholderId: BOOTSTRAP_STAKEHOLDER_ID, branchId })
      .expect(201);

    const response = await request(app.getHttpServer())
      .delete(`/chunks/${label}/artifacts/${artifactId}`)
      .set('Authorization', `Bearer ${token}`)
      .query({ branchId, stakeholderId: BOOTSTRAP_STAKEHOLDER_ID });

    expect(response.status).toBe(403);
  });

  it('GET /chunks/:label/artifacts?branchId= returns the mainline-effective tuple with no branch overlay', async () => {
    const label = await createChunk(uniqueLabel('e2e-effective-mainline'));
    const artifactId = await createArtifact();
    const token = mintSessionToken();
    await request(app.getHttpServer())
      .post(`/chunks/${label}/artifacts`)
      .set('Authorization', `Bearer ${token}`)
      .set('X-Workspace-Id', WORKSPACE_ID)
      .send({ artifactId, stakeholderId: BOOTSTRAP_STAKEHOLDER_ID })
      .expect(201);

    const response = await request(app.getHttpServer())
      .get(`/chunks/${label}/artifacts`)
      .set('Authorization', `Bearer ${token}`)
      .set('X-Workspace-Id', WORKSPACE_ID)
      .query({ stakeholderId: BOOTSTRAP_STAKEHOLDER_ID });

    expect(response.status).toBe(200);
    expect(response.body).toEqual([{ artifactId, branchId: null, status: 'active' }]);
  });

  it('GET /chunks/:label/artifacts?branchId= returns the branch-overlaid tuple when a branch-scoped association exists', async () => {
    const branchId = await createBranch();
    const label = await createChunk(uniqueLabel('e2e-effective-branch'), branchId);
    const artifactId = await createArtifact();
    const token = mintSessionToken();
    await request(app.getHttpServer())
      .post(`/chunks/${label}/artifacts`)
      .set('Authorization', `Bearer ${token}`)
      .set('X-Workspace-Id', WORKSPACE_ID)
      .send({ artifactId, stakeholderId: BOOTSTRAP_STAKEHOLDER_ID, branchId })
      .expect(201);

    const response = await request(app.getHttpServer())
      .get(`/chunks/${label}/artifacts`)
      .set('Authorization', `Bearer ${token}`)
      .set('X-Workspace-Id', WORKSPACE_ID)
      .query({ branchId, stakeholderId: BOOTSTRAP_STAKEHOLDER_ID });

    expect(response.status).toBe(200);
    expect(response.body).toEqual([{ artifactId, branchId, status: 'active' }]);
  });

  it('GET /chunks/:label/artifacts returns 404 for an unknown chunk', async () => {
    const token = mintSessionToken();
    const response = await request(app.getHttpServer())
      .get(`/chunks/${uniqueLabel('e2e-effective-nonexistent')}/artifacts`)
      .set('Authorization', `Bearer ${token}`)
      .set('X-Workspace-Id', WORKSPACE_ID)
      .query({ stakeholderId: BOOTSTRAP_STAKEHOLDER_ID });

    expect(response.status).toBe(404);
  });

  it('GET /chunks/:label/artifacts returns 403 when the X-Workspace-Id header is missing', async () => {
    const label = await createChunk(uniqueLabel('e2e-effective-no-header'));
    const token = mintSessionToken();

    const response = await request(app.getHttpServer())
      .get(`/chunks/${label}/artifacts`)
      .set('Authorization', `Bearer ${token}`)
      .query({ stakeholderId: BOOTSTRAP_STAKEHOLDER_ID });

    expect(response.status).toBe(403);
  });

  it('GET /artifacts/:id/download-token issues a token that expires', async () => {
    const artifactId = await createArtifact();
    const token = mintSessionToken();

    const response = await request(app.getHttpServer())
      .get(`/artifacts/${artifactId}/download-token`)
      .set('Authorization', `Bearer ${token}`)
      .set('X-Workspace-Id', WORKSPACE_ID)
      .query({ stakeholderId: BOOTSTRAP_STAKEHOLDER_ID });

    expect(response.status).toBe(200);
    expect(typeof response.body.token).toBe('string');
    expect(new Date(response.body.expiresAt).getTime()).toBeGreaterThan(Date.now());
  });

  it('GET /artifacts/:id/download-token returns 404 for an unknown artifact', async () => {
    const token = mintSessionToken();
    const response = await request(app.getHttpServer())
      .get('/artifacts/00000000-0000-0000-0000-00000000dead/download-token')
      .set('Authorization', `Bearer ${token}`)
      .set('X-Workspace-Id', WORKSPACE_ID)
      .query({ stakeholderId: BOOTSTRAP_STAKEHOLDER_ID });

    expect(response.status).toBe(404);
  });

  it('GET /artifacts/:id/download-token returns 403 when the X-Workspace-Id header is missing', async () => {
    const artifactId = await createArtifact();
    const token = mintSessionToken();

    const response = await request(app.getHttpServer())
      .get(`/artifacts/${artifactId}/download-token`)
      .set('Authorization', `Bearer ${token}`)
      .query({ stakeholderId: BOOTSTRAP_STAKEHOLDER_ID });

    expect(response.status).toBe(403);
  });

  it('GET /artifacts/content/:token streams back the exact originally-uploaded bytes with the correct Content-Type', async () => {
    const content = 'exact original bytes for signed download';
    const artifactId = await createArtifact(content);
    const token = mintSessionToken();
    const tokenResponse = await request(app.getHttpServer())
      .get(`/artifacts/${artifactId}/download-token`)
      .set('Authorization', `Bearer ${token}`)
      .set('X-Workspace-Id', WORKSPACE_ID)
      .query({ stakeholderId: BOOTSTRAP_STAKEHOLDER_ID })
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
    const token = mintSessionToken();
    const tokenResponse = await request(app.getHttpServer())
      .get(`/artifacts/${artifactId}/download-token`)
      .set('Authorization', `Bearer ${token}`)
      .set('X-Workspace-Id', WORKSPACE_ID)
      .query({ stakeholderId: BOOTSTRAP_STAKEHOLDER_ID })
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
