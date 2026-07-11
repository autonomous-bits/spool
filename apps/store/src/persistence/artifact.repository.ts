import { randomUUID } from 'node:crypto';
import type { Pool, QueryResult, QueryResultRow } from 'pg';
import { Inject, Injectable } from '@nestjs/common';
import { Artifact } from '../domain/artifact.js';
import type { ArtifactBlobStore } from './artifact-blob-store.js';
import { ARTIFACT_BLOB_STORE } from './artifact-blob-store.token.js';
import { PG_POOL } from './pg-pool.token.js';

interface ArtifactRow extends QueryResultRow {
  id: string;
  workspace_id: string;
  uri: string;
  mime_type: string;
  created_by_stakeholder_id: string;
  created_at: Date;
}

function toArtifact(row: ArtifactRow): Artifact {
  return new Artifact({
    id: row.id,
    workspaceId: row.workspace_id,
    uri: row.uri,
    mimeType: row.mime_type,
    createdByStakeholderId: row.created_by_stakeholder_id,
    createdAt: row.created_at,
  });
}

export interface CreateArtifactInput {
  workspaceId: string;
  content: Buffer;
  mimeType: string;
  createdByStakeholderId: string;
}

/**
 * Postgres-backed repository for the Artifact aggregate (Meridian IDEA-58/IDEA-59/IDEA-61,
 * resolved for this environment by IDEA-85). `create` writes the blob via the injected
 * `ArtifactBlobStore` port before inserting the single, never-updated metadata row — if the
 * metadata insert fails, the blob is removed on a best-effort basis so storage and metadata don't
 * diverge. There is no `update`: per IDEA-59, an artifact's blob reference is never rewritten in
 * place, only replaced by creating an entirely new Artifact.
 */
@Injectable()
export class ArtifactRepository {
  constructor(
    @Inject(PG_POOL) private readonly pool: Pool,
    @Inject(ARTIFACT_BLOB_STORE) private readonly blobStore: ArtifactBlobStore,
  ) {}

  async create(input: CreateArtifactInput): Promise<Artifact> {
    const id = randomUUID();
    const uri = await this.blobStore.write(id, input.content, input.workspaceId);

    let artifact: Artifact;
    try {
      artifact = new Artifact({
        id,
        workspaceId: input.workspaceId,
        uri,
        mimeType: input.mimeType,
        createdByStakeholderId: input.createdByStakeholderId,
      });
    } catch (error) {
      await this.blobStore.remove(uri).catch(() => undefined);
      throw error;
    }

    try {
      const result: QueryResult<ArtifactRow> = await this.pool.query<ArtifactRow>(
        `INSERT INTO artifacts (
           id, workspace_id, uri, mime_type, created_by_stakeholder_id, created_at
         ) VALUES ($1, $2, $3, $4, $5, $6)
         RETURNING *`,
        [
          artifact.id,
          artifact.workspaceId,
          artifact.uri,
          artifact.mimeType,
          artifact.createdByStakeholderId,
          artifact.createdAt,
        ],
      );

      const row = result.rows[0];
      if (row === undefined) {
        throw new Error('ArtifactRepository.create: INSERT ... RETURNING * produced no row');
      }

      return toArtifact(row);
    } catch (error) {
      await this.blobStore.remove(uri).catch(() => undefined);
      throw error;
    }
  }

  /**
   * Looks up an artifact by id, scoped to a workspace (Meridian IDEA-98/IDEA-100, G11 SG5). A
   * cross-workspace id is indistinguishable from "does not exist" (returns `undefined`, not a
   * scope violation), mirroring `ChunkRepository.findById`'s precedent.
   */
  async findById(id: string, workspaceId: string): Promise<Artifact | undefined> {
    const result: QueryResult<ArtifactRow> = await this.pool.query<ArtifactRow>(
      'SELECT * FROM artifacts WHERE id = $1 AND workspace_id = $2',
      [id, workspaceId],
    );

    const row = result.rows[0];
    return row === undefined ? undefined : toArtifact(row);
  }
}
