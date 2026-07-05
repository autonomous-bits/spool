import { Injectable, Module, type OnApplicationShutdown } from '@nestjs/common';
import { Pool } from 'pg';
import { BranchRepository } from './branch.repository.js';
import { ChunkRepository } from './chunk.repository.js';
import { EdgeRepository } from './edge.repository.js';
import { loadDatabaseConfig } from './database-config.js';
import { PG_POOL } from './pg-pool.token.js';

/**
 * Wraps the shared `pg.Pool` in an injectable provider that closes on application shutdown, per
 * the node-memory-management guidance for long-lived resources (OnApplicationShutdown, not a
 * manual process hook).
 */
@Injectable()
class PgPoolProvider implements OnApplicationShutdown {
  readonly pool = new Pool(loadDatabaseConfig());

  async onApplicationShutdown(): Promise<void> {
    await this.pool.end();
  }
}

@Module({
  providers: [
    PgPoolProvider,
    {
      provide: PG_POOL,
      useFactory: (provider: PgPoolProvider) => provider.pool,
      inject: [PgPoolProvider],
    },
    ChunkRepository,
    BranchRepository,
    EdgeRepository,
  ],
  exports: [PG_POOL, ChunkRepository, BranchRepository, EdgeRepository],
})
// eslint-disable-next-line @typescript-eslint/no-extraneous-class -- NestJS module classes are intentionally empty; behavior comes entirely from the @Module decorator.
export class PersistenceModule {}
