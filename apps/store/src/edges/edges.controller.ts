import { Body, Controller, Get, Headers, Param, Post } from '@nestjs/common';
import { SessionTokenService } from '../auth/session-token.service.js';
import { verifySessionClaims } from '../auth/session-claims.helper.js';
import type { EdgeResponse } from './edge-response.dto.js';
import { parseCreateEdgeRequest } from './create-edge-request.dto.js';
import { EdgesService } from './edges.service.js';

@Controller('edges')
export class EdgesController {
  constructor(
    private readonly edges: EdgesService,
    private readonly sessionTokenService: SessionTokenService,
  ) {}

  @Post()
  async create(
    @Body() body: unknown,
    @Headers('authorization') authorizationHeader: unknown,
    @Headers('x-workspace-id') workspaceId: string | undefined,
  ): Promise<EdgeResponse> {
    const claims = verifySessionClaims(authorizationHeader, this.sessionTokenService);
    const request = parseCreateEdgeRequest(body);
    return this.edges.create(request, workspaceId, claims);
  }

  @Get(':id')
  async findOne(
    @Param('id') id: string,
    @Headers('authorization') authorizationHeader: unknown,
    @Headers('x-workspace-id') workspaceId: string | undefined,
  ): Promise<EdgeResponse> {
    const claims = verifySessionClaims(authorizationHeader, this.sessionTokenService);
    return this.edges.findById(id, workspaceId, claims);
  }
}
