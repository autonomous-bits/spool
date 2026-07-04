/**
 * Canonical, conflict-gated merge entrypoint.
 *
 * `MergeRepository.mergeBranch` (story S07) executes the atomic promotion of
 * a verified branch but deliberately does not itself invoke pre-merge
 * conflict detection (story S06's `ConflictDetectionRepository`) — that
 * separation was fine while nothing called `mergeBranch` in production, but
 * left the feature-01 technical spec's "merge branch" protected-operation
 * contract ("conflict checks against the divergence point") unenforced on
 * any real path (found during rubber-duck review comparing Feature 01/02
 * against the live Meridian workspace).
 *
 * This service is the fix: it is the one supported way to merge a branch.
 * It always runs `ConflictDetectionRepository.detectConflicts` first and
 * refuses to merge if any chunk, edge, or chunk-artifact-association change
 * was made independently on both sides since the branch diverged, before
 * ever calling `MergeRepository.mergeBranch`.
 *
 * Sources of authority:
 * - Feature-01 technical spec §"Protected operation contracts" — "Merge
 *   branch" requires "conflict checks against the divergence point".
 * - Feature-02 technical spec §"Required domain error categories" —
 *   "conflict-detection failures during merge must map to existing
 *   categories (for example, lineage violation or branch isolation
 *   violation) rather than introducing an ad hoc conflict error type." This
 *   service throws `EdgeLineageError('lineage-violation')`: a reported
 *   conflict is, by construction, an independent change to the same chunk,
 *   edge, or chunk-artifact-association identity on both branch and
 *   mainline since divergence — the same "would rewrite a fixed lineage
 *   identity" concern `lineage-violation` already covers for edges, widened
 *   here to the full conflict-detection scope (`IDEA-46`).
 * - `MergeRepository.mergeBranch` remains callable directly as a low-level
 *   primitive (existing tests and any caller that has already performed its
 *   own conflict check are unaffected), but `MergeDeliveryOrchestrator` and
 *   any future controller/MCP wiring must go through this service instead.
 */

import { Injectable } from '@nestjs/common';
import type { HumanActorContext } from '../domain/branch-lifecycle.js';
import { EdgeLineageError } from '../domain/edge-lineage.js';
import type { BranchId, WorkspaceId } from '../domain/types/index.js';
import { ConflictDetectionRepository } from './conflict-detection.repository.js';
import { MergeRepository, type MergeOutcome } from './merge.repository.js';

@Injectable()
export class ConflictGatedMergeService {
  constructor(
    private readonly conflictDetection: ConflictDetectionRepository,
    private readonly merges: MergeRepository,
  ) {}

  /**
   * Runs pre-merge conflict detection, then merges only if no conflicts are
   * reported. Throws `EdgeLineageError('lineage-violation')` if any chunk,
   * edge, or chunk-artifact-association change was made independently on
   * both branch and mainline since divergence. Otherwise delegates directly
   * to `MergeRepository.mergeBranch`, which still performs the atomic,
   * all-or-nothing promotion (unchanged by this service).
   */
  async mergeBranch(
    workspaceId: WorkspaceId,
    branchId: BranchId,
    actor: HumanActorContext,
  ): Promise<MergeOutcome> {
    const report = await this.conflictDetection.detectConflicts(workspaceId, branchId);
    const conflictCount =
      report.chunkConflicts.length +
      report.edgeConflicts.length +
      report.artifactAssociationConflicts.length;
    if (conflictCount > 0) {
      throw new EdgeLineageError(
        'lineage-violation',
        `branch '${branchId}' has ${conflictCount} independent mainline conflict(s) ` +
          `since divergence (${report.chunkConflicts.length} chunk, ` +
          `${report.edgeConflicts.length} edge, ` +
          `${report.artifactAssociationConflicts.length} artifact-association); ` +
          `resolve via catch-up before merging`,
      );
    }
    return this.merges.mergeBranch(workspaceId, branchId, actor);
  }
}
