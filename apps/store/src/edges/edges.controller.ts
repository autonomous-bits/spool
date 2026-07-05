import { Body, Controller, Get, Param, Post } from '@nestjs/common';
import type { EdgeResponse } from './edge-response.dto.js';
import { parseCreateEdgeRequest } from './create-edge-request.dto.js';
import { EdgesService } from './edges.service.js';

@Controller('edges')
export class EdgesController {
  constructor(private readonly edges: EdgesService) {}

  @Post()
  async create(@Body() body: unknown): Promise<EdgeResponse> {
    const request = parseCreateEdgeRequest(body);
    return this.edges.create(request);
  }

  @Get(':id')
  async findOne(@Param('id') id: string): Promise<EdgeResponse> {
    return this.edges.findById(id);
  }
}
