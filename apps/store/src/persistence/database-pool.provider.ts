/**
 * NestJS provider wiring for the store's Postgres connection pool.
 *
 * Sources of authority:
 * - node-memory-management skill: "Use OnApplicationShutdown for resources
 *   that must be closed on SIGTERM or app.close()."
 * - Constitution II: containerized Postgres runtime dependency.
 */

import {
  Global,
  Inject,
  Injectable,
  Module,
  type OnApplicationShutdown,
  type Provider,
} from '@nestjs/common';
import { Pool } from 'pg';
import { loadDatabaseConfig } from './database-config.js';

/** DI token for the shared `pg.Pool` instance. */
export const PG_POOL = Symbol('PG_POOL');

export const pgPoolProvider: Provider = {
  provide: PG_POOL,
  useFactory: (): Pool => {
    const config = loadDatabaseConfig();
    return new Pool({
      host: config.host,
      port: config.port,
      user: config.user,
      password: config.password,
      database: config.database,
    });
  },
};

/**
 * Closes the shared pool on application shutdown so no connections are left
 * dangling when the Nest app stops.
 */
@Injectable()
export class DatabasePoolShutdown implements OnApplicationShutdown {
  constructor(@Inject(PG_POOL) private readonly pool: Pool) {}

  async onApplicationShutdown(): Promise<void> {
    await this.pool.end();
  }
}

@Global()
@Module({
  providers: [pgPoolProvider, DatabasePoolShutdown],
  exports: [PG_POOL],
})
export class DatabaseModule {}
