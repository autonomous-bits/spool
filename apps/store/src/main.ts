import 'reflect-metadata';
import { NestFactory } from '@nestjs/core';
import { AppModule } from './app.module.js';

async function bootstrap(): Promise<void> {
  const application = await NestFactory.create(AppModule);
  const port = Number.parseInt(process.env.PORT ?? '3000', 10);

  await application.listen(port);
}

void bootstrap();
