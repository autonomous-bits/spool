# Store Unit Spec Example

Unit test for a NestJS service that depends on a repository provider. Demonstrates `Test.createTestingModule`, typed `Pick<>` mocks, `vi.fn<T>()`, and exception paths.

```typescript
import { NotFoundException } from '@nestjs/common';
import { Test, type TestingModule } from '@nestjs/testing';
import { beforeEach, describe, expect, it, vi } from 'vitest';

interface ChunkRecord {
  id: string;
  content: string;
  status: 'draft' | 'approved' | 'promoted';
}

class ChunksRepository {
  findById(_id: string): Promise<ChunkRecord | null> {
    throw new Error('not implemented');
  }

  save(_record: ChunkRecord): Promise<ChunkRecord> {
    throw new Error('not implemented');
  }
}

class ChunksService {
  constructor(private readonly repo: ChunksRepository) {}

  async findById(id: string): Promise<ChunkRecord> {
    const chunk = await this.repo.findById(id);
    if (chunk === null) {
      throw new NotFoundException(`Chunk ${id} not found`);
    }
    return chunk;
  }

  async approve(id: string): Promise<ChunkRecord> {
    const chunk = await this.findById(id);
    if (chunk.status !== 'draft') {
      throw new Error(`Chunk ${id} is not in draft status`);
    }
    return this.repo.save({ ...chunk, status: 'approved' });
  }
}

describe('ChunksService', () => {
  let service: ChunksService;
  const repo = {
    findById: vi.fn<ChunksRepository['findById']>(),
    save: vi.fn<ChunksRepository['save']>(),
  } satisfies Pick<ChunksRepository, 'findById' | 'save'>;

  beforeEach(async () => {
    repo.findById.mockReset();
    repo.save.mockReset();

    const module: TestingModule = await Test.createTestingModule({
      providers: [
        ChunksService,
        { provide: ChunksRepository, useValue: repo },
      ],
    }).compile();

    service = module.get(ChunksService);
  });

  it('returns a chunk by id', async () => {
    const chunk: ChunkRecord = { id: 'abc', content: 'Hello', status: 'draft' };
    repo.findById.mockResolvedValue(chunk);

    await expect(service.findById('abc')).resolves.toEqual(chunk);
    expect(repo.findById).toHaveBeenCalledWith('abc');
    expect(repo.findById).toHaveBeenCalledOnce();
  });

  it('throws NotFoundException when the repository returns null', async () => {
    repo.findById.mockResolvedValue(null);

    await expect(service.findById('missing')).rejects.toThrow(NotFoundException);
  });

  it('transitions a draft chunk to approved and persists it', async () => {
    const draft: ChunkRecord = { id: 'abc', content: 'Hello', status: 'draft' };
    const approved: ChunkRecord = { ...draft, status: 'approved' };

    repo.findById.mockResolvedValue(draft);
    repo.save.mockResolvedValue(approved);

    await expect(service.approve('abc')).resolves.toEqual(approved);
    expect(repo.save).toHaveBeenCalledWith(approved);
  });

  it('does not persist a chunk that is already approved', async () => {
    repo.findById.mockResolvedValue({ id: 'abc', content: 'Hello', status: 'approved' });

    await expect(service.approve('abc')).rejects.toThrow('not in draft status');
    expect(repo.save).not.toHaveBeenCalled();
  });
});
```
