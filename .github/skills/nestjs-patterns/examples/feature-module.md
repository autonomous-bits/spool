# Feature Module Example

```typescript
import { Controller, Get, Injectable, Module, NotFoundException, Param } from '@nestjs/common';
import { randomUUID } from 'node:crypto';

type ChunkStatus = 'draft' | 'approved' | 'promoted';

interface Chunk {
  id: string;
  content: string;
  status: ChunkStatus;
}

interface ChunkResponse {
  id: string;
  content: string;
  status: ChunkStatus;
}

@Injectable()
export class ChunksService {
  private readonly store = new Map<string, Chunk>();

  create(content: string): ChunkResponse {
    const chunk: Chunk = { id: randomUUID(), content, status: 'draft' };
    this.store.set(chunk.id, chunk);
    return chunk satisfies ChunkResponse;
  }

  findById(id: string): ChunkResponse {
    const chunk = this.store.get(id);
    if (chunk === undefined) {
      throw new NotFoundException(`Chunk ${id} not found`);
    }
    return chunk satisfies ChunkResponse;
  }
}

@Controller('chunks')
export class ChunksController {
  constructor(private readonly chunks: ChunksService) {}

  @Get(':id')
  findOne(@Param('id') id: string): ChunkResponse {
    return this.chunks.findById(id);
  }
}

@Module({
  controllers: [ChunksController],
  providers: [ChunksService],
  exports: [ChunksService],
})
export class ChunksModule {}
```
