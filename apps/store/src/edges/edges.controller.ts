import { BadRequestException, Body, Controller, Get, Headers, Param, Post, Query } from '@nestjs/common';
import type { EdgeResponse } from './edge-response.dto.js';
import { parseCreateEdgeRequest } from './create-edge-request.dto.js';
import { EdgesService } from './edges.service.js';

function requireStakeholderId(value: string | undefined): string {
  if (typeof value !== 'string' || value.trim().length === 0) {
    throw new BadRequestException('stakeholderId query parameter must be a non-empty string');
  }
  return value;
}

@Controller('edges')
export class EdgesController {
  constructor(private readonly edges: EdgesService) {}

  @Post()
  async create(
    @Body() body: unknown,
    @Headers('x-workspace-id') workspaceId: string | undefined,
  ): Promise<EdgeResponse> {
    const request = parseCreateEdgeRequest(body);
    return this.edges.create(request, workspaceId);
  }

  @Get(':id')
  async findOne(
    @Param('id') id: string,
    @Headers('x-workspace-id') workspaceId: string | undefined,
    @Query('stakeholderId') stakeholderId: string | undefined,
  ): Promise<EdgeResponse> {
    return this.edges.findById(id, workspaceId, requireStakeholderId(stakeholderId));
  }
}
