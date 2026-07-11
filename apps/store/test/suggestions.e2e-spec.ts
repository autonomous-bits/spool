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

describe('Suggestions HTTP API (containerized Postgres)', () => {
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

  function uniqueLabel(prefix: string): string {
    return `${prefix}-${Math.random().toString(36).slice(2, 10)}`;
  }

  async function fetchSuggestionRow(id: string): Promise<{
    status: string;
    submitted_by_stakeholder_id: string;
    submitted_by_actor_kind: string;
    decided_by_stakeholder_id: string | null;
    decided_at: Date | null;
  }> {
    const pool = database.pool;
    const result = await pool.query(
      `SELECT status, submitted_by_stakeholder_id, submitted_by_actor_kind,
              decided_by_stakeholder_id, decided_at
         FROM suggestions WHERE id = $1`,
      [id],
    );
    const row = result.rows[0] as
      | {
          status: string;
          submitted_by_stakeholder_id: string;
          submitted_by_actor_kind: string;
          decided_by_stakeholder_id: string | null;
          decided_at: Date | null;
        }
      | undefined;
    if (row === undefined) {
      throw new Error(`No suggestion row found for id ${id}`);
    }
    return row;
  }

  async function fetchStateLogRows(
    suggestionId: string,
  ): Promise<{ old_status: string | null; new_status: string }[]> {
    const pool = database.pool;
    const result = await pool.query(
      'SELECT old_status, new_status FROM suggestion_state_logs WHERE suggestion_id = $1',
      [suggestionId],
    );
    return result.rows as { old_status: string | null; new_status: string }[];
  }

  it('POST /suggestions with a valid chunk-shaped body persists pending + one state log row', async () => {
    const response = await request(app.getHttpServer())
      .post('/suggestions')
      .set('X-Workspace-Id', WORKSPACE_ID)
      .send({
        label: uniqueLabel('e2e-suggestion'),
        content: 'Some proposed content.',
        discipline: 'engineering',
        stakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
      });

    expect(response.status).toBe(201);
    expect(response.body.status).toBe('pending');

    const row = await fetchSuggestionRow(response.body.id as string);
    expect(row.status).toBe('pending');
    expect(row.submitted_by_stakeholder_id).toBe(BOOTSTRAP_STAKEHOLDER_ID);
    expect(row.submitted_by_actor_kind).toBe('delegated');
    expect(row.decided_by_stakeholder_id).toBeNull();
    expect(row.decided_at).toBeNull();

    const logRows = await fetchStateLogRows(response.body.id as string);
    expect(logRows).toHaveLength(1);
    expect(logRows[0]?.old_status).toBeNull();
    expect(logRows[0]?.new_status).toBe('pending');
  });

  it('POST /suggestions with a valid edge-shaped body persists pending + one state log row', async () => {
    const response = await request(app.getHttpServer())
      .post('/suggestions')
      .set('X-Workspace-Id', WORKSPACE_ID)
      .send({
        fromChunkLabel: uniqueLabel('e2e-from'),
        toChunkLabel: uniqueLabel('e2e-to'),
        relationshipType: 'refines',
        discipline: 'engineering',
        stakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
      });

    expect(response.status).toBe(201);
    expect(response.body.status).toBe('pending');

    const row = await fetchSuggestionRow(response.body.id as string);
    expect(row.status).toBe('pending');
    expect(row.submitted_by_actor_kind).toBe('delegated');
    expect(row.decided_by_stakeholder_id).toBeNull();
    expect(row.decided_at).toBeNull();

    const logRows = await fetchStateLogRows(response.body.id as string);
    expect(logRows).toHaveLength(1);
    expect(logRows[0]?.old_status).toBeNull();
    expect(logRows[0]?.new_status).toBe('pending');
  });

  it('POST /suggestions returns 400 when the body mixes chunk and edge fields', async () => {
    const response = await request(app.getHttpServer())
      .post('/suggestions')
      .set('X-Workspace-Id', WORKSPACE_ID)
      .send({
        label: uniqueLabel('e2e-mixed'),
        content: 'content',
        fromChunkLabel: uniqueLabel('e2e-mixed-from'),
        toChunkLabel: uniqueLabel('e2e-mixed-to'),
        relationshipType: 'refines',
        discipline: 'engineering',
        stakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
      });

    expect(response.status).toBe(400);
  });

  it('POST /suggestions returns 400 when the body provides neither variant', async () => {
    const response = await request(app.getHttpServer())
      .post('/suggestions')
      .set('X-Workspace-Id', WORKSPACE_ID)
      .send({
        discipline: 'engineering',
        stakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
      });

    expect(response.status).toBe(400);
  });

  it('POST /suggestions returns 400 for a partial edge-shaped body', async () => {
    const response = await request(app.getHttpServer())
      .post('/suggestions')
      .set('X-Workspace-Id', WORKSPACE_ID)
      .send({
        fromChunkLabel: uniqueLabel('e2e-partial-from'),
        discipline: 'engineering',
        stakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
      });

    expect(response.status).toBe(400);
  });

  it('POST /suggestions returns 403 for an unknown stakeholderId (not a workspace member)', async () => {
    const response = await request(app.getHttpServer())
      .post('/suggestions')
      .set('X-Workspace-Id', WORKSPACE_ID)
      .send({
        label: uniqueLabel('e2e-unknown-stakeholder'),
        content: 'content',
        discipline: 'engineering',
        stakeholderId: '00000000-0000-0000-0000-00000000dead',
      });

    expect(response.status).toBe(403);
  });

  it('POST /suggestions returns 403 when the X-Workspace-Id header is missing', async () => {
    const response = await request(app.getHttpServer()).post('/suggestions').send({
      label: uniqueLabel('e2e-no-header'),
      content: 'content',
      discipline: 'engineering',
      stakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
    });

    expect(response.status).toBe(403);
  });

  async function createPendingSuggestion(): Promise<string> {
    const response = await request(app.getHttpServer())
      .post('/suggestions')
      .set('X-Workspace-Id', WORKSPACE_ID)
      .send({
        label: uniqueLabel('e2e-accept-suggestion'),
        content: 'Some proposed content.',
        discipline: 'security',
        stakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
      });
    expect(response.status).toBe(201);
    return response.body.id as string;
  }

  it('POST /suggestions/:id/accept creates a branch with the suggestion discipline and originSuggestionId', async () => {
    const suggestionId = await createPendingSuggestion();
    const token = mintSessionToken(BOOTSTRAP_STAKEHOLDER_ID, 'security');
    const branchName = uniqueLabel('e2e-accepted-branch');

    const acceptResponse = await request(app.getHttpServer())
      .post(`/suggestions/${suggestionId}/accept`)
      .set('Authorization', authHeader(token))
      .set('X-Workspace-Id', WORKSPACE_ID)
      .send({ name: branchName });

    expect(acceptResponse.status).toBe(201);
    expect(acceptResponse.body.discipline).toBe('security');
    expect(acceptResponse.body.originSuggestionId).toBe(suggestionId);
    expect(acceptResponse.body.status).toBe('draft');

    const branchGetResponse = await request(app.getHttpServer())
      .get(`/branches/${acceptResponse.body.id as string}`)
      .set('X-Workspace-Id', WORKSPACE_ID)
      .query({ stakeholderId: BOOTSTRAP_STAKEHOLDER_ID });
    expect(branchGetResponse.status).toBe(200);
    expect(branchGetResponse.body.originSuggestionId).toBe(suggestionId);
    expect(branchGetResponse.body.discipline).toBe('security');

    const logRows = await database.pool.query<{ old_status: string | null; new_status: string }>(
      "SELECT old_status, new_status FROM suggestion_state_logs WHERE suggestion_id = $1 AND new_status = 'accepted'",
      [suggestionId],
    );
    expect(logRows.rows).toHaveLength(1);
    expect(logRows.rows[0]?.old_status).toBe('pending');
  });

  it('POST /suggestions/:id/accept returns 409 and creates no branch for a non-pending suggestion', async () => {
    const suggestionId = await createPendingSuggestion();
    const token = mintSessionToken(BOOTSTRAP_STAKEHOLDER_ID, 'security');
    await request(app.getHttpServer())
      .post(`/suggestions/${suggestionId}/accept`)
      .set('Authorization', authHeader(token))
      .set('X-Workspace-Id', WORKSPACE_ID)
      .send({ name: uniqueLabel('e2e-accepted-branch') });

    const secondBranchName = uniqueLabel('e2e-accepted-branch-second');
    const response = await request(app.getHttpServer())
      .post(`/suggestions/${suggestionId}/accept`)
      .set('Authorization', authHeader(token))
      .set('X-Workspace-Id', WORKSPACE_ID)
      .send({ name: secondBranchName });

    expect(response.status).toBe(409);
    const branchRow = await database.pool.query('SELECT id FROM branches WHERE name = $1', [
      secondBranchName,
    ]);
    expect(branchRow.rows).toHaveLength(0);
  });

  it('POST /suggestions/:id/accept returns 401 when Authorization is missing', async () => {
    const suggestionId = await createPendingSuggestion();

    const response = await request(app.getHttpServer())
      .post(`/suggestions/${suggestionId}/accept`)
      .set('X-Workspace-Id', WORKSPACE_ID)
      .send({ name: uniqueLabel('e2e-accepted-branch') });

    expect(response.status).toBe(401);
  });

  it('POST /suggestions/:id/accept returns 401 for a malformed Authorization header', async () => {
    const suggestionId = await createPendingSuggestion();

    const response = await request(app.getHttpServer())
      .post(`/suggestions/${suggestionId}/accept`)
      .set('Authorization', 'Token not-a-bearer-token')
      .set('X-Workspace-Id', WORKSPACE_ID)
      .send({ name: uniqueLabel('e2e-accepted-branch') });

    expect(response.status).toBe(401);
  });

  it('POST /suggestions/:id/accept returns 401 when session-token verification rejects the bearer token', async () => {
    const suggestionId = await createPendingSuggestion();

    const response = await request(app.getHttpServer())
      .post(`/suggestions/${suggestionId}/accept`)
      .set('Authorization', authHeader('not-a-real-signed-token'))
      .set('X-Workspace-Id', WORKSPACE_ID)
      .send({ name: uniqueLabel('e2e-accepted-branch') });

    expect(response.status).toBe(401);
  });

  it('POST /suggestions/:id/accept returns 403 when the X-Workspace-Id header is missing', async () => {
    const suggestionId = await createPendingSuggestion();
    const token = mintSessionToken(BOOTSTRAP_STAKEHOLDER_ID, 'security');

    const response = await request(app.getHttpServer())
      .post(`/suggestions/${suggestionId}/accept`)
      .set('Authorization', authHeader(token))
      .send({ name: uniqueLabel('e2e-accepted-branch') });

    expect(response.status).toBe(403);
  });

  it('POST /suggestions/:id/accept returns 404 for an unknown suggestion id', async () => {
    const token = mintSessionToken(BOOTSTRAP_STAKEHOLDER_ID, 'security');

    const response = await request(app.getHttpServer())
      .post('/suggestions/00000000-0000-0000-0000-00000000dead/accept')
      .set('Authorization', authHeader(token))
      .set('X-Workspace-Id', WORKSPACE_ID)
      .send({ name: uniqueLabel('e2e-accepted-branch') });

    expect(response.status).toBe(404);
  });

  it('POST /suggestions/:id/accept rolls back the suggestion on a duplicate branch name, with no orphaned rows', async () => {
    const existingBranchName = uniqueLabel('e2e-existing-branch');
    await request(app.getHttpServer())
      .post('/branches')
      .set('X-Workspace-Id', WORKSPACE_ID)
      .send({
        name: existingBranchName,
        discipline: 'security',
        stakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
      });
    const suggestionId = await createPendingSuggestion();
    const token = mintSessionToken(BOOTSTRAP_STAKEHOLDER_ID, 'security');

    const response = await request(app.getHttpServer())
      .post(`/suggestions/${suggestionId}/accept`)
      .set('Authorization', authHeader(token))
      .set('X-Workspace-Id', WORKSPACE_ID)
      .send({ name: existingBranchName });

    expect(response.status).toBe(400);

    const suggestionRow = await database.pool.query<{
      status: string;
      decided_at: Date | null;
    }>('SELECT status, decided_at FROM suggestions WHERE id = $1', [suggestionId]);
    expect(suggestionRow.rows[0]?.status).toBe('pending');
    expect(suggestionRow.rows[0]?.decided_at).toBeNull();
  });

  it('POST /suggestions/:id/reject sets status=rejected and logs one transition row', async () => {
    const suggestionId = await createPendingSuggestion();
    const token = mintSessionToken(BOOTSTRAP_STAKEHOLDER_ID, 'security');

    const response = await request(app.getHttpServer())
      .post(`/suggestions/${suggestionId}/reject`)
      .set('Authorization', authHeader(token))
      .set('X-Workspace-Id', WORKSPACE_ID);

    expect(response.status).toBe(201);
    expect(response.body.status).toBe('rejected');

    const row = await fetchSuggestionRow(suggestionId);
    expect(row.status).toBe('rejected');

    const logRows = await fetchStateLogRows(suggestionId);
    const rejectionLog = logRows.find((log) => log.new_status === 'rejected');
    expect(rejectionLog?.old_status).toBe('pending');
  });

  it('POST /suggestions/:id/reject returns 409 for a non-pending suggestion', async () => {
    const suggestionId = await createPendingSuggestion();
    const token = mintSessionToken(BOOTSTRAP_STAKEHOLDER_ID, 'security');
    await request(app.getHttpServer())
      .post(`/suggestions/${suggestionId}/reject`)
      .set('Authorization', authHeader(token))
      .set('X-Workspace-Id', WORKSPACE_ID);

    const response = await request(app.getHttpServer())
      .post(`/suggestions/${suggestionId}/reject`)
      .set('Authorization', authHeader(token))
      .set('X-Workspace-Id', WORKSPACE_ID);

    expect(response.status).toBe(409);
  });

  it('POST /suggestions/:id/reject returns 401 without a valid Authorization header', async () => {
    const suggestionId = await createPendingSuggestion();

    const response = await request(app.getHttpServer())
      .post(`/suggestions/${suggestionId}/reject`)
      .set('X-Workspace-Id', WORKSPACE_ID);

    expect(response.status).toBe(401);
  });

  it('POST /suggestions/:id/reject returns 403 when the X-Workspace-Id header is missing', async () => {
    const suggestionId = await createPendingSuggestion();
    const token = mintSessionToken(BOOTSTRAP_STAKEHOLDER_ID, 'security');

    const response = await request(app.getHttpServer())
      .post(`/suggestions/${suggestionId}/reject`)
      .set('Authorization', authHeader(token));

    expect(response.status).toBe(403);
  });

  it('GET /suggestions/:id returns the suggestion given a valid stakeholderId and X-Workspace-Id header', async () => {
    const suggestionId = await createPendingSuggestion();

    const response = await request(app.getHttpServer())
      .get(`/suggestions/${suggestionId}`)
      .set('X-Workspace-Id', WORKSPACE_ID)
      .query({ stakeholderId: BOOTSTRAP_STAKEHOLDER_ID });

    expect(response.status).toBe(200);
    expect(response.body.id).toBe(suggestionId);
  });

  it('GET /suggestions/:id returns 404 for an unknown id', async () => {
    const response = await request(app.getHttpServer())
      .get('/suggestions/00000000-0000-0000-0000-00000000dead')
      .set('X-Workspace-Id', WORKSPACE_ID)
      .query({ stakeholderId: BOOTSTRAP_STAKEHOLDER_ID });

    expect(response.status).toBe(404);
  });

  it('GET /suggestions/:id returns 400 when the stakeholderId query param is missing', async () => {
    const suggestionId = await createPendingSuggestion();

    const response = await request(app.getHttpServer())
      .get(`/suggestions/${suggestionId}`)
      .set('X-Workspace-Id', WORKSPACE_ID);

    expect(response.status).toBe(400);
  });

  it('GET /suggestions/:id returns 403 when the X-Workspace-Id header is missing', async () => {
    const suggestionId = await createPendingSuggestion();

    const response = await request(app.getHttpServer())
      .get(`/suggestions/${suggestionId}`)
      .query({ stakeholderId: BOOTSTRAP_STAKEHOLDER_ID });

    expect(response.status).toBe(403);
  });

  it('GET /suggestions?status=pending returns only pending suggestions ordered oldest first, given a valid stakeholderId and X-Workspace-Id header', async () => {
    const firstId = await createPendingSuggestion();
    const secondId = await createPendingSuggestion();
    const token = mintSessionToken(BOOTSTRAP_STAKEHOLDER_ID, 'security');
    await request(app.getHttpServer())
      .post(`/suggestions/${secondId}/reject`)
      .set('Authorization', authHeader(token))
      .set('X-Workspace-Id', WORKSPACE_ID);

    const response = await request(app.getHttpServer())
      .get('/suggestions?status=pending')
      .set('X-Workspace-Id', WORKSPACE_ID)
      .query({ stakeholderId: BOOTSTRAP_STAKEHOLDER_ID });

    expect(response.status).toBe(200);
    const ids = (response.body as { id: string }[]).map((suggestion) => suggestion.id);
    expect(ids).toContain(firstId);
    expect(ids).not.toContain(secondId);

    const createdAts = (response.body as { createdAt: string }[]).map((s) => s.createdAt);
    expect(createdAts).toEqual([...createdAts].sort());
  });

  it('GET /suggestions returns 403 when the X-Workspace-Id header is missing', async () => {
    const response = await request(app.getHttpServer())
      .get('/suggestions?status=pending')
      .query({ stakeholderId: BOOTSTRAP_STAKEHOLDER_ID });

    expect(response.status).toBe(403);
  });
});
