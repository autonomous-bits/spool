# Stream Pipeline Example

```typescript
import { Injectable, type OnApplicationShutdown } from '@nestjs/common';
import { Transform } from 'node:stream';
import { pipeline } from 'node:stream/promises';

interface ChunkRecord {
  id: string;
  content: string;
  status: 'draft' | 'approved' | 'promoted';
}

class ApprovedFilter extends Transform {
  constructor() {
    super({ objectMode: true });
  }

  override _transform(
    chunk: ChunkRecord,
    _encoding: BufferEncoding,
    callback: () => void,
  ): void {
    if (chunk.status === 'approved' || chunk.status === 'promoted') {
      this.push(chunk);
    }
    callback();
  }
}

@Injectable()
export class DocumentProjectionService implements OnApplicationShutdown {
  private readonly activeControllers = new Set<AbortController>();

  async streamProjection(
    source: NodeJS.ReadableStream,
    destination: NodeJS.WritableStream,
  ): Promise<void> {
    const controller = new AbortController();
    this.activeControllers.add(controller);

    try {
      await pipeline(source, new ApprovedFilter(), destination, {
        signal: controller.signal,
      });
    } finally {
      this.activeControllers.delete(controller);
    }
  }

  onApplicationShutdown(): void {
    for (const controller of this.activeControllers) {
      controller.abort();
    }
    this.activeControllers.clear();
  }
}

declare class ChunkGraph {}
declare class DocumentProjection {}
declare function computeProjection(graph: ChunkGraph): DocumentProjection;

const projectionCache = new WeakMap<ChunkGraph, DocumentProjection>();

function getCachedProjection(graph: ChunkGraph): DocumentProjection {
  const cached = projectionCache.get(graph);
  if (cached !== undefined) return cached;
  const result = computeProjection(graph);
  projectionCache.set(graph, result);
  return result;
}
```
