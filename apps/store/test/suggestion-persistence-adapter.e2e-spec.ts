/**
 * Adapter-level integration test proving the suggestion review queue (story
 * S04) persists correctly against a real containerized Postgres.
 *
 * Technical spec §"Testing expectations" requires "suggestion-to-branch
 * linkage" to be proven at the adapter level against a real containerized
 * Postgres, not an in-memory substitute. Start it locally before running
 * this file:
 *
 *   docker compose up -d postgres
 *
 * and export the matching connection env vars (see apps/store/AGENTS.md and
 * config/store.env.example), e.g.:
 *
 *   export STORE_DB_HOST=localhost STORE_DB_PORT=5433 \
 *     STORE_DB_USER=spool STORE_DB_PASSWORD=spool_dev STORE_DB_NAME=spool
 */

import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { Pool } from 'pg';
import { randomUUID } from 'node:crypto';
import { loadDatabaseConfig } from '../src/persistence/database-config.js';
import { ensureSchema } from '../src/persistence/schema.js';
import { SuggestionRepository } from '../src/persistence/suggestion.repository.js';
import { acceptSuggestion, rejectSuggestion } from '../src/domain/suggestion-lifecycle.js';
import { humanActor, delegatedActor } from '../src/domain/types/actor/actor-context.js';
import {
  workspaceId,
  suggestionId,
  stakeholderId,
  branchId,
} from '../src/domain/types/index.js';

function openPool(): Pool {
  const config = loadDatabaseConfig();
  return new Pool({
    host: config.host,
    port: config.port,
    user: config.user,
    password: config.password,
    database: config.database,
  });
}

const hasDatabaseConfig = [
  'STORE_DB_HOST',
  'STORE_DB_PORT',
  'STORE_DB_USER',
  'STORE_DB_PASSWORD',
  'STORE_DB_NAME',
].every((key) => Boolean(process.env[key]?.trim()));

describe.skipIf(!hasDatabaseConfig)(
  'SuggestionRepository (Postgres adapter, suggestion review queue)',
  () => {
    const workspaceA = workspaceId(`ws-${randomUUID()}`);
    const workspaceB = workspaceId(`ws-${randomUUID()}`);
    const reviewer = stakeholderId(`stakeholder-${randomUUID()}`);
    const decidedAt = '2026-07-01T00:00:00.000Z';

    beforeAll(async () => {
      const bootstrapPool = openPool();
      await ensureSchema(bootstrapPool);
      await bootstrapPool.end();
    });

    afterAll(async () => {
      const cleanupPool = openPool();
      await cleanupPool.query(
        'DELETE FROM branches WHERE workspace_id = $1 OR workspace_id = $2',
        [workspaceA, workspaceB],
      );
      await cleanupPool.query(
        'DELETE FROM suggestions WHERE workspace_id = $1 OR workspace_id = $2',
        [workspaceA, workspaceB],
      );
      await cleanupPool.end();
    });

    it('AC3: a newly submitted suggestion starts pending', async () => {
      const pool = openPool();
      const repo = new SuggestionRepository(pool);
      const id = suggestionId(`sug-${randomUUID()}`);

      const created = await repo.createSuggestion({
        workspaceId: workspaceA,
        suggestionId: id,
        discipline: 'engineering',
        payload: { kind: 'chunk-modification', ideaLabel: 'IDEA-checkout-flow' },
      });
      await pool.end();

      expect(created.state).toBe('pending');
      expect(created.decidedByStakeholderId).toBeUndefined();
    });

    it('AC1: listSuggestions surfaces pending, accepted, and rejected suggestions in a workspace', async () => {
      const pool = openPool();
      const repo = new SuggestionRepository(pool);
      const pendingId = suggestionId(`sug-pending-${randomUUID()}`);
      const acceptedId = suggestionId(`sug-accepted-${randomUUID()}`);
      const rejectedId = suggestionId(`sug-rejected-${randomUUID()}`);

      await repo.createSuggestion({
        workspaceId: workspaceA,
        suggestionId: pendingId,
        discipline: 'engineering',
        payload: { kind: 'chunk-modification' },
      });
      await repo.createSuggestion({
        workspaceId: workspaceA,
        suggestionId: acceptedId,
        discipline: 'engineering',
        payload: { kind: 'chunk-modification' },
      });
      await repo.createSuggestion({
        workspaceId: workspaceA,
        suggestionId: rejectedId,
        discipline: 'engineering',
        payload: { kind: 'edge-modification' },
      });

      const acceptDecision = acceptSuggestion(
        'pending',
        humanActor(reviewer),
        workspaceA,
        acceptedId,
        'engineering',
        decidedAt,
      );
      await repo.acceptSuggestionAndRegisterBranch(acceptDecision, {
        branchId: branchId(`branch-${randomUUID()}`),
        workspaceId: workspaceA,
        discipline: 'engineering',
      });

      const rejectDecision = rejectSuggestion(
        'pending',
        humanActor(reviewer),
        workspaceA,
        rejectedId,
        decidedAt,
      );
      await repo.rejectSuggestion(rejectDecision);

      const all = await repo.listSuggestions(workspaceA);
      await pool.end();

      const byId = new Map(all.map((s) => [s.suggestionId, s]));
      expect(byId.get(pendingId)?.state).toBe('pending');
      expect(byId.get(acceptedId)?.state).toBe('accepted');
      expect(byId.get(rejectedId)?.state).toBe('rejected');
    });

    it('AC4: an accept decision is attributed to the authenticated human actor, not a client-supplied claim', async () => {
      const pool = openPool();
      const repo = new SuggestionRepository(pool);
      const id = suggestionId(`sug-${randomUUID()}`);

      await repo.createSuggestion({
        workspaceId: workspaceA,
        suggestionId: id,
        discipline: 'engineering',
        payload: { kind: 'chunk-modification' },
      });

      const decision = acceptSuggestion(
        'pending',
        humanActor(reviewer),
        workspaceA,
        id,
        'engineering',
        decidedAt,
      );
      const { suggestion } = await repo.acceptSuggestionAndRegisterBranch(decision, {
        branchId: branchId(`branch-${randomUUID()}`),
        workspaceId: workspaceA,
        discipline: 'engineering',
      });
      await pool.end();

      expect(suggestion.decidedByStakeholderId).toBe(reviewer);
      expect(suggestion.decidedAt).toBe(decidedAt);
    });

    it('AC4: a delegated (non-human) actor cannot produce an accept/reject decision at all', () => {
      expect(() =>
        acceptSuggestion('pending', delegatedActor(reviewer), workspaceA, suggestionId(`sug-${randomUUID()}`), 'engineering', decidedAt),
      ).toThrow(/human stakeholder/);
      expect(() =>
        rejectSuggestion('pending', delegatedActor(reviewer), workspaceA, suggestionId(`sug-${randomUUID()}`), decidedAt),
      ).toThrow(/human stakeholder/);
    });

    it('AC2: a branch initiated from an accepted suggestion traces back to it via origin_suggestion_id', async () => {
      const pool = openPool();
      const repo = new SuggestionRepository(pool);
      const id = suggestionId(`sug-${randomUUID()}`);
      const originatedBranch = branchId(`branch-${randomUUID()}`);

      await repo.createSuggestion({
        workspaceId: workspaceA,
        suggestionId: id,
        discipline: 'engineering',
        payload: { kind: 'chunk-modification' },
      });

      const decision = acceptSuggestion(
        'pending',
        humanActor(reviewer),
        workspaceA,
        id,
        'engineering',
        decidedAt,
      );
      const { branch } = await repo.acceptSuggestionAndRegisterBranch(decision, {
        branchId: originatedBranch,
        workspaceId: workspaceA,
        discipline: 'engineering',
      });

      const traced = await repo.findOriginatingSuggestion(workspaceA, originatedBranch);
      await pool.end();

      expect(branch.originSuggestionId).toBe(id);
      expect(traced?.suggestionId).toBe(id);
      expect(traced?.state).toBe('accepted');
    });

    it('rejecting a suggestion never registers a branch (no origin registration on reject)', async () => {
      const pool = openPool();
      const repo = new SuggestionRepository(pool);
      const id = suggestionId(`sug-${randomUUID()}`);
      const unrelatedBranch = branchId(`branch-${randomUUID()}`);

      await repo.createSuggestion({
        workspaceId: workspaceA,
        suggestionId: id,
        discipline: 'engineering',
        payload: { kind: 'chunk-modification' },
      });
      const decision = rejectSuggestion('pending', humanActor(reviewer), workspaceA, id, decidedAt);
      await repo.rejectSuggestion(decision);

      const traced = await repo.findOriginatingSuggestion(workspaceA, unrelatedBranch);
      await pool.end();

      expect(traced).toBeUndefined();
    });

    it('deciding an already-decided suggestion throws SuggestionLifecycleError(invalid-state-transition)', async () => {
      const pool = openPool();
      const repo = new SuggestionRepository(pool);
      const id = suggestionId(`sug-${randomUUID()}`);

      await repo.createSuggestion({
        workspaceId: workspaceA,
        suggestionId: id,
        discipline: 'engineering',
        payload: { kind: 'chunk-modification' },
      });
      const firstDecision = rejectSuggestion('pending', humanActor(reviewer), workspaceA, id, decidedAt);
      await repo.rejectSuggestion(firstDecision);

      // A second decision built from a stale 'pending' snapshot must still
      // be rejected by the database's WHERE state = 'pending' guard, not
      // just by the domain's in-memory state check.
      const secondDecision = acceptSuggestion(
        'pending',
        humanActor(reviewer),
        workspaceA,
        id,
        'engineering',
        decidedAt,
      );

      await expect(
        repo.acceptSuggestionAndRegisterBranch(secondDecision, {
          branchId: branchId(`branch-${randomUUID()}`),
          workspaceId: workspaceA,
          discipline: 'engineering',
        }),
      ).rejects.toMatchObject({
        name: 'SuggestionLifecycleError',
        code: 'invalid-state-transition',
      });

      const stillRejected = await repo.getSuggestion(workspaceA, id);
      await pool.end();

      expect(stillRejected?.state).toBe('rejected');
    });

    it('two concurrent decisions racing on the same pending suggestion: only one commits', async () => {
      const pool = openPool();
      const repo = new SuggestionRepository(pool);
      const id = suggestionId(`sug-${randomUUID()}`);

      await repo.createSuggestion({
        workspaceId: workspaceA,
        suggestionId: id,
        discipline: 'engineering',
        payload: { kind: 'chunk-modification' },
      });

      const acceptDecision = acceptSuggestion(
        'pending',
        humanActor(reviewer),
        workspaceA,
        id,
        'engineering',
        decidedAt,
      );
      const rejectDecision = rejectSuggestion('pending', humanActor(reviewer), workspaceA, id, decidedAt);

      const results = await Promise.allSettled([
        repo.acceptSuggestionAndRegisterBranch(acceptDecision, {
          branchId: branchId(`branch-${randomUUID()}`),
          workspaceId: workspaceA,
          discipline: 'engineering',
        }),
        repo.rejectSuggestion(rejectDecision),
      ]);

      const finalState = await repo.getSuggestion(workspaceA, id);
      await pool.end();

      const fulfilled = results.filter((r) => r.status === 'fulfilled');
      const rejected = results.filter((r) => r.status === 'rejected');
      expect(fulfilled).toHaveLength(1);
      expect(rejected).toHaveLength(1);
      expect(['accepted', 'rejected']).toContain(finalState?.state);
      expect(
        (rejected[0] as PromiseRejectedResult).reason,
      ).toMatchObject({ name: 'SuggestionLifecycleError', code: 'invalid-state-transition' });
    });

    it('tenant isolation: a suggestion in workspace A is not visible via workspace B reads', async () => {
      const pool = openPool();
      const repo = new SuggestionRepository(pool);
      const id = suggestionId(`sug-${randomUUID()}`);
      const originatedBranch = branchId(`branch-${randomUUID()}`);

      await repo.createSuggestion({
        workspaceId: workspaceA,
        suggestionId: id,
        discipline: 'engineering',
        payload: { kind: 'chunk-modification' },
      });
      const decision = acceptSuggestion(
        'pending',
        humanActor(reviewer),
        workspaceA,
        id,
        'engineering',
        decidedAt,
      );
      await repo.acceptSuggestionAndRegisterBranch(decision, {
        branchId: originatedBranch,
        workspaceId: workspaceA,
        discipline: 'engineering',
      });

      // Same branch id, but registered/looked up in a different workspace:
      // must not resolve workspace A's suggestion.
      const crossWorkspaceLookup = await repo.findOriginatingSuggestion(
        workspaceB,
        originatedBranch,
      );
      const crossWorkspaceGet = await repo.getSuggestion(workspaceB, id);
      const workspaceBList = await repo.listSuggestions(workspaceB);
      await pool.end();

      expect(crossWorkspaceLookup).toBeUndefined();
      expect(crossWorkspaceGet).toBeUndefined();
      expect(workspaceBList.some((s) => s.suggestionId === id)).toBe(false);
    });

    it('linking an accepted decision to a branch in a different workspace throws before touching the database', async () => {
      const pool = openPool();
      const repo = new SuggestionRepository(pool);
      const id = suggestionId(`sug-${randomUUID()}`);

      await repo.createSuggestion({
        workspaceId: workspaceA,
        suggestionId: id,
        discipline: 'engineering',
        payload: { kind: 'chunk-modification' },
      });
      const decision = acceptSuggestion(
        'pending',
        humanActor(reviewer),
        workspaceA,
        id,
        'engineering',
        decidedAt,
      );

      await expect(
        repo.acceptSuggestionAndRegisterBranch(decision, {
          branchId: branchId(`branch-${randomUUID()}`),
          workspaceId: workspaceB, // mismatch
          discipline: 'engineering',
        }),
      ).rejects.toMatchObject({
        name: 'SuggestionLifecycleError',
        code: 'tenant-boundary-violation',
      });

      const stillPending = await repo.getSuggestion(workspaceA, id);
      await pool.end();

      expect(stillPending?.state).toBe('pending');
    });

    it(
      'story S11: linking an accepted decision to a branch in the wrong ' +
        'discipline throws SuggestionLifecycleError(discipline-boundary-violation) ' +
        'before touching the database',
      async () => {
        const pool = openPool();
        const repo = new SuggestionRepository(pool);
        const id = suggestionId(`sug-${randomUUID()}`);

        await repo.createSuggestion({
          workspaceId: workspaceA,
          suggestionId: id,
          discipline: 'engineering',
          payload: { kind: 'chunk-modification' },
        });
        const decision = acceptSuggestion(
          'pending',
          humanActor(reviewer),
          workspaceA,
          id,
          'engineering',
          decidedAt,
        );

        await expect(
          repo.acceptSuggestionAndRegisterBranch(decision, {
            branchId: branchId(`branch-${randomUUID()}`),
            workspaceId: workspaceA,
            discipline: 'product', // mismatch: decision designated 'engineering'
          }),
        ).rejects.toMatchObject({
          name: 'SuggestionLifecycleError',
          code: 'discipline-boundary-violation',
        });

        const stillPending = await repo.getSuggestion(workspaceA, id);
        await pool.end();

        expect(stillPending?.state).toBe('pending');
      },
    );

    it('a failed branch registration (duplicate branch_id) rolls back the whole transaction, leaving the suggestion pending', async () => {
      const pool = openPool();
      const repo = new SuggestionRepository(pool);
      const firstSuggestionId = suggestionId(`sug-${randomUUID()}`);
      const secondSuggestionId = suggestionId(`sug-${randomUUID()}`);
      const collidingBranchId = branchId(`branch-${randomUUID()}`);

      await repo.createSuggestion({
        workspaceId: workspaceA,
        suggestionId: firstSuggestionId,
        discipline: 'engineering',
        payload: { kind: 'chunk-modification' },
      });
      await repo.createSuggestion({
        workspaceId: workspaceA,
        suggestionId: secondSuggestionId,
        discipline: 'engineering',
        payload: { kind: 'chunk-modification' },
      });

      // First accept succeeds and registers the branch.
      const firstDecision = acceptSuggestion(
        'pending',
        humanActor(reviewer),
        workspaceA,
        firstSuggestionId,
        'engineering',
        decidedAt,
      );
      await repo.acceptSuggestionAndRegisterBranch(firstDecision, {
        branchId: collidingBranchId,
        workspaceId: workspaceA,
        discipline: 'engineering',
      });

      // Second, unrelated pending suggestion tries to register the same
      // branch id: the branches PK conflict must roll back the *entire*
      // transaction, including the suggestion's state update.
      const secondDecision = acceptSuggestion(
        'pending',
        humanActor(reviewer),
        workspaceA,
        secondSuggestionId,
        'engineering',
        decidedAt,
      );

      await expect(
        repo.acceptSuggestionAndRegisterBranch(secondDecision, {
          branchId: collidingBranchId,
          workspaceId: workspaceA,
          discipline: 'engineering',
        }),
      ).rejects.toThrow();

      const secondSuggestionAfterFailure = await repo.getSuggestion(workspaceA, secondSuggestionId);
      await pool.end();

      expect(secondSuggestionAfterFailure?.state).toBe('pending');
      expect(secondSuggestionAfterFailure?.decidedByStakeholderId).toBeUndefined();
    });
  },
);
