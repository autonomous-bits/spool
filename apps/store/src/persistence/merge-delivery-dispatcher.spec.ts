/**
 * Unit test proving the merge-triggered delivery dispatch boundary is
 * non-blocking and isolated from delivery failures (story S08 AC2/AC3).
 *
 * No database is needed: `MergeDeliveryDispatcher` depends only on
 * `DeliverySubscriptionRepository` (faked here) and an injected
 * `DeliveryPushPort` (faked here).
 */

import { describe, expect, it, vi } from 'vitest';
import {
  MergeDeliveryDispatcher,
  type DeliveryPushPort,
  type MergeDeliveryEvent,
} from './merge-delivery-dispatcher.js';
import type { DeliverySubscriptionRepository } from './delivery-subscription.repository.js';
import type {
  BranchId,
  ConsumerId,
  WorkspaceId,
} from '../domain/types/index.js';

function fakeSubscriptionRepository(
  matching: { consumerId: ConsumerId }[],
): Pick<DeliverySubscriptionRepository, 'listSubscriptionsForDiscipline'> {
  return {
    listSubscriptionsForDiscipline: vi.fn().mockResolvedValue(
      matching.map((m) => ({
        workspaceId: 'ws-1' as WorkspaceId,
        consumerId: m.consumerId,
        webhookUrl: 'https://example.test/hook',
        createdAt: '2026-07-01T00:00:00.000Z',
        updatedAt: '2026-07-01T00:00:00.000Z',
      })),
    ),
  };
}

const baseEvent: MergeDeliveryEvent = {
  workspaceId: 'ws-1' as WorkspaceId,
  branchId: 'branch-1' as BranchId,
  discipline: 'engineering',
  mergedAt: '2026-07-01T00:00:00.000Z',
};

describe('MergeDeliveryDispatcher', () => {
  it('AC2: dispatchMergeCompleted returns synchronously, before any push attempt resolves', async () => {
    let pushResolved = false;
    const pushPort: DeliveryPushPort = {
      push: vi.fn().mockImplementation(
        () =>
          new Promise<void>((resolve) => {
            setTimeout(() => {
              pushResolved = true;
              resolve();
            }, 20);
          }),
      ),
    };
    const subscriptions = fakeSubscriptionRepository([{ consumerId: 'consumer-1' as ConsumerId }]);
    const dispatcher = new MergeDeliveryDispatcher(
      subscriptions as DeliverySubscriptionRepository,
      pushPort,
    );

    const returnValue = dispatcher.dispatchMergeCompleted(baseEvent);

    expect(returnValue).toBeUndefined();
    expect(pushResolved).toBe(false);

    // Give the scheduled work a chance to run and complete, to confirm it
    // does eventually fire (not just "never runs").
    await new Promise((resolve) => setTimeout(resolve, 50));
    expect(pushPort.push).toHaveBeenCalledTimes(1);
    expect(pushResolved).toBe(true);
  });

  it('AC1/AC3: a rejecting push attempt for one consumer does not affect delivery to another, and never throws back to the caller', async () => {
    const pushPort: DeliveryPushPort = {
      push: vi
        .fn()
        .mockImplementationOnce(() => Promise.reject(new Error('webhook unreachable')))
        .mockImplementationOnce(() => Promise.resolve()),
    };
    const subscriptions = fakeSubscriptionRepository([
      { consumerId: 'consumer-failing' as ConsumerId },
      { consumerId: 'consumer-ok' as ConsumerId },
    ]);
    const dispatcher = new MergeDeliveryDispatcher(
      subscriptions as DeliverySubscriptionRepository,
      pushPort,
    );

    expect(() => dispatcher.dispatchMergeCompleted(baseEvent)).not.toThrow();

    await new Promise((resolve) => setTimeout(resolve, 20));
    expect(pushPort.push).toHaveBeenCalledTimes(2);
  });

  it('looks up subscriptions scoped to the merged branch workspace and discipline', async () => {
    const listSubscriptionsForDiscipline = vi.fn().mockResolvedValue([]);
    const subscriptions = { listSubscriptionsForDiscipline };
    const pushPort: DeliveryPushPort = { push: vi.fn().mockResolvedValue(undefined) };
    const dispatcher = new MergeDeliveryDispatcher(
      subscriptions as unknown as DeliverySubscriptionRepository,
      pushPort,
    );

    dispatcher.dispatchMergeCompleted(baseEvent);
    await new Promise((resolve) => setTimeout(resolve, 20));

    expect(listSubscriptionsForDiscipline).toHaveBeenCalledWith(
      baseEvent.workspaceId,
      baseEvent.discipline,
    );
  });

  it('a subscription-lookup failure is caught and logged, not left as an unhandled rejection', async () => {
    const subscriptions = {
      listSubscriptionsForDiscipline: vi.fn().mockRejectedValue(new Error('db unavailable')),
    };
    const pushPort: DeliveryPushPort = { push: vi.fn() };
    const dispatcher = new MergeDeliveryDispatcher(
      subscriptions as unknown as DeliverySubscriptionRepository,
      pushPort,
    );
    const unhandled = vi.fn();
    process.once('unhandledRejection', unhandled);

    expect(() => dispatcher.dispatchMergeCompleted(baseEvent)).not.toThrow();
    await new Promise((resolve) => setTimeout(resolve, 20));

    process.removeListener('unhandledRejection', unhandled);
    expect(unhandled).not.toHaveBeenCalled();
    expect(pushPort.push).not.toHaveBeenCalled();
  });

  it('a push port that throws synchronously (instead of rejecting) is also caught, not left as an unhandled rejection', async () => {
    const subscriptions = fakeSubscriptionRepository([{ consumerId: 'consumer-1' as ConsumerId }]);
    const pushPort: DeliveryPushPort = {
      push: vi.fn().mockImplementation(() => {
        throw new Error('synchronous transport failure');
      }),
    };
    const dispatcher = new MergeDeliveryDispatcher(
      subscriptions as DeliverySubscriptionRepository,
      pushPort,
    );
    const unhandled = vi.fn();
    process.once('unhandledRejection', unhandled);

    expect(() => dispatcher.dispatchMergeCompleted(baseEvent)).not.toThrow();
    await new Promise((resolve) => setTimeout(resolve, 20));

    process.removeListener('unhandledRejection', unhandled);
    expect(unhandled).not.toHaveBeenCalled();
  });
});
