import { Body, Controller, Delete, Headers, Param, Post } from '@nestjs/common';
import { verifySessionClaims } from '../auth/session-claims.helper.js';
import { SessionTokenService } from '../auth/session-token.service.js';
import {
  parseAssignDisciplineRequest,
  parseDisciplineParam,
} from './assign-discipline-request.dto.js';
import { parseAddMemberRequest } from './add-member-request.dto.js';
import { parseCreateWorkspaceRequest } from './create-workspace-request.dto.js';
import type { StakeholderDisciplineResponse } from './stakeholder-discipline-response.dto.js';
import type { WorkspaceMembershipResponse } from './workspace-membership-response.dto.js';
import type { WorkspaceResponse } from './workspace-response.dto.js';
import { WorkspacesService } from './workspaces.service.js';

/**
 * Human-only workspace registry endpoints (Meridian IDEA-94). No MCP tool is exposed for either
 * endpoint (mirrors the human-only precedent for branch submit/verify/merge, Meridian IDEA-81).
 *
 * G11 SG4 (Meridian IDEA-98/IDEA-100): `POST /workspaces` requires no `X-Workspace-Id` header at
 * all (creating a workspace necessarily precedes membership in it). `POST /workspaces/:id/members`
 * is already token-gated, so it additionally requires `X-Workspace-Id` to equal the `:id` route
 * param, validated against the token's workspaceId claim (the token tier).
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
    @Headers('x-workspace-id') workspaceId: string | undefined,
  ): Promise<WorkspaceMembershipResponse> {
    const claims = verifySessionClaims(authorizationHeader, this.sessionTokenService);
    const request = parseAddMemberRequest(body);
    return this.workspaces.addMember(id, request.stakeholderId, workspaceId, claims);
  }

  @Post(':id/stakeholders/:stakeholderId/disciplines')
  async assignDiscipline(
    @Param('id') id: string,
    @Param('stakeholderId') stakeholderId: string,
    @Body() body: unknown,
    @Headers('authorization') authorizationHeader: unknown,
    @Headers('x-workspace-id') workspaceId: string | undefined,
  ): Promise<StakeholderDisciplineResponse> {
    const claims = verifySessionClaims(authorizationHeader, this.sessionTokenService);
    const request = parseAssignDisciplineRequest(body);
    return this.workspaces.assignDiscipline(
      id,
      stakeholderId,
      request.discipline,
      workspaceId,
      claims,
    );
  }

  @Delete(':id/stakeholders/:stakeholderId/disciplines/:discipline')
  async revokeDiscipline(
    @Param('id') id: string,
    @Param('stakeholderId') stakeholderId: string,
    @Param('discipline') discipline: unknown,
    @Headers('authorization') authorizationHeader: unknown,
    @Headers('x-workspace-id') workspaceId: string | undefined,
  ): Promise<void> {
    const claims = verifySessionClaims(authorizationHeader, this.sessionTokenService);
    return this.workspaces.revokeDiscipline(
      id,
      stakeholderId,
      parseDisciplineParam(discipline),
      workspaceId,
      claims,
    );
  }
}
