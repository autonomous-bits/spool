import { BadRequestException, Body, Controller, Get, Headers, Param, Post, Query } from '@nestjs/common';
import { SessionTokenService } from '../auth/session-token.service.js';
import { verifySessionClaims } from '../auth/session-claims.helper.js';
import type { ChunkResponse, NeighbourResponse } from './chunk-response.dto.js';
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
  constructor(
    private readonly chunks: ChunksService,
    private readonly sessionTokenService: SessionTokenService,
  ) {}

  @Get()
  async search(
    @Headers('authorization') authorizationHeader: unknown,
    @Headers('x-workspace-id') headerWorkspaceId: string | undefined,
    @Query('discipline') discipline?: string,
    @Query('type') chunkType?: string,
    @Query('status') status?: string,
    @Query('contextKind') contextKind?: string,
    @Query('branchId') branchId?: string,
    @Query('q') q?: string,
    @Query('limit') limitStr?: string,
    @Query('cursor') cursor?: string,
  ): Promise<{ chunks: ChunkResponse[]; nextCursor: string | null }> {
    const claims = verifySessionClaims(authorizationHeader, this.sessionTokenService);
    const limit = limitStr !== undefined ? parseInt(limitStr, 10) : 20;

    return this.chunks.search(
      {
        discipline,
        chunkType,
        status,
        contextKind,
        branchId,
        q,
        workspaceId: headerWorkspaceId,
      },
      isNaN(limit) ? 20 : limit,
      claims,
      cursor,
    );
  }

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

  @Get(':id/neighbourhood')
  async getNeighbourhood(
    @Param('id') id: string,
    @Headers('authorization') authorizationHeader: unknown,
    @Headers('x-workspace-id') headerWorkspaceId: string | undefined,
    @Query('depth') depthStr?: string,
    @Query('branchId') branchId?: string,
  ): Promise<{ chunk: ChunkResponse; neighbours: NeighbourResponse[] }> {
    const claims = verifySessionClaims(authorizationHeader, this.sessionTokenService);
    const depth = depthStr !== undefined ? parseInt(depthStr, 10) : 1;
    if (isNaN(depth) || depth < 1) {
      throw new BadRequestException('depth must be a positive integer');
    }
    if (depth > 5) {
      throw new BadRequestException('depth cannot exceed 5');
    }
    return this.chunks.getNeighbourhood(id, headerWorkspaceId, claims, depth, branchId);
  }
}
