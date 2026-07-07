import { describe, expect, it } from 'vitest';
import { Suggestion, type SuggestionProps } from './suggestion.js';

const STAKEHOLDER_ID = '00000000-0000-0000-0000-000000000001';
const WORKSPACE_ID = '00000000-0000-0000-0000-00000000d0fa';

function chunkProps(overrides: Partial<SuggestionProps> = {}): SuggestionProps {
  return {
    workspaceId: WORKSPACE_ID,
    variant: { kind: 'chunk', label: 'ATOMIC-1', content: 'Some proposed content.' },
    discipline: 'product',
    submittedByStakeholderId: STAKEHOLDER_ID,
    submittedByActorKind: 'delegated',
    ...overrides,
  };
}

function edgeProps(overrides: Partial<SuggestionProps> = {}): SuggestionProps {
  return {
    workspaceId: WORKSPACE_ID,
    variant: {
      kind: 'edge',
      fromChunkLabel: 'ATOMIC-1',
      toChunkLabel: 'ATOMIC-2',
      relationshipType: 'refines',
    },
    discipline: 'product',
    submittedByStakeholderId: STAKEHOLDER_ID,
    submittedByActorKind: 'delegated',
    ...overrides,
  };
}

describe('Suggestion', () => {
  it('constructs a chunk-shaped suggestion with defaulted status pending and an id', () => {
    const suggestion = new Suggestion(chunkProps());

    expect(suggestion.variant).toEqual({
      kind: 'chunk',
      label: 'ATOMIC-1',
      content: 'Some proposed content.',
    });
    expect(suggestion.status).toBe('pending');
    expect(suggestion.discipline).toBe('product');
    expect(suggestion.submittedByStakeholderId).toBe(STAKEHOLDER_ID);
    expect(suggestion.submittedByActorKind).toBe('delegated');
    expect(suggestion.id).toBeTruthy();
    expect(suggestion.decidedByStakeholderId).toBeUndefined();
    expect(suggestion.decidedAt).toBeUndefined();
  });

  it('constructs an edge-shaped suggestion', () => {
    const suggestion = new Suggestion(edgeProps());

    expect(suggestion.variant).toEqual({
      kind: 'edge',
      fromChunkLabel: 'ATOMIC-1',
      toChunkLabel: 'ATOMIC-2',
      relationshipType: 'refines',
    });
    expect(suggestion.status).toBe('pending');
  });

  it.each(['', '   '])('rejects a blank chunk label %j', (label) => {
    expect(
      () =>
        new Suggestion(
          chunkProps({ variant: { kind: 'chunk', label, content: 'content' } }),
        ),
    ).toThrow(TypeError);
  });

  it.each(['', '   '])('rejects blank chunk content %j', (content) => {
    expect(
      () =>
        new Suggestion(
          chunkProps({ variant: { kind: 'chunk', label: 'ATOMIC-1', content } }),
        ),
    ).toThrow(TypeError);
  });

  it.each(['', '   '])('rejects a blank edge fromChunkLabel %j', (fromChunkLabel) => {
    expect(
      () =>
        new Suggestion(
          edgeProps({
            variant: {
              kind: 'edge',
              fromChunkLabel,
              toChunkLabel: 'ATOMIC-2',
              relationshipType: 'refines',
            },
          }),
        ),
    ).toThrow(TypeError);
  });

  it.each(['', '   '])('rejects a blank edge toChunkLabel %j', (toChunkLabel) => {
    expect(
      () =>
        new Suggestion(
          edgeProps({
            variant: {
              kind: 'edge',
              fromChunkLabel: 'ATOMIC-1',
              toChunkLabel,
              relationshipType: 'refines',
            },
          }),
        ),
    ).toThrow(TypeError);
  });

  it('rejects fromChunkLabel === toChunkLabel', () => {
    expect(
      () =>
        new Suggestion(
          edgeProps({
            variant: {
              kind: 'edge',
              fromChunkLabel: 'SAME',
              toChunkLabel: 'SAME',
              relationshipType: 'refines',
            },
          }),
        ),
    ).toThrow(TypeError);
  });

  it('rejects an invalid relationshipType', () => {
    expect(
      () =>
        new Suggestion(
          edgeProps({
            variant: {
              kind: 'edge',
              fromChunkLabel: 'ATOMIC-1',
              toChunkLabel: 'ATOMIC-2',
              relationshipType: 'bogus' as unknown as 'refines',
            },
          }),
        ),
    ).toThrow(TypeError);
  });

  it('rejects an invalid discipline', () => {
    expect(() =>
      new Suggestion(chunkProps({ discipline: 'marketing' as unknown as 'product' })),
    ).toThrow(TypeError);
  });

  it('rejects an invalid submittedByActorKind', () => {
    expect(() =>
      new Suggestion(
        chunkProps({ submittedByActorKind: 'robot' as unknown as 'delegated' }),
      ),
    ).toThrow(TypeError);
  });

  it('requires a non-blank submittedByStakeholderId', () => {
    expect(() => new Suggestion(chunkProps({ submittedByStakeholderId: '' }))).toThrow(TypeError);
    expect(() => new Suggestion(chunkProps({ submittedByStakeholderId: '   ' }))).toThrow(
      TypeError,
    );
  });

  it.each(['', '   '])('rejects a blank workspaceId %j', (workspaceId) => {
    expect(() => new Suggestion(chunkProps({ workspaceId }))).toThrow(TypeError);
  });
});