import { trimAndValidateIdentifier } from './identifier-validation.js';

export type ArtifactId = string & { readonly __tag: 'ArtifactId' };

export function artifactId(value: string): ArtifactId {
  return trimAndValidateIdentifier('ArtifactId', value) as ArtifactId;
}
