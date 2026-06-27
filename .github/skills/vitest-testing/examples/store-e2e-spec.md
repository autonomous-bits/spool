# Store E2E Spec Example

Integration test that boots the full `AppModule` without binding a real port. Use `overrideProvider()` for infrastructure providers that would otherwise require a database, queue, or external service.

File location: `apps/store/test/<feature>.e2e-spec.ts`

```typescript
import type { INestApplication } from '@nestjs/common';
import { Test, type TestingModule } from '@nestjs/testing';
import { afterAll, beforeAll, describe, expect, it, vi } from 'vitest';
import request from 'supertest';
import { AppModule } from '../src/app.module.js';

interface ChunkRecord {
  id: string;
  content: string;
  status: 'draft' | 'approved' | 'promoted';
}

class ChunksRepository {
  findById(_id: string): Promise<ChunkRecord | null> {
    throw new Error('not implemented');
  }
}

const chunksRepository = {
  findById: vi.fn<ChunksRepository['findById']>(),
} satisfies Pick<ChunksRepository, 'findById'>;

describe('Chunks e2e', () => {
  let app: INestApplication;

  beforeAll(async () => {
    const moduleRef: TestingModule = await Test.createTestingModule({
      imports: [AppModule],
    })
      .overrideProvider(ChunksRepository)
      .useValue(chunksRepository)
      .compile();

    app = moduleRef.createNestApplication();
    await app.init();
  });

  afterAll(async () => {
    await app.close();
  });

  it('GET /health returns 200 with store metadata', async () => {
    const response = await request(app.getHttpServer()).get('/health');

    expect(response.status).toBe(200);
    expect(response.body).toEqual({
      status: 'ok',
      service: 'store',
    });
  });

  it('GET /chunks/:id returns a chunk from the repository', async () => {
    chunksRepository.findById.mockResolvedValue({
      id: 'abc-123',
      content: 'Idea chunk content',
      status: 'draft',
    });

    const response = await request(app.getHttpServer()).get('/chunks/abc-123');

    expect(response.status).toBe(200);
    expect(response.body).toEqual({
      id: 'abc-123',
      content: 'Idea chunk content',
      status: 'draft',
    });
  });
});
```
