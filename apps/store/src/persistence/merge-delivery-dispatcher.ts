/**
 * Asynchronous, non-blocking dispatch boundary for push delivery triggered
 * by a branch merge (story S08).
 *
 * Sources of authority:
 * - Story S08 AC2: "A stakeholder can confirm that a branch merge completes
 *   without waiting for downstream push delivery to finish." AC3: on-demand
 *   pull access must be available independently of push success.
 * - Story S08 out-of-scope: "Merge transaction mechanics ... are out of
 *   scope for this story." This dispatcher is therefore deliberately
 *   decoupled from `MergeRepository.mergeBranch` — it is not invoked from,
 *   and does not import, `merge.repository.ts`. A future story that wires
 *   API/MCP-level merge orchestration is expected to call
 *   `dispatchMergeCompleted` with the already-committed merge outcome,
 *   after `mergeBranch` has resolved.
 * - Technical spec §"Downstream delivery split" (`IDEA-63`): "Real-time push
 *   delivery of merged, visibility-resolved graph updates must run as an
 *   asynchronous background process triggered by branch merge events; it
 *   must not block the merge transaction." Meridian `IDEA-63` (verified live):
 *   "A background queue worker that processes branch merge events and
 *   pushes visibility-resolved graph updates to registered downstream
 *   containers, external webhooks, and egress lanes." `dispatchMergeCompleted`
 *   models that queue-worker boundary: it schedules the actual subscription
 *   lookup and push attempts on a later event-loop tick (`setImmediate`) and
 *   returns synchronously, so a caller is never blocked waiting for it.
 * - Technical spec §"Delivery subscription persistence" (`IDEA-65`): a push
 *   failure must never affect the underlying subscription record. Each
 *   push attempt here is isolated via `Promise.allSettled`, and this class
 *   never writes to `delivery_subscriptions` — only `DeliverySubscriptionRepository`
 *   does.
 *
 * A full durable outbox/retry-queue implementation (surviving a process
 * crash between merge-commit and dispatch) is a larger concern not
 * specified by any source of authority available to this story; this
 * in-process scheduler satisfies "runs asynchronously rather than inside
 * the merge transaction" without overreaching into that scope.
 */

import { Inject, Injectable, Logger } from '@nestjs/common';
import type { BranchId, Discipline, WorkspaceId } from '../domain/types/index.js';
import {
  DeliverySubscriptionRepository,
  type PersistedDeliverySubscription,
} from './delivery-subscription.repository.js';

export interface MergeDeliveryEvent {
  readonly workspaceId: WorkspaceId;
  readonly branchId: BranchId;
  readonly discipline: Discipline;
  readonly mergedAt: string;
}

/**
 * Pluggable push transport. Production wiring supplies a real
 * webhook/egress client; tests supply a fake. Failures thrown/rejected here
 * must never propagate back to the merge caller (see class-level doc).
 */
export interface DeliveryPushPort {
  push(subscription: PersistedDeliverySubscription, event: MergeDeliveryEvent): Promise<void>;
}

export const DELIVERY_PUSH_PORT = Symbol('DELIVERY_PUSH_PORT');

@Injectable()
export class MergeDeliveryDispatcher {
  private readonly logger = new Logger(MergeDeliveryDispatcher.name);

  constructor(
    private readonly subscriptions: DeliverySubscriptionRepository,
    @Inject(DELIVERY_PUSH_PORT) private readonly pushPort: DeliveryPushPort,
  ) {}

  /**
   * Schedules push delivery for a completed merge and returns immediately
   * (AC2) — it does not return a `Promise` a caller could mistakenly
   * `await`, and the actual subscription lookup/push work runs on a later
   * event-loop tick via `setImmediate`, after the caller's current
   * synchronous work (including any merge-transaction commit) has already
   * finished.
   *
   * Every failure in the scheduled work — including a subscription-lookup
   * failure, not just an individual push rejection — is caught and logged
   * here rather than left as an unhandled rejection, so a problem in the
   * delivery path can never crash the process or surface back to whichever
   * caller triggered the merge (AC2/AC3).
   */
  dispatchMergeCompleted(event: MergeDeliveryEvent): void {
    setImmediate(() => {
      this.processMergeCompleted(event).catch((error: unknown) => {
        const reason = error instanceof Error ? error.message : String(error);
        this.logger.error(
          `merge delivery dispatch failed workspaceId=${event.workspaceId} branchId=${event.branchId} reason=${reason}`,
        );
      });
    });
  }

  private async processMergeCompleted(event: MergeDeliveryEvent): Promise<void> {
    const matching = await this.subscriptions.listSubscriptionsForDiscipline(
      event.workspaceId,
      event.discipline,
    );
    // Isolate each push attempt: one consumer's failure must never affect
    // another's delivery, and no failure here ever touches subscription
    // persistence (AC1/AC3).
    const results = await Promise.allSettled(
      matching.map((subscription) => this.pushPort.push(subscription, event)),
    );
    for (const [index, result] of results.entries()) {
      if (result.status === 'rejected') {
        const reason = result.reason instanceof Error ? result.reason.message : String(result.reason);
        this.logger.warn(
          `push delivery failed consumerId=${matching[index]!.consumerId} workspaceId=${event.workspaceId} reason=${reason}`,
        );
      }
    }
  }
}
