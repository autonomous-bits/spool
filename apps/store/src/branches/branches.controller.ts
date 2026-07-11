import {
  Body,
  Controller,
  Get,
  Headers,
  Param,
  Post,
} from '@nestjs/common';
import { SessionTokenService } from '../auth/session-token.service.js';
import { verifySessionClaims } from '../auth/session-claims.helper.js';
import type { BranchResponse } from './branch-response.dto.js';
import { parseCreateBranchRequest } from './create-branch-request.dto.js';
import { BranchesService } from './branches.service.js';

@Controller('branches')
export class BranchesController {
  constructor(
    private readonly branches: BranchesService,
    private readonly sessionTokenService: SessionTokenService,
  ) {}

  @Post()
  async create(
    @Body() body: unknown,
    @Headers('authorization') authorizationHeader: unknown,
    @Headers('x-workspace-id') workspaceId: string | undefined,
  ): Promise<BranchResponse> {
    const claims = verifySessionClaims(authorizationHeader, this.sessionTokenService);
    const request = parseCreateBranchRequest(body);
    return this.branches.create(request, workspaceId, claims);
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
    @Headers('authorization') authorizationHeader: unknown,
    @Headers('x-workspace-id') workspaceId: string | undefined,
  ): Promise<BranchResponse> {
    const claims = verifySessionClaims(authorizationHeader, this.sessionTokenService);
    return this.branches.findById(id, workspaceId, claims);
  }
}
