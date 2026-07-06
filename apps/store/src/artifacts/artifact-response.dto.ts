import type { Artifact } from '../domain/artifact.js';

/**
 * HTTP-facing shape of a persisted Artifact, per Meridian IDEA-52/IDEA-34. Deliberately excludes
 * the artifact's content/bytes — per SG3's acceptance criteria, `POST /artifacts` returns id + uri
 * metadata only, never the blob itself (that's what the signed download endpoint, SG4, is for).
 */
export interface ArtifactResponse {
  id: string;
  uri: string;
  mimeType: string;
  createdByStakeholderId: string;
  createdAt: Date;
}

export function toArtifactResponse(artifact: Artifact): ArtifactResponse {
  return {
    id: artifact.id,
    uri: artifact.uri,
    mimeType: artifact.mimeType,
    createdByStakeholderId: artifact.createdByStakeholderId,
    createdAt: artifact.createdAt,
  } satisfies ArtifactResponse;
}
