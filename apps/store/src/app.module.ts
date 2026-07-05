import { Module } from '@nestjs/common';
import { BranchesModule } from './branches/branches.module.js';
import { ChunksModule } from './chunks/chunks.module.js';
import { EdgesModule } from './edges/edges.module.js';
import { HealthController } from './health.controller.js';
import { PersistenceModule } from './persistence/persistence.module.js';

@Module({
  imports: [PersistenceModule, ChunksModule, BranchesModule, EdgesModule],
  controllers: [HealthController],
})
// eslint-disable-next-line @typescript-eslint/no-extraneous-class -- NestJS module classes are intentionally empty; behavior comes entirely from the @Module decorator.
export class AppModule {}
