import { BadRequestException, Body, Controller, Get, Headers, Param, Post, Query } from '@nestjs/common';
import type { ChunkResponse } from './chunk-response.dto.js';
import { parseCreateChunkRequest } from './create-chunk-request.dto.js';
import { ChunksService } from './chunks.service.js';

function requireStakeholderId(value: string | undefined): string {
  if (typeof value !== 'string' || value.trim().length === 0) {
    throw new BadRequestException('stakeholderId query parameter must be a non-empty string');
  }
  return value;
}

@Controller('chunks')
export class ChunksController {
  constructor(private readonly chunks: ChunksService) {}

  @Post()
  async create(
    @Body() body: unknown,
    @Headers('x-workspace-id') workspaceId: string | undefined,
  ): Promise<ChunkResponse> {
    const request = parseCreateChunkRequest(body);
    return this.chunks.create(request, workspaceId);
  }

  @Get(':id')
  async findOne(
    @Param('id') id: string,
    @Headers('x-workspace-id') workspaceId: string | undefined,
    @Query('stakeholderId') stakeholderId: string | undefined,
  ): Promise<ChunkResponse> {
    return this.chunks.findById(id, workspaceId, requireStakeholderId(stakeholderId));
  }
}
