import { Body, Controller, Get, Param, Post } from '@nestjs/common';
import type { BranchResponse } from './branch-response.dto.js';
import { parseCreateBranchRequest } from './create-branch-request.dto.js';
import { BranchesService } from './branches.service.js';

@Controller('branches')
export class BranchesController {
  constructor(private readonly branches: BranchesService) {}

  @Post()
  async create(@Body() body: unknown): Promise<BranchResponse> {
    const request = parseCreateBranchRequest(body);
    return this.branches.create(request);
  }

  @Get(':id')
  async findOne(@Param('id') id: string): Promise<BranchResponse> {
    return this.branches.findById(id);
  }
}
