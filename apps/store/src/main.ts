import 'reflect-metadata';
import { NestFactory } from '@nestjs/core';
import { Pool } from 'pg';
import { AppModule } from './app.module.js';
import { loadDatabaseConfig } from './persistence/database-config.js';
import { runMigrations } from './persistence/migrator.js';

async function bootstrap(): Promise<void> {
  const migrationPool = new Pool(loadDatabaseConfig());
  try {
    await runMigrations(migrationPool);
  } finally {
    await migrationPool.end();
  }

  const application = await NestFactory.create(AppModule);
  const port = Number.parseInt(process.env.PORT ?? '3000', 10);

  await application.listen(port);
}

void bootstrap();
