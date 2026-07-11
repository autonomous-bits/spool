import type { Pool } from 'pg';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { DeliverySubscription } from '../../src/domain/delivery-subscription.js';
import { Workspace } from '../../src/domain/workspace.js';
import { BOOTSTRAP_STAKEHOLDER_ID } from '../../src/persistence/bootstrap-stakeholder.js';
import { DeliverySubscriptionRepository } from '../../src/persistence/delivery-subscription.repository.js';
import { WorkspaceRepository } from '../../src/persistence/workspace.repository.js';
import { setUpTestDatabase, type TestDatabase } from '../support/test-database.js';

function buildSubscription(
  workspaceId: string,
  overrides: Partial<ConstructorParameters<typeof DeliverySubscription>[0]> = {},
): DeliverySubscription {
  return new DeliverySubscription({
    workspaceId,
    url: 'https://example.com/webhook',
    createdByStakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
    ...overrides,
  });
}

describe('DeliverySubscriptionRepository (containerized Postgres)', () => {
  let database: TestDatabase;
  let pool: Pool;
  let repository: DeliverySubscriptionRepository;
  let workspaceA: Workspace;
  let workspaceB: Workspace;

  beforeAll(async () => {
    database = await setUpTestDatabase();
    pool = database.pool;
    repository = new DeliverySubscriptionRepository(pool);

    const workspaceRepository = new WorkspaceRepository(pool);
    workspaceA = await workspaceRepository.createWithFirstMember(
      new Workspace({
        name: `workspace-a-${Math.random().toString(36).slice(2, 10)}`,
        createdByStakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
      }),
    );
    workspaceB = await workspaceRepository.createWithFirstMember(
      new Workspace({
        name: `workspace-b-${Math.random().toString(36).slice(2, 10)}`,
        createdByStakeholderId: BOOTSTRAP_STAKEHOLDER_ID,
      }),
    );
  });

  afterAll(async () => {
    await database.close();
  });

  it('create persists a subscription and round-trips it via findById', async () => {
    const subscription = buildSubscription(workspaceA.id);

    const created = await repository.create(subscription);

    expect(created.id).toBe(subscription.id);
    expect(created.signingSecret).toBe(subscription.signingSecret);
    expect(created.isActive).toBe(true);

    const found = await repository.findById(created.id, workspaceA.id);
    expect(found).toEqual(created);
  });

  it('create persists a discipline filter as a round-tripped array', async () => {
    const subscription = buildSubscription(workspaceA.id, {
      disciplineFilter: ['engineering', 'security'],
    });

    const created = await repository.create(subscription);
    const found = await repository.findById(created.id, workspaceA.id);

    expect(found?.disciplineFilter).toEqual(['engineering', 'security']);
  });

  it('listByWorkspace only returns subscriptions for that workspace', async () => {
    const inA = await repository.create(buildSubscription(workspaceA.id));
    const inB = await repository.create(buildSubscription(workspaceB.id));

    const listA = await repository.listByWorkspace(workspaceA.id);
    const listB = await repository.listByWorkspace(workspaceB.id);

    expect(listA.map((s) => s.id)).toContain(inA.id);
    expect(listA.map((s) => s.id)).not.toContain(inB.id);
    expect(listB.map((s) => s.id)).toContain(inB.id);
    expect(listB.map((s) => s.id)).not.toContain(inA.id);
  });

  it('findById returns undefined for an unknown id', async () => {
    const found = await repository.findById('00000000-0000-0000-0000-00000000dead', workspaceA.id);

    expect(found).toBeUndefined();
  });

  it('findById does not return a subscription belonging to a different workspace (tenant isolation)', async () => {
    const created = await repository.create(buildSubscription(workspaceA.id));

    const foundFromWrongWorkspace = await repository.findById(created.id, workspaceB.id);

    expect(foundFromWrongWorkspace).toBeUndefined();
  });

  it('deactivate soft-deletes a subscription within its own workspace', async () => {
    const created = await repository.create(buildSubscription(workspaceA.id));

    const deactivated = await repository.deactivate(created.id, workspaceA.id);

    expect(deactivated?.isActive).toBe(false);

    const found = await repository.findById(created.id, workspaceA.id);
    expect(found?.isActive).toBe(false);
  });

  it('deactivate does not affect a subscription belonging to a different workspace (tenant isolation)', async () => {
    const created = await repository.create(buildSubscription(workspaceA.id));

    const result = await repository.deactivate(created.id, workspaceB.id);

    expect(result).toBeUndefined();

    const stillActive = await repository.findById(created.id, workspaceA.id);
    expect(stillActive?.isActive).toBe(true);
  });
});
