import { createHmac } from 'node:crypto';
import { Test, type TestingModule } from '@nestjs/testing';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { DeliveryAttempt } from '../domain/delivery-attempt.js';
import { DeliverySubscription } from '../domain/delivery-subscription.js';
import { DeliveryAttemptRepository } from '../persistence/delivery-attempt.repository.js';
import { DeliverySubscriptionRepository } from '../persistence/delivery-subscription.repository.js';
import { DeliveryWorkerService } from './delivery-worker.service.js';

const POLL_INTERVAL_MS = 1_000;
const CLAIM_BATCH_SIZE = 10;

function buildAttempt(overrides: Partial<{ id: string; attemptCount: number }> = {}): DeliveryAttempt {
  return new DeliveryAttempt({
    id: overrides.id ?? 'attempt-1',
    subscriptionId: 'subscription-1',
    mergeEventId: 'merge-event-1',
    branchId: 'branch-1',
    mergedAt: new Date('2026-07-11T14:27:59.524Z'),
    status: 'in_progress',
    attemptCount: overrides.attemptCount ?? 1,
    lastAttemptedAt: new Date('2026-07-11T14:27:59.524Z'),
    nextRetryAt: null,
    createdAt: new Date('2026-07-11T14:27:59.524Z'),
    updatedAt: new Date('2026-07-11T14:27:59.524Z'),
  });
}

function buildSubscription(): DeliverySubscription {
  return new DeliverySubscription({
    id: 'subscription-1',
    workspaceId: 'workspace-1',
    url: 'https://example.com/delivery',
    signingSecret: 'worker-secret',
    createdByStakeholderId: 'stakeholder-1',
  });
}

describe('DeliveryWorkerService', () => {
  let module: TestingModule;
  let service: DeliveryWorkerService;
  let deliveryAttemptRepository: Pick<
    DeliveryAttemptRepository,
    'claimBatch' | 'markSucceeded' | 'markFailed'
  >;
  let deliverySubscriptionRepository: Pick<DeliverySubscriptionRepository, 'findByIdUnscoped'>;
  let fetchMock: ReturnType<typeof vi.fn>;

  beforeEach(async () => {
    vi.useFakeTimers();
    vi.setSystemTime(0);
    fetchMock = vi.fn();
    vi.stubGlobal('fetch', fetchMock);

    module = await Test.createTestingModule({
      providers: [
        DeliveryWorkerService,
        {
          provide: DeliveryAttemptRepository,
          useValue: {
            claimBatch: vi.fn(),
            markSucceeded: vi.fn(),
            markFailed: vi.fn(),
          } satisfies Pick<DeliveryAttemptRepository, 'claimBatch' | 'markSucceeded' | 'markFailed'>,
        },
        {
          provide: DeliverySubscriptionRepository,
          useValue: {
            findByIdUnscoped: vi.fn(),
          } satisfies Pick<DeliverySubscriptionRepository, 'findByIdUnscoped'>,
        },
      ],
    }).compile();

    service = module.get(DeliveryWorkerService);
    deliveryAttemptRepository = module.get(DeliveryAttemptRepository);
    deliverySubscriptionRepository = module.get(DeliverySubscriptionRepository);
  });

  afterEach(async () => {
    await module.close();
    vi.unstubAllGlobals();
    vi.useRealTimers();
  });

  it('starts polling on module init and posts a signed webhook payload', async () => {
    const attempt = buildAttempt();
    const subscription = buildSubscription();
    const abortTimeoutSpy = vi.spyOn(AbortSignal, 'timeout');
    vi.mocked(deliveryAttemptRepository.claimBatch)
      .mockResolvedValueOnce([attempt])
      .mockResolvedValue([]);
    vi.mocked(deliverySubscriptionRepository.findByIdUnscoped).mockResolvedValue(subscription);
    vi.mocked(deliveryAttemptRepository.markSucceeded).mockResolvedValue(attempt);
    fetchMock.mockResolvedValue(new Response(null, { status: 202 }));

    service.onModuleInit();

    expect(deliveryAttemptRepository.claimBatch).not.toHaveBeenCalled();

    await vi.advanceTimersByTimeAsync(POLL_INTERVAL_MS);

    expect(deliveryAttemptRepository.claimBatch).toHaveBeenCalledWith(CLAIM_BATCH_SIZE);
    expect(fetchMock).toHaveBeenCalledTimes(1);

    const [url, options] = fetchMock.mock.calls[0] as [string, RequestInit];
    const expectedBody = JSON.stringify({
      mergeEventId: attempt.mergeEventId,
      branchId: attempt.branchId,
      mergedAt: attempt.mergedAt,
    });

    expect(url).toBe(subscription.url);
    expect(options.method).toBe('POST');
    expect(options.body).toBe(expectedBody);
    expect(options.redirect).toBe('manual');
    expect(options.signal).toBeInstanceOf(AbortSignal);
    expect((options.headers as Record<string, string>)['X-Spool-Delivery-Id']).toBe(attempt.id);
    expect((options.headers as Record<string, string>)['X-Spool-Signature']).toBe(
      `sha256=${createHmac('sha256', subscription.signingSecret).update(expectedBody).digest('hex')}`,
    );
    expect(abortTimeoutSpy).toHaveBeenCalledWith(5_000);
    expect(deliveryAttemptRepository.markSucceeded).toHaveBeenCalledWith(attempt.id);
    expect(deliveryAttemptRepository.markFailed).not.toHaveBeenCalled();
  });

  it('marks a first failed attempt pending again 2 seconds later after a non-2xx response', async () => {
    const attempt = buildAttempt({ attemptCount: 1 });
    const subscription = buildSubscription();
    vi.mocked(deliveryAttemptRepository.claimBatch).mockResolvedValue([attempt]);
    vi.mocked(deliverySubscriptionRepository.findByIdUnscoped).mockResolvedValue(subscription);
    vi.mocked(deliveryAttemptRepository.markFailed).mockResolvedValue(attempt);
    fetchMock.mockResolvedValue(new Response(null, { status: 500 }));

    service.onModuleInit();
    await vi.advanceTimersByTimeAsync(POLL_INTERVAL_MS);

    expect(deliveryAttemptRepository.markSucceeded).not.toHaveBeenCalled();
    expect(deliveryAttemptRepository.markFailed).toHaveBeenCalledWith(
      attempt.id,
      new Date(3_000),
    );
  });

  it('marks a second failed attempt pending again 4 seconds later after a transport error', async () => {
    const attempt = buildAttempt({ attemptCount: 2 });
    const subscription = buildSubscription();
    vi.mocked(deliveryAttemptRepository.claimBatch).mockResolvedValue([attempt]);
    vi.mocked(deliverySubscriptionRepository.findByIdUnscoped).mockResolvedValue(subscription);
    vi.mocked(deliveryAttemptRepository.markFailed).mockResolvedValue(attempt);
    fetchMock.mockRejectedValue(new Error('network down'));

    service.onModuleInit();
    await vi.advanceTimersByTimeAsync(POLL_INTERVAL_MS);

    expect(deliveryAttemptRepository.markFailed).toHaveBeenCalledWith(
      attempt.id,
      new Date(5_000),
    );
  });

  it('marks a third failed attempt terminal with no retry timestamp', async () => {
    const attempt = buildAttempt({ attemptCount: 3 });
    const subscription = buildSubscription();
    vi.mocked(deliveryAttemptRepository.claimBatch).mockResolvedValue([attempt]);
    vi.mocked(deliverySubscriptionRepository.findByIdUnscoped).mockResolvedValue(subscription);
    vi.mocked(deliveryAttemptRepository.markFailed).mockResolvedValue(attempt);
    fetchMock.mockResolvedValue(new Response(null, { status: 503 }));

    service.onModuleInit();
    await vi.advanceTimersByTimeAsync(POLL_INTERVAL_MS);

    expect(deliveryAttemptRepository.markFailed).toHaveBeenCalledWith(attempt.id, null);
  });

  it('clears the interval on module destroy so no later polls run', async () => {
    vi.mocked(deliveryAttemptRepository.claimBatch).mockResolvedValue([]);

    service.onModuleInit();
    await vi.advanceTimersByTimeAsync(POLL_INTERVAL_MS);

    expect(deliveryAttemptRepository.claimBatch).toHaveBeenCalledTimes(1);

    service.onModuleDestroy();
    await vi.advanceTimersByTimeAsync(POLL_INTERVAL_MS * 3);

    expect(deliveryAttemptRepository.claimBatch).toHaveBeenCalledTimes(1);
  });
});
