import { Body, Controller, Headers, Param, Post } from '@nestjs/common';
import { verifySessionClaims } from '../auth/session-claims.helper.js';
import { SessionTokenService } from '../auth/session-token.service.js';
import { parseAddMemberRequest } from './add-member-request.dto.js';
import { parseCreateWorkspaceRequest } from './create-workspace-request.dto.js';
import type { WorkspaceMembershipResponse } from './workspace-membership-response.dto.js';
import type { WorkspaceResponse } from './workspace-response.dto.js';
import { WorkspacesService } from './workspaces.service.js';

/**
 * Human-only workspace registry endpoints (Meridian IDEA-94). No MCP tool is exposed for either
 * endpoint (mirrors the human-only precedent for branch submit/verify/merge, Meridian IDEA-81).
 */
@Controller('workspaces')
export class WorkspacesController {
  constructor(
    private readonly workspaces: WorkspacesService,
    private readonly sessionTokenService: SessionTokenService,
  ) {}

  @Post()
  async create(
    @Body() body: unknown,
    @Headers('authorization') authorizationHeader: unknown,
  ): Promise<WorkspaceResponse> {
    const claims = verifySessionClaims(authorizationHeader, this.sessionTokenService);
    const request = parseCreateWorkspaceRequest(body);
    return this.workspaces.create(request, claims);
  }

  @Post(':id/members')
  async addMember(
    @Param('id') id: string,
    @Body() body: unknown,
    @Headers('authorization') authorizationHeader: unknown,
  ): Promise<WorkspaceMembershipResponse> {
    const claims = verifySessionClaims(authorizationHeader, this.sessionTokenService);
    const request = parseAddMemberRequest(body);
    return this.workspaces.addMember(id, request.stakeholderId, claims);
  }
}
