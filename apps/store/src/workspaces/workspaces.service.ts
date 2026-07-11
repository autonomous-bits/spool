import {
  BadRequestException,
  ConflictException,
  ForbiddenException,
  Injectable,
  NotFoundException,
} from '@nestjs/common';
import type { SessionTokenClaims } from '../auth/session-token.service.js';
import { Workspace } from '../domain/workspace.js';
import { WorkspaceMembershipAlreadyExistsError } from '../domain/workspace-membership.js';
import { assertWorkspaceScope, WorkspaceScopeViolationError } from '../domain/workspace-scope.js';
import { StakeholderRepository } from '../persistence/stakeholder.repository.js';
import {
  WorkspaceRepository,
  type WorkspaceAddMemberResult,
} from '../persistence/workspace.repository.js';
import type { CreateWorkspaceRequest } from './create-workspace-request.dto.js';
import { toWorkspaceResponse, type WorkspaceResponse } from './workspace-response.dto.js';
import {
  toWorkspaceMembershipResponse,
  type WorkspaceMembershipResponse,
} from './workspace-membership-response.dto.js';

const FOREIGN_KEY_VIOLATION = '23503';

function isPgErrorWithCode(error: unknown, code: string): boolean {
  return (
    typeof error === 'object' &&
    error !== null &&
    'code' in error &&
    (error).code === code
  );
}

/**
 * Application service for workspace creation and direct-add membership (Meridian IDEA-94/
 * IDEA-88/IDEA-95), sitting between the HTTP controller and the persistence/domain layers. Both
 * operations require a verified human session token (Meridian IDEA-81) — the acting stakeholder
 * comes from `claims.stakeholderId`, never a caller-declared body field.
 */
@Injectable()
export class WorkspacesService {
  constructor(
    private readonly workspaceRepository: WorkspaceRepository,
    private readonly stakeholderRepository: StakeholderRepository,
  ) {}

  async create(
    request: CreateWorkspaceRequest,
    claims: SessionTokenClaims,
  ): Promise<WorkspaceResponse> {
    let workspace: Workspace;
    try {
      workspace = new Workspace({
        name: request.name,
        createdByStakeholderId: claims.stakeholderId,
      });
    } catch (error) {
      // Domain invariants (blank name/createdByStakeholderId) surfaced as 400s, not 500s.
      const message = error instanceof Error ? error.message : 'Invalid workspace';
      throw new BadRequestException(message);
    }

    try {
      const created = await this.workspaceRepository.createWithFirstMember(workspace);
      return toWorkspaceResponse(created);
    } catch (error) {
      if (isPgErrorWithCode(error, FOREIGN_KEY_VIOLATION)) {
        throw new BadRequestException(`Unknown stakeholderId: ${claims.stakeholderId}`);
      }
      throw error;
    }
  }

  async addMember(
    workspaceId: string,
    targetStakeholderId: string,
    headerWorkspaceId: string | null | undefined,
    claims: SessionTokenClaims,
  ): Promise<WorkspaceMembershipResponse> {
    // G11 SG4 (Meridian IDEA-98/IDEA-100): this route is already token-gated, so X-Workspace-Id
    // must both match the token's workspaceId claim (the token tier) AND name the same workspace
    // as the :id route param — otherwise a caller with a token bound to workspace A could target
    // /workspaces/B/members while merely asserting X-Workspace-Id: A.
    try {
      assertWorkspaceScope(headerWorkspaceId, { tier: 'token', workspaceIdClaim: claims.workspaceId });
    } catch (error) {
      if (error instanceof WorkspaceScopeViolationError) {
        throw new ForbiddenException(error.message);
      }
      throw error;
    }
    if (headerWorkspaceId !== workspaceId) {
      throw new ForbiddenException(
        `X-Workspace-Id ${headerWorkspaceId} does not match the target workspace ${workspaceId}`,
      );
    }

    const targetStakeholder = await this.stakeholderRepository.findById(targetStakeholderId);
    if (targetStakeholder === undefined) {
      throw new NotFoundException(`Stakeholder ${targetStakeholderId} not found`);
    }

    let result: WorkspaceAddMemberResult;
    try {
      result = await this.workspaceRepository.addMember(
        workspaceId,
        claims.stakeholderId,
        targetStakeholderId,
      );
    } catch (error) {
      if (error instanceof WorkspaceMembershipAlreadyExistsError) {
        throw new ConflictException(error.message);
      }
      throw error;
    }

    switch (result.kind) {
      case 'workspace_not_found':
        throw new NotFoundException(`Workspace ${workspaceId} not found`);
      case 'caller_not_member':
        throw new ForbiddenException(
          `Stakeholder ${claims.stakeholderId} is not a member of workspace ${workspaceId}`,
        );
      case 'added':
        return toWorkspaceMembershipResponse(result.membership);
    }
  }
}
