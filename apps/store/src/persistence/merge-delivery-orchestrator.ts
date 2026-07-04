/**
 * Post-commit orchestration boundary connecting branch merge (story S07's
 * `MergeRepository.mergeBranch`) to downstream push-delivery dispatch
 * (story S08's `MergeDeliveryDispatcher`).
 *
 * Sources of authority:
 * - Story S08 AC2: "A stakeholder can confirm that a branch merge completes
 *   without waiting for downstream push delivery to finish."
 * - Story S08 out-of-scope: "Merge transaction mechanics ... are out of
 *   scope for this story." This service does not alter `mergeBranch`'s
 *   transaction in any way — it calls the existing public method, waits for
 *   it to resolve (i.e. after `COMMIT` has already happened inside that
 *   method), and only *then* triggers dispatch. `mergeBranch` itself has no
 *   dependency on this service or on delivery at all.
 * - Technical spec §"Downstream delivery split" (`IDEA-63`): push delivery
 *   "must not block the merge transaction" — satisfied because
 *   `dispatchMergeCompleted` schedules its work via `setImmediate` and
 *   returns synchronously; this method returns the merge outcome to its
 *   caller without ever waiting on that scheduled work.
 */

import { Injectable } from '@nestjs/common';
import type { HumanActorContext } from '../domain/branch-lifecycle.js';
import type { BranchId, WorkspaceId } from '../domain/types/index.js';
import { MergeRepository, type MergeOutcome } from './merge.repository.js';
import { MergeDeliveryDispatcher } from './merge-delivery-dispatcher.js';

@Injectable()
export class MergeDeliveryOrchestrator {
  constructor(
    private readonly merges: MergeRepository,
    private readonly dispatcher: MergeDeliveryDispatcher,
  ) {}

  /**
   * Merges a branch and, once (and only once) that merge has committed,
   * triggers non-blocking downstream push-delivery dispatch for the merge's
   * discipline. Returns as soon as the merge itself resolves — it does not
   * wait for the dispatched delivery work to finish (AC2).
   */
  async mergeBranchAndDispatchDelivery(
    workspaceId: WorkspaceId,
    branchId: BranchId,
    actor: HumanActorContext,
  ): Promise<MergeOutcome> {
    const outcome = await this.merges.mergeBranch(workspaceId, branchId, actor);
    this.dispatcher.dispatchMergeCompleted({
      workspaceId: outcome.workspaceId,
      branchId: outcome.branchId,
      discipline: outcome.discipline,
      mergedAt: outcome.mergedAt,
    });
    return outcome;
  }
}
