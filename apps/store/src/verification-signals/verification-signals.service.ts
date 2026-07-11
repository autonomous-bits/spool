import { ConflictException, ForbiddenException, Injectable, NotFoundException } from '@nestjs/common';
import type { SessionTokenClaims } from '../auth/session-token.service.js';
import { BranchLifecycleError } from '../domain/branch-lifecycle.js';
import { assertWorkspaceScope, WorkspaceScopeViolationError } from '../domain/workspace-scope.js';
import { VerificationSignalRepository } from '../persistence/verification-signal.repository.js';
import { WorkspaceRepository } from '../persistence/workspace.repository.js';
import type { CreateVerificationSignalRequest } from './create-verification-signal-request.dto.js';
import {
  toVerificationSignalResponse,
  type VerificationSignalResponse,
} from './verification-signal-response.dto.js';

/**
 * Application service for verification-signal submission and reads (Meridian IDEA-21/IDEA-31),
 * sitting between the HTTP controller and the `VerificationSignalRepository` persistence layer.
 * Meridian IDEA-139 now binds these routes to verified session tokens plus a live
 * `workspace_memberships` check against `claims.stakeholderId`; an unexpired token from a removed
 * member must still be rejected.
 *
 * Meridian IDEA-21 still keeps `verifierName` as untrusted free text describing whichever agent,
 * tool, or human produced the evaluation. Authenticated reporter identity is recorded separately as
 * `reportedByStakeholderId`, derived only from verified token claims and never from request body
 * fields.
 */
@Injectable()
export class VerificationSignalsService {
  constructor(
    private readonly verificationSignalRepository: VerificationSignalRepository,
    private readonly workspaceRepository: WorkspaceRepository,
  ) {}

  private async assertScope(
    headerWorkspaceId: string | null | undefined,
    claims: SessionTokenClaims,
  ): Promise<string> {
    const isMember =
      headerWorkspaceId === null ||
      headerWorkspaceId === undefined ||
      headerWorkspaceId.trim().length === 0
        ? false
        : await this.workspaceRepository.isMember(headerWorkspaceId, claims.stakeholderId);

    try {
      assertWorkspaceScope(headerWorkspaceId, { workspaceIdClaim: claims.workspaceId, isMember });
    } catch (error) {
      if (error instanceof WorkspaceScopeViolationError) {
        throw new ForbiddenException(error.message);
      }
      throw error;
    }

    return headerWorkspaceId;
  }

  /**
   * Submits a verification signal against a branch. Only allowed while the branch is `submitted`
   * or `verified` (Meridian IDEA-20/IDEA-43); never mutates the branch's own status.
   */
  async create(
    branchId: string,
    request: CreateVerificationSignalRequest,
    headerWorkspaceId: string | null | undefined,
    claims: SessionTokenClaims,
  ): Promise<VerificationSignalResponse> {
    const workspaceId = await this.assertScope(headerWorkspaceId, claims);

    const result = await this.verificationSignalRepository.create({
      branchId,
      workspaceId,
      reportedByStakeholderId: claims.stakeholderId,
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
   * Lists verification signals for a branch ordered oldest-first. Per Meridian IDEA-139 this read
   * path is authenticated exactly like writes; `verifierName` remains untrusted text, while
   * `reportedByStakeholderId` is the authenticated caller identity persisted at submission time.
   */
  async findAllForBranch(
    branchId: string,
    headerWorkspaceId: string | null | undefined,
    claims: SessionTokenClaims,
  ): Promise<VerificationSignalResponse[]> {
    const workspaceId = await this.assertScope(headerWorkspaceId, claims);

    const signals = await this.verificationSignalRepository.findByBranchId(branchId, workspaceId);
    return signals.map(toVerificationSignalResponse);
  }
}
