/**
 * Unit test proving `ConflictGatedMergeService` is a true gate: it always
 * runs conflict detection before merging, refuses to merge when any
 * conflict is reported, and otherwise delegates straight through to
 * `MergeRepository.mergeBranch` unchanged.
 *
 * Sources of authority:
 * - Feature-01 technical spec §"Protected operation contracts" — "Merge
 *   branch" requires "conflict checks against the divergence point".
 * - Rubber-duck review of Feature 01/02 vs Meridian: found no production
 *   merge path enforced this contract.
 */

import { describe, expect, it, vi } from 'vitest';
import { ConflictGatedMergeService } from './conflict-gated-merge.service.js';
import type { ConflictDetectionRepository, ConflictReport } from './conflict-detection.repository.js';
import type { MergeRepository, MergeOutcome } from './merge.repository.js';
import type { HumanActorContext } from '../domain/branch-lifecycle.js';
import { EdgeLineageError } from '../domain/edge-lineage.js';
import type { BranchId, StakeholderId, WorkspaceId } from '../domain/types/index.js';

const workspaceId = 'ws-1' as WorkspaceId;
const branchId = 'branch-1' as BranchId;
const actor: HumanActorContext = {
  kind: 'human',
  stakeholderId: 'stakeholder-1' as StakeholderId,
};

const cleanReport: ConflictReport = {
  workspaceId,
  branchId,
  divergedAt: '2026-06-01T00:00:00.000Z' as ConflictReport['divergedAt'],
  chunkConflicts: [],
  edgeConflicts: [],
  artifactAssociationConflicts: [],
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

describe('ConflictGatedMergeService', () => {
  it('merges when detectConflicts reports no conflicts', async () => {
    const detectConflicts = vi.fn().mockResolvedValue(cleanReport);
    const mergeBranch = vi.fn().mockResolvedValue(outcome);
    const service = new ConflictGatedMergeService(
      { detectConflicts } as unknown as ConflictDetectionRepository,
      { mergeBranch } as unknown as MergeRepository,
    );

    const result = await service.mergeBranch(workspaceId, branchId, actor);

    expect(detectConflicts).toHaveBeenCalledWith(workspaceId, branchId);
    expect(mergeBranch).toHaveBeenCalledWith(workspaceId, branchId, actor);
    expect(result).toBe(outcome);
  });

  it.each([
    ['chunk', { ...cleanReport, chunkConflicts: [{ ideaLabel: 'IDEA-1' }] as never }],
    ['edge', { ...cleanReport, edgeConflicts: [{ sourceLabel: 'IDEA-1' }] as never }],
    [
      'artifact-association',
      { ...cleanReport, artifactAssociationConflicts: [{ chunkLabel: 'IDEA-1' }] as never },
    ],
  ])('refuses to merge when a %s conflict is reported', async (_kind, report) => {
    const detectConflicts = vi.fn().mockResolvedValue(report);
    const mergeBranch = vi.fn();
    const service = new ConflictGatedMergeService(
      { detectConflicts } as unknown as ConflictDetectionRepository,
      { mergeBranch } as unknown as MergeRepository,
    );

    await expect(service.mergeBranch(workspaceId, branchId, actor)).rejects.toMatchObject({
      constructor: EdgeLineageError,
      code: 'lineage-violation',
    });
    expect(mergeBranch).not.toHaveBeenCalled();
  });

  it('propagates a detectConflicts failure without ever calling mergeBranch', async () => {
    const detectionError = new Error('branch not registered');
    const detectConflicts = vi.fn().mockRejectedValue(detectionError);
    const mergeBranch = vi.fn();
    const service = new ConflictGatedMergeService(
      { detectConflicts } as unknown as ConflictDetectionRepository,
      { mergeBranch } as unknown as MergeRepository,
    );

    await expect(service.mergeBranch(workspaceId, branchId, actor)).rejects.toBe(detectionError);
    expect(mergeBranch).not.toHaveBeenCalled();
  });
});
