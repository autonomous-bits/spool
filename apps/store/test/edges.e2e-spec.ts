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

describe('Edges HTTP API (containerized Postgres)', () => {
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
    defaultToken = mintSessionToken(BOOTSTRAP_STAKEHOLDER_ID);
  });

  afterAll(async () => {
    await app.close();
    await database.close();
  });

  function mintSessionToken(stakeholderId: string): string {
    return sessionTokenService.sign({
      stakeholderId,
      authTime: Math.floor(Date.now() / 1000),
      workspaceId: WORKSPACE_ID,
    });
  }

  async function createChunk(label: string, discipline = 'engineering'): Promise<string> {
    const response = await request(app.getHttpServer())
      .post('/chunks')
      .set('Authorization', `Bearer ${defaultToken}`)
      .set('X-Workspace-Id', WORKSPACE_ID)
      .send({
        label,
        content: 'An atomic idea captured over HTTP.',
        discipline,
        chunkType: 'feature',
        contextKind: 'permanent',
        stakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
      });
    expect(response.status).toBe(201);
    return response.body.id as string;
  }

  async function createBranchScopedChunk(
    label: string,
    branchId: string,
    discipline = 'engineering',
  ): Promise<string> {
    const response = await request(app.getHttpServer())
      .post('/chunks')
      .set('Authorization', `Bearer ${defaultToken}`)
      .set('X-Workspace-Id', WORKSPACE_ID)
      .send({
        label,
        content: 'A branch-scoped idea.',
        discipline,
        chunkType: 'feature',
        contextKind: 'permanent',
        stakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
        branchId,
      });
    expect(response.status).toBe(201);
    return response.body.id as string;
  }

  async function createBranch(discipline = 'engineering'): Promise<string> {
    const response = await request(app.getHttpServer())
      .post('/branches')
      .set('Authorization', `Bearer ${defaultToken}`)
      .set('X-Workspace-Id', WORKSPACE_ID)
      .send({
        name: `e2e-branch-${Math.random().toString(36).slice(2, 10)}`,
        discipline,
        stakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
      });
    expect(response.status).toBe(201);
    return response.body.id as string;
  }

  function uniqueLabel(prefix: string): string {
    return `${prefix}-${Math.random().toString(36).slice(2, 10)}`;
  }

  it('POST /edges creates a branchless edge and GET /edges/:id retrieves it', async () => {
    const fromLabel = uniqueLabel('e2e-from');
    const toLabel = uniqueLabel('e2e-to');
    await createChunk(fromLabel);
    await createChunk(toLabel);

    const createResponse = await request(app.getHttpServer())
      .post('/edges')
      .set('Authorization', `Bearer ${defaultToken}`)
      .set('X-Workspace-Id', WORKSPACE_ID)
      .send({
        fromChunkLabel: fromLabel,
        toChunkLabel: toLabel,
        type: 'refines',
        discipline: 'engineering',
        stakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
      });

    expect(createResponse.status).toBe(201);
    expect(createResponse.body.status).toBe('active');
    expect(createResponse.body.branchId).toBeNull();
    expect(createResponse.body.supersededByEdgeId).toBeNull();

    const getResponse = await request(app.getHttpServer())
      .get(`/edges/${createResponse.body.id as string}`)
      .set('Authorization', `Bearer ${defaultToken}`)
      .set('X-Workspace-Id', WORKSPACE_ID)
      .query({ stakeholderId: BOOTSTRAP_STAKEHOLDER_ID });

    expect(getResponse.status).toBe(200);
    expect(getResponse.body).toMatchObject({
      id: createResponse.body.id,
      fromChunkLabel: fromLabel,
      toChunkLabel: toLabel,
      type: 'refines',
      discipline: 'engineering',
    });
  });

  it('POST /edges creates a branch-scoped edge', async () => {
    const branchId = await createBranch();
    const fromLabel = uniqueLabel('e2e-branch-from');
    const toLabel = uniqueLabel('e2e-branch-to');
    await createBranchScopedChunk(fromLabel, branchId);
    await createBranchScopedChunk(toLabel, branchId);

    const createResponse = await request(app.getHttpServer())
      .post('/edges')
      .set('Authorization', `Bearer ${defaultToken}`)
      .set('X-Workspace-Id', WORKSPACE_ID)
      .send({
        fromChunkLabel: fromLabel,
        toChunkLabel: toLabel,
        type: 'depends-on',
        discipline: 'engineering',
        stakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
        branchId,
      });

    expect(createResponse.status).toBe(201);
    expect(createResponse.body.branchId).toBe(branchId);
    expect(createResponse.body.originBranchId).toBe(branchId);

    const getResponse = await request(app.getHttpServer())
      .get(`/edges/${createResponse.body.id as string}`)
      .set('Authorization', `Bearer ${defaultToken}`)
      .set('X-Workspace-Id', WORKSPACE_ID)
      .query({ stakeholderId: BOOTSTRAP_STAKEHOLDER_ID });
    expect(getResponse.status).toBe(200);
    expect(getResponse.body.branchId).toBe(branchId);
  });

  it('POST /edges returns 400 for a missing required field', async () => {
    const response = await request(app.getHttpServer())
      .post('/edges')
      .set('Authorization', `Bearer ${defaultToken}`)
      .set('X-Workspace-Id', WORKSPACE_ID)
      .send({
        toChunkLabel: 'x',
        type: 'refines',
        discipline: 'engineering',
        stakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
      });

    expect(response.status).toBe(400);
  });

  it('POST /edges returns 400 for an invalid type', async () => {
    const fromLabel = uniqueLabel('e2e-bad-type-from');
    const toLabel = uniqueLabel('e2e-bad-type-to');
    await createChunk(fromLabel);
    await createChunk(toLabel);

    const response = await request(app.getHttpServer())
      .post('/edges')
      .set('Authorization', `Bearer ${defaultToken}`)
      .set('X-Workspace-Id', WORKSPACE_ID)
      .send({
        fromChunkLabel: fromLabel,
        toChunkLabel: toLabel,
        type: 'bogus',
        discipline: 'engineering',
        stakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
      });

    expect(response.status).toBe(400);
  });

  it('POST /edges returns 400 when fromChunkLabel === toChunkLabel', async () => {
    const label = uniqueLabel('e2e-same');
    await createChunk(label);

    const response = await request(app.getHttpServer())
      .post('/edges')
      .set('Authorization', `Bearer ${defaultToken}`)
      .set('X-Workspace-Id', WORKSPACE_ID)
      .send({
        fromChunkLabel: label,
        toChunkLabel: label,
        type: 'refines',
        discipline: 'engineering',
        stakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
      });

    expect(response.status).toBe(400);
  });

  it('POST /edges returns 404 when branchId does not exist', async () => {
    const fromLabel = uniqueLabel('e2e-missing-branch-from');
    const toLabel = uniqueLabel('e2e-missing-branch-to');
    await createChunk(fromLabel);
    await createChunk(toLabel);

    const response = await request(app.getHttpServer())
      .post('/edges')
      .set('Authorization', `Bearer ${defaultToken}`)
      .set('X-Workspace-Id', WORKSPACE_ID)
      .send({
        fromChunkLabel: fromLabel,
        toChunkLabel: toLabel,
        type: 'refines',
        discipline: 'engineering',
        stakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
        branchId: '00000000-0000-0000-0000-00000000dead',
      });

    expect(response.status).toBe(404);
  });

  it('POST /edges returns 409 when branch discipline does not match the request', async () => {
    const branchId = await createBranch('design');
    const fromLabel = uniqueLabel('e2e-wrong-discipline-from');
    const toLabel = uniqueLabel('e2e-wrong-discipline-to');
    await createBranchScopedChunk(fromLabel, branchId, 'design');
    await createBranchScopedChunk(toLabel, branchId, 'design');

    const response = await request(app.getHttpServer())
      .post('/edges')
      .set('Authorization', `Bearer ${defaultToken}`)
      .set('X-Workspace-Id', WORKSPACE_ID)
      .send({
        fromChunkLabel: fromLabel,
        toChunkLabel: toLabel,
        type: 'refines',
        discipline: 'engineering',
        stakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
        branchId,
      });

    expect(response.status).toBe(409);
  });

  it('POST /edges returns 404 when toChunkLabel does not resolve in scope', async () => {
    const fromLabel = uniqueLabel('e2e-unresolved-from');
    await createChunk(fromLabel);

    const response = await request(app.getHttpServer())
      .post('/edges')
      .set('Authorization', `Bearer ${defaultToken}`)
      .set('X-Workspace-Id', WORKSPACE_ID)
      .send({
        fromChunkLabel: fromLabel,
        toChunkLabel: uniqueLabel('e2e-nonexistent-to'),
        type: 'refines',
        discipline: 'engineering',
        stakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
      });

    expect(response.status).toBe(404);
  });

  it('POST /edges returns 409 for a duplicate active edge', async () => {
    const fromLabel = uniqueLabel('e2e-dup-from');
    const toLabel = uniqueLabel('e2e-dup-to');
    await createChunk(fromLabel);
    await createChunk(toLabel);

    const body = {
      fromChunkLabel: fromLabel,
      toChunkLabel: toLabel,
      type: 'refines',
      discipline: 'engineering',
      stakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
    };

    const first = await request(app.getHttpServer())
      .post('/edges')
      .set('Authorization', `Bearer ${defaultToken}`)
      .set('X-Workspace-Id', WORKSPACE_ID)
      .send(body);
    expect(first.status).toBe(201);

    const second = await request(app.getHttpServer())
      .post('/edges')
      .set('Authorization', `Bearer ${defaultToken}`)
      .set('X-Workspace-Id', WORKSPACE_ID)
      .send(body);
    expect(second.status).toBe(409);
  });

  it('GET /edges/:id returns 404 for an unknown id', async () => {
    const response = await request(app.getHttpServer())
      .get('/edges/00000000-0000-0000-0000-00000000dead')
      .set('Authorization', `Bearer ${defaultToken}`)
      .set('X-Workspace-Id', WORKSPACE_ID)
      .query({ stakeholderId: BOOTSTRAP_STAKEHOLDER_ID });

    expect(response.status).toBe(404);
  });
});
