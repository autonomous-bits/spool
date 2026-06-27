# Type-Safe Patterns

```typescript
import { randomUUID } from 'node:crypto';
import type { IncomingMessage } from 'node:http';

function firstOrDefault<T>(items: ReadonlyArray<T>, fallback: T): T {
  return items[0] ?? fallback;
}

interface ChunkOptions {
  maxTokens?: number;
}

function buildOptions(maxTokens: number | undefined): ChunkOptions {
  if (maxTokens === undefined) return {};
  return { maxTokens };
}

interface ChunkPayload {
  id: string;
  content: string;
}

function parseChunkPayload(raw: unknown): ChunkPayload {
  if (typeof raw !== 'object' || raw === null) {
    throw new TypeError('Payload must be an object');
  }
  const record = raw as Record<string, unknown>;
  if (typeof record['id'] !== 'string' || typeof record['content'] !== 'string') {
    throw new TypeError('Payload.id and payload.content must be strings');
  }
  return { id: record['id'], content: record['content'] };
}

type ChunkStatus = 'draft' | 'approved' | 'promoted';

interface StatusResponse {
  status: ChunkStatus;
  chunkId: string;
}

function buildStatusResponse(status: ChunkStatus): StatusResponse {
  return {
    status,
    chunkId: randomUUID(),
  } satisfies StatusResponse;
}

function handleRequest(_request: IncomingMessage, path: string): string {
  return `/api${path}`;
}
```
