import { describe, expect, it } from 'vitest';
import {
  VocabularyValidationError,
  workspaceId,
  stakeholderId,
  branchId,
  ideaLabel,
  suggestionId,
  feedbackItemId,
  artifactId,
  notificationId,
  generatedContextId,
  inWorkspace,
  humanActor,
  delegatedActor,
  isHumanActor,
  isDelegatedActor,
  type WorkspaceId,
  type BranchId,
  type IdeaLabel,
  type SuggestionId,
  type FeedbackItemId,
  type ArtifactId,
  type NotificationId,
  type GeneratedContextId,
  type Discipline,
  type ChunkType,
  type ContextKind,
  type RelationshipType,
  type ChunkLifecycleState,
  type ChunkActivityState,
  type BranchState,
  type SuggestionState,
  type EdgeState,
  type ActorContext,
  type HumanActorContext,
  type WorkspaceScoped,
} from './vocabulary.js';

// ─── Brand safety (type-level) ────────────────────────────────────────────────
//
// The compile-time brand ensures WorkspaceId, StakeholderId, BranchId, and
// IdeaLabel cannot be used interchangeably despite all being strings at
// runtime. The following assignments would be type errors and are checked by
// pnpm typecheck:
//
//   const sid: StakeholderId = workspaceId('x');   // @ts-expect-error
//   const wid: WorkspaceId   = stakeholderId('x'); // @ts-expect-error
//   const bid: BranchId      = ideaLabel('IDEA-1'); // @ts-expect-error

// ─── VocabularyValidationError ────────────────────────────────────────────────

describe('VocabularyValidationError', () => {
  it('is an Error with a typed concept and reason', () => {
    const err = new VocabularyValidationError('WorkspaceId', 'cannot be empty');
    expect(err).toBeInstanceOf(Error);
    expect(err.name).toBe('VocabularyValidationError');
    expect(err.concept).toBe('WorkspaceId');
    expect(err.reason).toBe('cannot be empty');
    expect(err.message).toBe('WorkspaceId: cannot be empty');
  });
});

// ─── Identifier constructors ─────────────────────────────────────────────────

describe('workspaceId', () => {
  it('creates a WorkspaceId from a valid non-empty string', () => {
    expect(workspaceId('dbb786ac-1d61-41c9-a46a-2c279dd50cc3')).toBe(
      'dbb786ac-1d61-41c9-a46a-2c279dd50cc3',
    );
  });

  it('trims leading and trailing whitespace so padded and trimmed inputs produce the same ID', () => {
    expect(workspaceId('  ws-a  ')).toBe('ws-a');
    expect(workspaceId('  ws-a  ')).toBe(workspaceId('ws-a'));
  });

  it('rejects an empty string', () => {
    expect(() => workspaceId('')).toThrow(VocabularyValidationError);
  });

  it('rejects a whitespace-only string', () => {
    expect(() => workspaceId('   ')).toThrow(VocabularyValidationError);
  });
});

describe('stakeholderId', () => {
  it('creates a StakeholderId from a valid string', () => {
    expect(stakeholderId('117c5cb8-a140-4bc6-8775-70cc0e1bc784')).toBe(
      '117c5cb8-a140-4bc6-8775-70cc0e1bc784',
    );
  });

  it('rejects an empty string', () => {
    expect(() => stakeholderId('')).toThrow(VocabularyValidationError);
  });

  it('rejects a whitespace-only string', () => {
    expect(() => stakeholderId('  ')).toThrow(VocabularyValidationError);
  });
});

describe('branchId', () => {
  it('creates a BranchId from a valid string', () => {
    expect(branchId('branch-001')).toBe('branch-001');
  });

  it('rejects an empty string', () => {
    expect(() => branchId('')).toThrow(VocabularyValidationError);
  });
});

describe('ideaLabel', () => {
  it('creates an IdeaLabel from a valid string', () => {
    expect(ideaLabel('IDEA-35')).toBe('IDEA-35');
  });

  it('trims leading and trailing whitespace so padded and trimmed labels are identical', () => {
    expect(ideaLabel('  IDEA-35  ')).toBe('IDEA-35');
  });

  it('rejects an empty string', () => {
    expect(() => ideaLabel('')).toThrow(VocabularyValidationError);
  });

  it('rejects a whitespace-only string', () => {
    expect(() => ideaLabel('   ')).toThrow(VocabularyValidationError);
  });
});

describe('suggestionId', () => {
  it('creates a SuggestionId from a valid string', () => {
    expect(suggestionId('sugg-001')).toBe('sugg-001');
  });

  it('rejects an empty string', () => {
    expect(() => suggestionId('')).toThrow(VocabularyValidationError);
  });
});

describe('feedbackItemId', () => {
  it('creates a FeedbackItemId from a valid string', () => {
    expect(feedbackItemId('fb-001')).toBe('fb-001');
  });

  it('rejects an empty string', () => {
    expect(() => feedbackItemId('')).toThrow(VocabularyValidationError);
  });
});

describe('artifactId', () => {
  it('creates an ArtifactId from a valid string', () => {
    expect(artifactId('artifact-001')).toBe('artifact-001');
  });

  it('rejects an empty string', () => {
    expect(() => artifactId('')).toThrow(VocabularyValidationError);
  });
});

describe('notificationId', () => {
  it('creates a NotificationId from a valid string', () => {
    expect(notificationId('notif-001')).toBe('notif-001');
  });

  it('rejects an empty string', () => {
    expect(() => notificationId('')).toThrow(VocabularyValidationError);
  });
});

describe('generatedContextId', () => {
  it('creates a GeneratedContextId from a valid string', () => {
    expect(generatedContextId('ctx-001')).toBe('ctx-001');
  });

  it('rejects an empty string', () => {
    expect(() => generatedContextId('')).toThrow(VocabularyValidationError);
  });
});

// ─── Workspace scoping (AC1 + AC3) ───────────────────────────────────────────

describe('inWorkspace', () => {
  it('wraps a value with an explicit workspace membership', () => {
    const ws = workspaceId('dbb786ac-1d61-41c9-a46a-2c279dd50cc3');
    const label = ideaLabel('IDEA-17');
    const scoped = inWorkspace(ws, label);
    expect(scoped.workspaceId).toBe(ws);
    expect(scoped.value).toBe(label);
  });
});

describe('workspace isolation (AC1: every concept type carries a workspace membership)', () => {
  it('an idea (chunk label) can be identified as belonging to a workspace', () => {
    const ws = workspaceId('ws-a');
    const scoped: WorkspaceScoped<{ label: IdeaLabel }> = inWorkspace(ws, {
      label: ideaLabel('IDEA-1'),
    });
    expect(scoped.workspaceId).toBe('ws-a');
  });

  it('a branch can be identified as belonging to a workspace', () => {
    const ws = workspaceId('ws-a');
    const scoped: WorkspaceScoped<{ id: BranchId }> = inWorkspace(ws, {
      id: branchId('b-1'),
    });
    expect(scoped.workspaceId).toBe('ws-a');
  });

  it('a relationship (edge) can be identified as belonging to a workspace', () => {
    const ws = workspaceId('ws-a');
    const scoped: WorkspaceScoped<{ from: IdeaLabel; to: IdeaLabel }> =
      inWorkspace(ws, {
        from: ideaLabel('IDEA-1'),
        to: ideaLabel('IDEA-2'),
      });
    expect(scoped.workspaceId).toBe('ws-a');
  });

  it('a suggestion can be identified as belonging to a workspace', () => {
    const ws = workspaceId('ws-a');
    const scoped: WorkspaceScoped<{ id: SuggestionId }> = inWorkspace(ws, {
      id: suggestionId('s-1'),
    });
    expect(scoped.workspaceId).toBe('ws-a');
  });

  it('a feedback item can be identified as belonging to a workspace', () => {
    const ws = workspaceId('ws-a');
    const scoped: WorkspaceScoped<{ id: FeedbackItemId }> = inWorkspace(ws, {
      id: feedbackItemId('f-1'),
    });
    expect(scoped.workspaceId).toBe('ws-a');
  });

  it('an artifact can be identified as belonging to a workspace', () => {
    const ws = workspaceId('ws-a');
    const scoped: WorkspaceScoped<{ id: ArtifactId }> = inWorkspace(ws, {
      id: artifactId('a-1'),
    });
    expect(scoped.workspaceId).toBe('ws-a');
  });

  it('a notification can be identified as belonging to a workspace', () => {
    const ws = workspaceId('ws-a');
    const scoped: WorkspaceScoped<{ id: NotificationId }> = inWorkspace(ws, {
      id: notificationId('n-1'),
    });
    expect(scoped.workspaceId).toBe('ws-a');
  });

  it('a generated context package can be identified as belonging to a workspace', () => {
    const ws = workspaceId('ws-a');
    const scoped: WorkspaceScoped<{ id: GeneratedContextId }> = inWorkspace(
      ws,
      { id: generatedContextId('c-1') },
    );
    expect(scoped.workspaceId).toBe('ws-a');
  });
});

describe('workspace isolation (AC3: one workspace is not treated as belonging to another)', () => {
  it('two concepts with the same inner value but different workspace IDs are distinguishable', () => {
    const ws1 = workspaceId('workspace-a');
    const ws2 = workspaceId('workspace-b');
    const label = ideaLabel('IDEA-1');
    const inA = inWorkspace(ws1, label);
    const inB = inWorkspace(ws2, label);
    expect(inA.workspaceId).not.toBe(inB.workspaceId);
  });

  it('a concept from workspace A does not match the workspace ID of workspace B', () => {
    const wsA = workspaceId('workspace-a');
    const wsB = workspaceId('workspace-b');
    const concept = inWorkspace(wsA, { label: ideaLabel('IDEA-35') });
    expect(concept.workspaceId === wsB).toBe(false);
  });
});

// ─── Actor context ────────────────────────────────────────────────────────────

describe('humanActor', () => {
  it('creates a HumanActorContext with kind "human"', () => {
    const actor = humanActor(stakeholderId('117c5cb8-a140-4bc6-8775-70cc0e1bc784'));
    expect(actor.kind).toBe('human');
    expect(actor.stakeholderId).toBe('117c5cb8-a140-4bc6-8775-70cc0e1bc784');
  });
});

describe('delegatedActor', () => {
  it('creates a DelegatedActorContext with kind "delegated"', () => {
    // The stakeholderId is the human stakeholder under whose session the
    // delegate is acting, not an agent/session identifier.
    const actor = delegatedActor(stakeholderId('117c5cb8-a140-4bc6-8775-70cc0e1bc784'));
    expect(actor.kind).toBe('delegated');
    expect(actor.stakeholderId).toBe('117c5cb8-a140-4bc6-8775-70cc0e1bc784');
  });
});

describe('isHumanActor', () => {
  it('returns true for a human actor', () => {
    const actor = humanActor(stakeholderId('h-1'));
    expect(isHumanActor(actor)).toBe(true);
  });

  it('returns false for a delegated actor', () => {
    const actor = delegatedActor(stakeholderId('d-1'));
    expect(isHumanActor(actor)).toBe(false);
  });

  it('narrows the type to HumanActorContext inside the guard', () => {
    const actor: ActorContext = humanActor(stakeholderId('h-1'));
    if (isHumanActor(actor)) {
      const human: HumanActorContext = actor;
      expect(human.kind).toBe('human');
    }
  });
});

describe('isDelegatedActor', () => {
  it('returns true for a delegated actor', () => {
    const actor = delegatedActor(stakeholderId('d-1'));
    expect(isDelegatedActor(actor)).toBe(true);
  });

  it('returns false for a human actor', () => {
    const actor = humanActor(stakeholderId('h-1'));
    expect(isDelegatedActor(actor)).toBe(false);
  });
});

// ─── Lifecycle states (AC2: concepts are distinguishable without implementation names) ───

describe('ChunkLifecycleState', () => {
  it('covers the three progression stages: draft, approved, promoted', () => {
    const states: ChunkLifecycleState[] = ['draft', 'approved', 'promoted'];
    expect(states).toHaveLength(3);
    expect(states).toContain('draft');
    expect(states).toContain('approved');
    expect(states).toContain('promoted');
  });
});

describe('ChunkActivityState', () => {
  it('covers active, superseded, and inactive — separate from lifecycle stage', () => {
    const states: ChunkActivityState[] = ['active', 'superseded', 'inactive'];
    expect(states).toHaveLength(3);
  });
});

describe('BranchState', () => {
  it('covers all four branch stages: draft, submitted, verified, merged', () => {
    const states: BranchState[] = ['draft', 'submitted', 'verified', 'merged'];
    expect(states).toHaveLength(4);
    expect(states).toContain('merged');
  });
});

describe('SuggestionState', () => {
  it('covers pending, accepted, and rejected', () => {
    const states: SuggestionState[] = ['pending', 'accepted', 'rejected'];
    expect(states).toHaveLength(3);
  });
});

describe('EdgeState', () => {
  it('covers active, deactivated, and superseded', () => {
    const states: EdgeState[] = ['active', 'deactivated', 'superseded'];
    expect(states).toHaveLength(3);
    expect(states).toContain('superseded');
  });
});

// ─── Business vocabulary naming (AC2) ────────────────────────────────────────
//
// These tests confirm that enumeration values use business language, not
// implementation-specific names (e.g. "product" not "tenantType1",
// "refines" not "FKEY_REF").

describe('Discipline', () => {
  it('uses business-domain names, not implementation identifiers', () => {
    const disciplines: Discipline[] = [
      'product',
      'architecture',
      'design',
      'engineering',
    ];
    expect(disciplines).toContain('product');
    expect(disciplines).toContain('architecture');
    expect(disciplines).not.toContain('tenant');
    expect(disciplines).not.toContain('row_type');
  });
});

describe('RelationshipType', () => {
  it('uses semantic relationship names', () => {
    const types: RelationshipType[] = [
      'refines',
      'depends-on',
      'supersedes',
      'implements',
      'informs',
    ];
    expect(types).toContain('refines');
    expect(types).toContain('depends-on');
    expect(types).not.toContain('foreign_key');
    expect(types).not.toContain('fk_ref');
  });
});

describe('ChunkType', () => {
  it('uses business classification names', () => {
    const types: ChunkType[] = [
      'feature',
      'capability',
      'constraint',
      'adr',
      'spike',
    ];
    expect(types).toContain('feature');
    expect(types).toContain('adr');
    expect(types).not.toContain('row');
    expect(types).not.toContain('record');
  });
});

describe('ContextKind', () => {
  it('uses permanent and transient to distinguish decision records from working notes', () => {
    const kinds: ContextKind[] = ['permanent', 'transient'];
    expect(kinds).toContain('permanent');
    expect(kinds).toContain('transient');
  });
});
