import { ConflictException, ForbiddenException, Injectable, NotFoundException } from '@nestjs/common';
import { BranchLifecycleError } from '../domain/branch-lifecycle.js';
import { WorkspaceScopeViolationError } from '../domain/workspace-scope.js';
import { VerificationSignalRepository } from '../persistence/verification-signal.repository.js';
import type { CreateVerificationSignalRequest } from './create-verification-signal-request.dto.js';
import {
  toVerificationSignalResponse,
  type VerificationSignalResponse,
} from './verification-signal-response.dto.js';

/**
 * Application service for verification-signal submission and reads (Meridian IDEA-21/IDEA-43/
 * IDEA-31), sitting between the HTTP controller and the `VerificationSignalRepository`
 * persistence layer. Submission is deliberately unauthenticated (mirrors `POST /suggestions`
 * precedent): IDEA-21's "agents, tools, or other humans" is broader than the registered-
 * stakeholder set, so there is no ActorContext/session-token to verify here.
 *
 * G11 SG5 workspace scoping (interim resolution, Meridian gap-report IDEA-103): verification
 * signals have no `stakeholderId`/caller-identity concept (`verifierName` is intentionally
 * broad free text per IDEA-21), so neither the delegated tier's membership check nor the token
 * tier's claim check applies here. Instead this service only asserts the `X-Workspace-Id`
 * header is present; the actual scope enforcement -- comparing it against the target branch's
 * own `workspace_id` -- happens in `VerificationSignalRepository`, which folds the comparison
 * into the same atomic transaction/lookup as the rest of `create`/`findByBranchId`, and reuses
 * the existing `not_found` result kind so a mismatched workspace never leaks whether the branch
 * id exists.
 */
@Injectable()
export class VerificationSignalsService {
  constructor(private readonly verificationSignalRepository: VerificationSignalRepository) {}

  private assertWorkspaceHeaderPresent(headerWorkspaceId: string | null | undefined): string {
    if (
      headerWorkspaceId === null ||
      headerWorkspaceId === undefined ||
      headerWorkspaceId.trim().length === 0
    ) {
      throw new ForbiddenException(
        new WorkspaceScopeViolationError('missing X-Workspace-Id header').message,
      );
    }
    return headerWorkspaceId;
  }

  /**
   * Submits a verification signal against a branch. Only allowed while the branch is
   * `submitted` or `verified` (Meridian IDEA-20/IDEA-43); never mutates the branch's own status.
   */
  async create(
    branchId: string,
    request: CreateVerificationSignalRequest,
    headerWorkspaceId: string | null | undefined,
  ): Promise<VerificationSignalResponse> {
    const workspaceId = this.assertWorkspaceHeaderPresent(headerWorkspaceId);

    const result = await this.verificationSignalRepository.create({
      branchId,
      workspaceId,
      verifierName: request.verifierName,
      status: request.status,
      ...(request.reason === undefined ? {} : { reason: request.reason }),
    });

    switch (result.kind) {
      case 'not_found':
        throw new NotFoundException(`Branch ${branchId} not found`);
      case 'not_reviewable':
        throw new ConflictException(
          new BranchLifecycleError(
            `expected submitted or verified branch, received ${result.branchStatus}`,
          ).message,
        );
      case 'created':
        return toVerificationSignalResponse(result.signal);
    }
  }

  /**
   * Lists verification signals for a branch ordered oldest-first (G09 SG1). Deliberately
   * unauthenticated beyond the workspace-header check, matching this codebase's existing
   * GET /branches, GET /suggestions precedent of unauthenticated reads.
   */
  async findAllForBranch(
    branchId: string,
    headerWorkspaceId: string | null | undefined,
  ): Promise<VerificationSignalResponse[]> {
    const workspaceId = this.assertWorkspaceHeaderPresent(headerWorkspaceId);

    const signals = await this.verificationSignalRepository.findByBranchId(branchId, workspaceId);
    return signals.map(toVerificationSignalResponse);
  }
}
