import type { Pool } from 'pg';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { Suggestion } from '../../src/domain/suggestion.js';
import { SuggestionRepository } from '../../src/persistence/suggestion.repository.js';
import { BOOTSTRAP_STAKEHOLDER_ID } from '../../src/persistence/bootstrap-stakeholder.js';
import { setUpTestDatabase, type TestDatabase } from '../support/test-database.js';

function buildChunkSuggestion(
  overrides: Partial<ConstructorParameters<typeof Suggestion>[0]> = {},
): Suggestion {
  return new Suggestion({
    variant: {
      kind: 'chunk',
      label: `suggestion-${Math.random().toString(36).slice(2, 10)}`,
      content: 'Some proposed content.',
    },
    discipline: 'engineering',
    submittedByStakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
    submittedByActorKind: 'delegated',
    ...overrides,
  });
}

function buildEdgeSuggestion(
  overrides: Partial<ConstructorParameters<typeof Suggestion>[0]> = {},
): Suggestion {
  return new Suggestion({
    variant: {
      kind: 'edge',
      fromChunkLabel: `from-${Math.random().toString(36).slice(2, 10)}`,
      toChunkLabel: `to-${Math.random().toString(36).slice(2, 10)}`,
      relationshipType: 'refines',
    },
    discipline: 'engineering',
    submittedByStakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
    submittedByActorKind: 'delegated',
    ...overrides,
  });
}

describe('SuggestionRepository (containerized Postgres)', () => {
  let database: TestDatabase;
  let pool: Pool;
  let suggestionRepository: SuggestionRepository;

  beforeAll(async () => {
    database = await setUpTestDatabase();
    pool = database.pool;
    suggestionRepository = new SuggestionRepository(pool);
  });

  afterAll(async () => {
    await database?.close();
  });

  it('create persists a chunk-shaped suggestion with status pending and one state-log row', async () => {
    const suggestion = buildChunkSuggestion();

    const created = await suggestionRepository.create(suggestion);

    expect(created.status).toBe('pending');
    expect(created.decidedByStakeholderId).toBeUndefined();
    expect(created.decidedAt).toBeUndefined();

    const row = await pool.query<{
      status: string;
      submitted_by_stakeholder_id: string;
      submitted_by_actor_kind: string;
      decided_by_stakeholder_id: string | null;
      decided_at: Date | null;
    }>(
      `SELECT status, submitted_by_stakeholder_id, submitted_by_actor_kind,
              decided_by_stakeholder_id, decided_at
         FROM suggestions WHERE id = $1`,
      [created.id],
    );
    expect(row.rows).toHaveLength(1);
    expect(row.rows[0]?.status).toBe('pending');
    expect(row.rows[0]?.submitted_by_stakeholder_id).toBe(BOOTSTRAP_STAKEHOLDER_ID);
    expect(row.rows[0]?.submitted_by_actor_kind).toBe('delegated');
    expect(row.rows[0]?.decided_by_stakeholder_id).toBeNull();
    expect(row.rows[0]?.decided_at).toBeNull();

    const logRows = await pool.query<{ old_status: string | null; new_status: string }>(
      'SELECT old_status, new_status FROM suggestion_state_logs WHERE suggestion_id = $1',
      [created.id],
    );
    expect(logRows.rows).toHaveLength(1);
    expect(logRows.rows[0]?.old_status).toBeNull();
    expect(logRows.rows[0]?.new_status).toBe('pending');
  });

  it('create persists an edge-shaped suggestion with status pending and one state-log row', async () => {
    const suggestion = buildEdgeSuggestion();

    const created = await suggestionRepository.create(suggestion);

    expect(created.status).toBe('pending');

    const logRows = await pool.query<{ old_status: string | null; new_status: string }>(
      'SELECT old_status, new_status FROM suggestion_state_logs WHERE suggestion_id = $1',
      [created.id],
    );
    expect(logRows.rows).toHaveLength(1);
    expect(logRows.rows[0]?.old_status).toBeNull();
    expect(logRows.rows[0]?.new_status).toBe('pending');
  });

  it('findById round-trips every field exactly for a persisted chunk-shaped suggestion', async () => {
    const suggestion = buildChunkSuggestion({ discipline: 'security' });

    const created = await suggestionRepository.create(suggestion);
    const found = await suggestionRepository.findById(created.id);

    expect(found).toBeDefined();
    expect(found).toEqual(created);
  });

  it('findById round-trips every field exactly for a persisted edge-shaped suggestion', async () => {
    const suggestion = buildEdgeSuggestion({ discipline: 'design' });

    const created = await suggestionRepository.create(suggestion);
    const found = await suggestionRepository.findById(created.id);

    expect(found).toBeDefined();
    expect(found).toEqual(created);
  });

  it('findById returns an explicit not-found result (undefined) for an unknown id', async () => {
    const found = await suggestionRepository.findById('00000000-0000-0000-0000-00000000dead');

    expect(found).toBeUndefined();
  });

  it('rejects an unknown submittedByStakeholderId via the foreign key constraint', async () => {
    const suggestion = buildChunkSuggestion({
      submittedByStakeholderId: '00000000-0000-0000-0000-00000000dead',
    });

    await expect(suggestionRepository.create(suggestion)).rejects.toThrow();
  });

  function uniqueBranchName(): string {
    return `accepted-branch-${Math.random().toString(36).slice(2, 10)}`;
  }

  it('accept creates a linked draft branch and logs exactly one pending->accepted transition', async () => {
    const suggestion = await suggestionRepository.create(
      buildChunkSuggestion({ discipline: 'security' }),
    );
    const branchName = uniqueBranchName();

    const result = await suggestionRepository.accept(
      suggestion.id,
      branchName,
      BOOTSTRAP_STAKEHOLDER_ID,
    );

    expect(result.kind).toBe('accepted');
    if (result.kind !== 'accepted') {
      throw new Error('expected accepted result');
    }
    expect(result.branch.name).toBe(branchName);
    expect(result.branch.discipline).toBe('security');
    expect(result.branch.originSuggestionId).toBe(suggestion.id);
    expect(result.branch.status).toBe('draft');

    const suggestionRow = await pool.query<{
      status: string;
      decided_by_stakeholder_id: string | null;
      decided_at: Date | null;
    }>(
      'SELECT status, decided_by_stakeholder_id, decided_at FROM suggestions WHERE id = $1',
      [suggestion.id],
    );
    expect(suggestionRow.rows[0]?.status).toBe('accepted');
    expect(suggestionRow.rows[0]?.decided_by_stakeholder_id).toBe(BOOTSTRAP_STAKEHOLDER_ID);
    expect(suggestionRow.rows[0]?.decided_at).not.toBeNull();

    const logRows = await pool.query<{ old_status: string | null; new_status: string }>(
      "SELECT old_status, new_status FROM suggestion_state_logs WHERE suggestion_id = $1 AND new_status = 'accepted'",
      [suggestion.id],
    );
    expect(logRows.rows).toHaveLength(1);
    expect(logRows.rows[0]?.old_status).toBe('pending');
  });

  it('accept returns not_found for an unknown suggestion id and creates no branch', async () => {
    const branchName = uniqueBranchName();

    const result = await suggestionRepository.accept(
      '00000000-0000-0000-0000-00000000dead',
      branchName,
      BOOTSTRAP_STAKEHOLDER_ID,
    );

    expect(result.kind).toBe('not_found');
    const branchRow = await pool.query('SELECT id FROM branches WHERE name = $1', [branchName]);
    expect(branchRow.rows).toHaveLength(0);
  });

  it('accept returns not_pending for an already-accepted suggestion and creates no second branch', async () => {
    const suggestion = await suggestionRepository.create(buildChunkSuggestion());
    await suggestionRepository.accept(suggestion.id, uniqueBranchName(), BOOTSTRAP_STAKEHOLDER_ID);

    const secondBranchName = uniqueBranchName();
    const result = await suggestionRepository.accept(
      suggestion.id,
      secondBranchName,
      BOOTSTRAP_STAKEHOLDER_ID,
    );

    expect(result.kind).toBe('not_pending');
    const branchRow = await pool.query('SELECT id FROM branches WHERE name = $1', [
      secondBranchName,
    ]);
    expect(branchRow.rows).toHaveLength(0);
  });

  it('accept rolls back the suggestion state on a duplicate branch name, leaving no orphaned rows', async () => {
    const existingBranchName = uniqueBranchName();
    await pool.query(
      `INSERT INTO branches (id, name, discipline, status, created_by_stakeholder_id)
       VALUES (gen_random_uuid(), $1, 'engineering', 'draft', $2)`,
      [existingBranchName, BOOTSTRAP_STAKEHOLDER_ID],
    );
    const suggestion = await suggestionRepository.create(buildChunkSuggestion());

    await expect(
      suggestionRepository.accept(suggestion.id, existingBranchName, BOOTSTRAP_STAKEHOLDER_ID),
    ).rejects.toThrow();

    const suggestionRow = await pool.query<{ status: string; decided_at: Date | null }>(
      'SELECT status, decided_at FROM suggestions WHERE id = $1',
      [suggestion.id],
    );
    expect(suggestionRow.rows[0]?.status).toBe('pending');
    expect(suggestionRow.rows[0]?.decided_at).toBeNull();

    const logRows = await pool.query(
      'SELECT id FROM suggestion_state_logs WHERE suggestion_id = $1',
      [suggestion.id],
    );
    expect(logRows.rows).toHaveLength(1);
  });

  it('reject sets status=rejected and logs exactly one pending->rejected transition', async () => {
    const suggestion = await suggestionRepository.create(
      buildChunkSuggestion({ discipline: 'security' }),
    );

    const result = await suggestionRepository.reject(suggestion.id, BOOTSTRAP_STAKEHOLDER_ID);

    expect(result.kind).toBe('rejected');

    const suggestionRow = await pool.query<{
      status: string;
      decided_by_stakeholder_id: string | null;
      decided_at: Date | null;
    }>(
      'SELECT status, decided_by_stakeholder_id, decided_at FROM suggestions WHERE id = $1',
      [suggestion.id],
    );
    expect(suggestionRow.rows[0]?.status).toBe('rejected');
    expect(suggestionRow.rows[0]?.decided_by_stakeholder_id).toBe(BOOTSTRAP_STAKEHOLDER_ID);
    expect(suggestionRow.rows[0]?.decided_at).not.toBeNull();

    const logRows = await pool.query<{ old_status: string | null; new_status: string }>(
      "SELECT old_status, new_status FROM suggestion_state_logs WHERE suggestion_id = $1 AND new_status = 'rejected'",
      [suggestion.id],
    );
    expect(logRows.rows).toHaveLength(1);
    expect(logRows.rows[0]?.old_status).toBe('pending');
  });

  it('reject returns not_found for an unknown suggestion id', async () => {
    const result = await suggestionRepository.reject(
      '00000000-0000-0000-0000-00000000dead',
      BOOTSTRAP_STAKEHOLDER_ID,
    );

    expect(result.kind).toBe('not_found');
  });

  it('reject returns not_pending for an already-rejected suggestion', async () => {
    const suggestion = await suggestionRepository.create(buildChunkSuggestion());
    await suggestionRepository.reject(suggestion.id, BOOTSTRAP_STAKEHOLDER_ID);

    const result = await suggestionRepository.reject(suggestion.id, BOOTSTRAP_STAKEHOLDER_ID);

    expect(result.kind).toBe('not_pending');
  });

  it('findAll returns suggestions ordered oldest-first, optionally filtered by status', async () => {
    const first = await suggestionRepository.create(buildChunkSuggestion());
    const second = await suggestionRepository.create(buildChunkSuggestion());
    await suggestionRepository.reject(second.id, BOOTSTRAP_STAKEHOLDER_ID);

    const pendingOnly = await suggestionRepository.findAll('pending');
    expect(pendingOnly.some((suggestion) => suggestion.id === first.id)).toBe(true);
    expect(pendingOnly.some((suggestion) => suggestion.id === second.id)).toBe(false);

    const all = await suggestionRepository.findAll();
    const firstIndex = all.findIndex((suggestion) => suggestion.id === first.id);
    const secondIndex = all.findIndex((suggestion) => suggestion.id === second.id);
    expect(firstIndex).toBeGreaterThanOrEqual(0);
    expect(secondIndex).toBeGreaterThan(firstIndex);
  });
});
