import { Module } from '@nestjs/common';
import { PersistenceModule } from '../persistence/persistence.module.js';
import { AuthModule } from '../auth/auth.module.js';
import { ARTIFACT_DOWNLOAD_CONFIG } from './artifact-download-config.token.js';
import { loadArtifactDownloadConfig } from './artifact-download-config.js';
import { ArtifactDownloadTokenService } from './artifact-download-token.service.js';
import { ArtifactsController } from './artifacts.controller.js';
import { ArtifactsService } from './artifacts.service.js';

@Module({
  imports: [PersistenceModule, AuthModule],
  controllers: [ArtifactsController],
  providers: [
    { provide: ARTIFACT_DOWNLOAD_CONFIG, useValue: loadArtifactDownloadConfig() },
    ArtifactDownloadTokenService,
    ArtifactsService,
  ],
})
// eslint-disable-next-line @typescript-eslint/no-extraneous-class -- NestJS module classes are intentionally empty; behavior comes entirely from the @Module decorator.
export class ArtifactsModule {}
