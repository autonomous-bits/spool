# Security Bootstrap Example

```typescript
import 'reflect-metadata';
import { NestFactory } from '@nestjs/core';
import { AppModule } from './app.module.js';

const rawPort = process.env['PORT'];
const port = rawPort !== undefined ? Number(rawPort) : 3000;

if (Number.isNaN(port) || port < 1 || port > 65535) {
  throw new Error(`Invalid PORT "${rawPort}". Expected a number between 1 and 65535.`);
}

const app = await NestFactory.create(AppModule);

await app.listen(port);
```
