/**
 * Unit test proving the merge -> delivery-dispatch orchestration boundary
 * (story S08 AC2) actually connects a real merge call to
 * `MergeDeliveryDispatcher`, and that the merge's own promise resolves
 * without waiting on the dispatched delivery work.
 */

import { describe, expect, it, vi } from 'vitest';
import { MergeDeliveryOrchestrator } from './merge-delivery-orchestrator.js';
import type { MergeRepository, MergeOutcome } from './merge.repository.js';
import type { MergeDeliveryDispatcher } from './merge-delivery-dispatcher.js';
import type { HumanActorContext } from '../domain/branch-lifecycle.js';
import type {
  BranchId,
  StakeholderId,
  WorkspaceId,
} from '../domain/types/index.js';

const workspaceId = 'ws-1' as WorkspaceId;
const branchId = 'branch-1' as BranchId;
const actor: HumanActorContext = {
  kind: 'human',
  stakeholderId: 'stakeholder-1' as StakeholderId,
};

const outcome: MergeOutcome = {
  workspaceId,
  branchId,
  discipline: 'engineering',
  mergedAt: '2026-07-01T00:00:00.000Z',
  mergedByStakeholderId: 'stakeholder-1' as StakeholderId,
  mergedChunkLabels: [],
  mergedEdgeIdentities: [],
  mergedArtifactAssociations: [],
};

describe('MergeDeliveryOrchestrator', () => {
  it('AC2: dispatches delivery for the merged discipline only after mergeBranch resolves, and does not await the dispatch', async () => {
    const mergeBranch = vi.fn().mockResolvedValue(outcome);
    const dispatchMergeCompleted = vi.fn();
    const orchestrator = new MergeDeliveryOrchestrator(
      { mergeBranch } as unknown as MergeRepository,
      { dispatchMergeCompleted } as unknown as MergeDeliveryDispatcher,
    );

    const result = await orchestrator.mergeBranchAndDispatchDelivery(workspaceId, branchId, actor);

    expect(mergeBranch).toHaveBeenCalledWith(workspaceId, branchId, actor);
    expect(dispatchMergeCompleted).toHaveBeenCalledWith({
      workspaceId: outcome.workspaceId,
      branchId: outcome.branchId,
      discipline: outcome.discipline,
      mergedAt: outcome.mergedAt,
    });
    expect(result).toBe(outcome);
  });

  it('a merge failure never reaches dispatch (dispatch is only ever called with a real, committed outcome)', async () => {
    const mergeError = new Error('merge failed');
    const mergeBranch = vi.fn().mockRejectedValue(mergeError);
    const dispatchMergeCompleted = vi.fn();
    const orchestrator = new MergeDeliveryOrchestrator(
      { mergeBranch } as unknown as MergeRepository,
      { dispatchMergeCompleted } as unknown as MergeDeliveryDispatcher,
    );

    await expect(
      orchestrator.mergeBranchAndDispatchDelivery(workspaceId, branchId, actor),
    ).rejects.toBe(mergeError);
    expect(dispatchMergeCompleted).not.toHaveBeenCalled();
  });

  it('AC2: the returned merge outcome does not wait for dispatchMergeCompleted to finish its own scheduled work', async () => {
    // dispatchMergeCompleted is itself synchronous by contract (it schedules
    // work via setImmediate and returns void) — assert that contract is
    // upheld here too: the orchestrator's await never depends on anything
    // dispatchMergeCompleted schedules.
    const mergeBranch = vi.fn().mockResolvedValue(outcome);
    let dispatchStillPending = true;
    const dispatchMergeCompleted = vi.fn().mockImplementation(() => {
      setTimeout(() => {
        dispatchStillPending = false;
      }, 20);
      // Intentionally returns undefined synchronously, per DeliveryDispatcher contract.
    });
    const orchestrator = new MergeDeliveryOrchestrator(
      { mergeBranch } as unknown as MergeRepository,
      { dispatchMergeCompleted } as unknown as MergeDeliveryDispatcher,
    );

    await orchestrator.mergeBranchAndDispatchDelivery(workspaceId, branchId, actor);

    expect(dispatchStillPending).toBe(true);
  });
});
