import {
  BadRequestException,
  Body,
  Controller,
  Get,
  Headers,
  Param,
  Post,
  Query,
} from '@nestjs/common';
import { SessionTokenService } from '../auth/session-token.service.js';
import { verifySessionClaims } from '../auth/session-claims.helper.js';
import type { BranchResponse } from './branch-response.dto.js';
import { parseCreateBranchRequest } from './create-branch-request.dto.js';
import { BranchesService } from './branches.service.js';

function requireStakeholderId(value: string | undefined): string {
  if (typeof value !== 'string' || value.trim().length === 0) {
    throw new BadRequestException('stakeholderId query parameter must be a non-empty string');
  }
  return value;
}

@Controller('branches')
export class BranchesController {
  constructor(
    private readonly branches: BranchesService,
    private readonly sessionTokenService: SessionTokenService,
  ) {}

  @Post()
  async create(
    @Body() body: unknown,
    @Headers('x-workspace-id') workspaceId: string | undefined,
  ): Promise<BranchResponse> {
    const request = parseCreateBranchRequest(body);
    return this.branches.create(request, workspaceId);
  }

  @Post(':id/submit')
  async submit(
    @Param('id') id: string,
    @Headers('authorization') authorizationHeader: unknown,
    @Headers('x-workspace-id') workspaceId: string | undefined,
  ): Promise<BranchResponse> {
    const claims = verifySessionClaims(authorizationHeader, this.sessionTokenService);
    return this.branches.submit(id, workspaceId, claims);
  }

  @Post(':id/verify')
  async verify(
    @Param('id') id: string,
    @Headers('authorization') authorizationHeader: unknown,
    @Headers('x-workspace-id') workspaceId: string | undefined,
  ): Promise<BranchResponse> {
    const claims = verifySessionClaims(authorizationHeader, this.sessionTokenService);
    return this.branches.verify(id, workspaceId, claims);
  }

  @Post(':id/reject')
  async reject(
    @Param('id') id: string,
    @Headers('authorization') authorizationHeader: unknown,
    @Headers('x-workspace-id') workspaceId: string | undefined,
  ): Promise<BranchResponse> {
    const claims = verifySessionClaims(authorizationHeader, this.sessionTokenService);
    return this.branches.reject(id, workspaceId, claims);
  }

  @Post(':id/merge')
  async merge(
    @Param('id') id: string,
    @Headers('authorization') authorizationHeader: unknown,
    @Headers('x-workspace-id') workspaceId: string | undefined,
  ): Promise<BranchResponse> {
    const claims = verifySessionClaims(authorizationHeader, this.sessionTokenService);
    return this.branches.merge(id, workspaceId, claims);
  }

  @Get(':id')
  async findOne(
    @Param('id') id: string,
    @Headers('x-workspace-id') workspaceId: string | undefined,
    @Query('stakeholderId') stakeholderId: string | undefined,
  ): Promise<BranchResponse> {
    return this.branches.findById(id, workspaceId, requireStakeholderId(stakeholderId));
  }
}
