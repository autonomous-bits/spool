import { describe, expect, it } from 'vitest';
import { Artifact, type ArtifactProps } from './artifact.js';

function validProps(overrides: Partial<ArtifactProps> = {}): ArtifactProps {
  return {
    uri: 'local://artifacts/00000000-0000-0000-0000-0000000000a1.bin',
    mimeType: 'text/plain',
    createdByStakeholderId: '00000000-0000-0000-0000-000000000001',
    ...overrides,
  };
}

describe('Artifact', () => {
  it('constructs an artifact with defaulted id and createdAt', () => {
    const artifact = new Artifact(validProps());

    expect(artifact.uri).toBe('local://artifacts/00000000-0000-0000-0000-0000000000a1.bin');
    expect(artifact.mimeType).toBe('text/plain');
    expect(artifact.createdByStakeholderId).toBe('00000000-0000-0000-0000-000000000001');
    expect(artifact.id).toBeTruthy();
    expect(artifact.createdAt).toBeInstanceOf(Date);
  });

  it.each(['', '   '])('rejects blank uri %j', (uri) => {
    expect(() => new Artifact(validProps({ uri }))).toThrow(TypeError);
  });

  it.each(['', '   '])('rejects blank mimeType %j', (mimeType) => {
    expect(() => new Artifact(validProps({ mimeType }))).toThrow(TypeError);
  });

  it('requires a non-blank createdByStakeholderId', () => {
    expect(() => new Artifact(validProps({ createdByStakeholderId: '' }))).toThrow(TypeError);
    expect(() => new Artifact(validProps({ createdByStakeholderId: '   ' }))).toThrow(TypeError);
  });

  it('exposes no mutating methods and no setters that could rewrite an existing blob reference in place', () => {
    const artifact = new Artifact(validProps());

    // Every own property must be a readonly data field (no writable accessors, no functions),
    // proving no domain path can mutate an existing artifact's blob reference in place per
    // Meridian IDEA-59 (updating content requires uploading a new artifact with a new id).
    const descriptors = Object.getOwnPropertyDescriptors(artifact);
    for (const [key, descriptor] of Object.entries(descriptors)) {
      expect(typeof descriptor.value, `property ${key} must not be a method`).not.toBe(
        'function',
      );
      expect(descriptor.set === undefined, `property ${key} must not have a setter`).toBe(true);
    }

    // Attempting to reassign a field in strict-mode TS-compiled JS throws because the class
    // declares fields as `readonly`; at runtime (post-compile) this is enforced by TypeScript,
    // so we assert the prototype exposes no mutator methods instead.
    const prototypeMethods = Object.getOwnPropertyNames(Object.getPrototypeOf(artifact)).filter(
      (name) => name !== 'constructor',
    );
    expect(prototypeMethods).toEqual([]);
  });
});
