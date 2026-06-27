# Feature Module Spec Example

```typescript
import { NotFoundException } from '@nestjs/common';
import { Test, type TestingModule } from '@nestjs/testing';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ChunksController, ChunksService } from './feature-module.js';

describe('ChunksController', () => {
  let controller: ChunksController;
  let service: Pick<ChunksService, 'findById'>;

  beforeEach(async () => {
    const module: TestingModule = await Test.createTestingModule({
      controllers: [ChunksController],
      providers: [
        {
          provide: ChunksService,
          useValue: {
            findById: vi.fn(),
          } satisfies Pick<ChunksService, 'findById'>,
        },
      ],
    }).compile();

    controller = module.get(ChunksController);
    service = module.get(ChunksService);
  });

  it('returns the chunk from ChunksService', () => {
    const expected = { id: 'abc', content: 'Hello', status: 'draft' as const };
    vi.mocked(service.findById).mockReturnValue(expected);

    const result = controller.findOne('abc');

    expect(result).toEqual(expected);
    expect(service.findById).toHaveBeenCalledWith('abc');
  });

  it('propagates NotFoundException from the service', () => {
    vi.mocked(service.findById).mockImplementation(() => {
      throw new NotFoundException('Chunk abc not found');
    });

    expect(() => controller.findOne('abc')).toThrow(NotFoundException);
  });
});
```
