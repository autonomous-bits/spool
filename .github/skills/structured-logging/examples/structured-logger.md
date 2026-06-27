# Structured Logger Example

```typescript
import { Injectable, Logger } from '@nestjs/common';

@Injectable()
export class ChunksService {
  private readonly logger = new Logger(ChunksService.name);

  async approve(chunkId: string, tenantId: string): Promise<void> {
    this.logger.debug(`approve called chunkId=${chunkId} tenantId=${tenantId}`);

    try {
      this.logger.log(`Chunk approved chunkId=${chunkId} tenantId=${tenantId}`);
    } catch (err) {
      const reason = err instanceof Error ? err.message : String(err);
      this.logger.error(
        `Chunk approval failed chunkId=${chunkId} tenantId=${tenantId} reason=${reason}`,
      );
      throw err;
    }
  }
}

type LogLevel = 'debug' | 'info' | 'warn' | 'error';

interface LogEntry {
  level: LogLevel;
  msg: string;
  ts: string;
  [key: string]: unknown;
}

function emitLog(level: LogLevel, msg: string, extra?: Record<string, unknown>): void {
  const entry: LogEntry = { level, msg, ts: new Date().toISOString(), ...extra };
  process.stderr.write(JSON.stringify(entry) + '\n');
}
```
