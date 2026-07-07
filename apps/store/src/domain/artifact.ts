import { randomUUID } from 'node:crypto';

export interface ArtifactProps {
  id?: string;
  workspaceId: string;
  uri: string;
  mimeType: string;
  createdByStakeholderId: string;
  createdAt?: Date;
}

function requireNonBlank(value: string, fieldName: string): string {
  if (value.trim().length === 0) {
    throw new TypeError(`Artifact ${fieldName} must not be empty or blank`);
  }
  return value;
}

/**
 * Artifact entity: a standalone, strictly immutable blob (e.g. code example, SQL query, diagram)
 * referenced by chunks via ChunkArtifactAssociation (Meridian IDEA-58/IDEA-59/IDEA-60). `uri`
 * identifies the blob's location as written by the ArtifactBlobStore backing this environment
 * (Meridian IDEA-85: a local-filesystem/Docker-volume key today, swappable for a real
 * S3-compatible object key later without changing this type). Per IDEA-59, updating an artifact's
 * content requires uploading an entirely new artifact with a new id — this class intentionally
 * exposes no setters or mutating methods, so no domain path can rewrite an existing artifact's
 * blob reference in place.
 */
export class Artifact {
  readonly id: string;
  readonly workspaceId: string;
  readonly uri: string;
  readonly mimeType: string;
  readonly createdByStakeholderId: string;
  readonly createdAt: Date;

  constructor(props: ArtifactProps) {
    this.workspaceId = requireNonBlank(props.workspaceId, 'workspaceId');
    this.uri = requireNonBlank(props.uri, 'uri');
    this.mimeType = requireNonBlank(props.mimeType, 'mimeType');

    if (props.createdByStakeholderId.trim().length === 0) {
      throw new TypeError('Artifact requires a non-blank createdByStakeholderId');
    }

    this.id = props.id ?? randomUUID();
    this.createdByStakeholderId = props.createdByStakeholderId;
    this.createdAt = props.createdAt ?? new Date();
  }
}
