import { createHmac } from 'node:crypto';
import { Injectable, type OnModuleDestroy, type OnModuleInit } from '@nestjs/common';
import type { DeliveryAttempt } from '../domain/delivery-attempt.js';
import { DeliveryAttemptRepository } from '../persistence/delivery-attempt.repository.js';
import { DeliverySubscriptionRepository } from '../persistence/delivery-subscription.repository.js';

const POLL_INTERVAL_MS = 1_000;
const CLAIM_BATCH_SIZE = 10;
const FIRST_RETRY_DELAY_MS = 2_000;
const SECOND_RETRY_DELAY_MS = 4_000;

function buildRequestBody(attempt: DeliveryAttempt): string {
  return JSON.stringify({
    mergeEventId: attempt.mergeEventId,
    branchId: attempt.branchId,
    mergedAt: attempt.mergedAt,
  });
}

function signRequestBody(rawBody: string, secret: string): string {
  return createHmac('sha256', secret).update(rawBody).digest('hex');
}

function nextRetryAtForAttempt(attemptCount: number): Date | null {
  if (attemptCount <= 1) {
    return new Date(Date.now() + FIRST_RETRY_DELAY_MS);
  }

  if (attemptCount === 2) {
    return new Date(Date.now() + SECOND_RETRY_DELAY_MS);
  }

  return null;
}

@Injectable()
export class DeliveryWorkerService implements OnModuleInit, OnModuleDestroy {
  private pollTimer: ReturnType<typeof setInterval> | undefined;
  private isPolling = false;

  constructor(
    private readonly deliveryAttemptRepository: DeliveryAttemptRepository,
    private readonly deliverySubscriptionRepository: DeliverySubscriptionRepository,
  ) {}

  onModuleInit(): void {
    this.pollTimer = setInterval(() => {
      void this.pollOnce();
    }, POLL_INTERVAL_MS);
  }

  onModuleDestroy(): void {
    if (this.pollTimer !== undefined) {
      clearInterval(this.pollTimer);
      this.pollTimer = undefined;
    }
  }

  private async pollOnce(): Promise<void> {
    if (this.isPolling) {
      return;
    }

    this.isPolling = true;
    try {
      const attempts = await this.deliveryAttemptRepository.claimBatch(CLAIM_BATCH_SIZE);
      for (const attempt of attempts) {
        try {
          await this.processAttempt(attempt);
        } catch {
          // Leave the current row state untouched if a repository update fails mid-transition.
        }
      }
    } finally {
      this.isPolling = false;
    }
  }

  private async processAttempt(attempt: DeliveryAttempt): Promise<void> {
    const subscription = await this.deliverySubscriptionRepository.findByIdUnscoped(
      attempt.subscriptionId,
    );

    if (subscription === undefined) {
      await this.deliveryAttemptRepository.markFailed(
        attempt.id,
        nextRetryAtForAttempt(attempt.attemptCount),
      );
      return;
    }

    const body = buildRequestBody(attempt);
    const signature = signRequestBody(body, subscription.signingSecret);

    let response: Response;
    try {
      response = await fetch(subscription.url, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'X-Spool-Delivery-Id': attempt.id,
          'X-Spool-Signature': `sha256=${signature}`,
        },
        body,
        signal: AbortSignal.timeout(5_000),
        redirect: 'manual',
      });
    } catch {
      await this.deliveryAttemptRepository.markFailed(
        attempt.id,
        nextRetryAtForAttempt(attempt.attemptCount),
      );
      return;
    }

    if (response.ok) {
      await this.deliveryAttemptRepository.markSucceeded(attempt.id);
      return;
    }

    await this.deliveryAttemptRepository.markFailed(
      attempt.id,
      nextRetryAtForAttempt(attempt.attemptCount),
    );
  }
}
