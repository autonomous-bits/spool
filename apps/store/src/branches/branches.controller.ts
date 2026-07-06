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
  async create(@Body() body: unknown): Promise<BranchResponse> {
    const request = parseCreateBranchRequest(body);
    return this.branches.create(request);
  }

  @Post(':id/submit')
  async submit(
    @Param('id') id: string,
    @Headers('authorization') authorizationHeader: unknown,
  ): Promise<BranchResponse> {
    const claims = verifySessionClaims(authorizationHeader, this.sessionTokenService);
    return this.branches.submit(id, claims);
  }

  @Post(':id/verify')
  async verify(
    @Param('id') id: string,
    @Headers('authorization') authorizationHeader: unknown,
  ): Promise<BranchResponse> {
    const claims = verifySessionClaims(authorizationHeader, this.sessionTokenService);
    return this.branches.verify(id, claims);
  }

  @Post(':id/reject')
  async reject(
    @Param('id') id: string,
    @Headers('authorization') authorizationHeader: unknown,
  ): Promise<BranchResponse> {
    const claims = verifySessionClaims(authorizationHeader, this.sessionTokenService);
    return this.branches.reject(id, claims);
  }

  @Post(':id/merge')
  async merge(
    @Param('id') id: string,
    @Headers('authorization') authorizationHeader: unknown,
  ): Promise<BranchResponse> {
    const claims = verifySessionClaims(authorizationHeader, this.sessionTokenService);
    return this.branches.merge(id, claims);
  }

  @Get(':id')
  async findOne(@Param('id') id: string): Promise<BranchResponse> {
    return this.branches.findById(id);
  }
}
