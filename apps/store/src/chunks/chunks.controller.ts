import { Body, Controller, Get, Param, Post } from '@nestjs/common';
import type { ChunkResponse } from './chunk-response.dto.js';
import { parseCreateChunkRequest } from './create-chunk-request.dto.js';
import { ChunksService } from './chunks.service.js';

@Controller('chunks')
export class ChunksController {
  constructor(private readonly chunks: ChunksService) {}

  @Post()
  async create(@Body() body: unknown): Promise<ChunkResponse> {
    const request = parseCreateChunkRequest(body);
    return this.chunks.create(request);
  }

  @Get(':id')
  async findOne(@Param('id') id: string): Promise<ChunkResponse> {
    return this.chunks.findById(id);
  }
}
