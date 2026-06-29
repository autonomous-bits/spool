/**
 * Tests for human-control domain invariants and protected operation contracts.
 *
 * Story: S04 — Preserve human control over accountable decisions.
 *
 * Sources of authority:
 *   - docs/specifications/feature-01-core-domain-model/stories/S04-human-control-of-decisions.md
 *   - docs/specifications/feature-01-core-domain-model/technical-specification.md
 *     §"Human accountability", §"Delegated agents", §"Protected operation contracts",
 *     §"Required domain error categories"
 *   - Meridian IDEA-28, IDEA-40, IDEA-42, IDEA-57
 */

import { describe, expect, it } from 'vitest';
import * as deprecatedHumanControl from './human-control.js';
import {
  HumanControlError,
  VocabularyValidationError,
  acceptSuggestion,
  approveChunk,
  assertHumanActor,
  delegatedActor,
  humanActor,
  ideaLabel,
  rejectSuggestion,
  stakeholderId,
  suggestionId,
  workspaceId,
  type ChunkApprovalRecord,
  type SuggestionAcceptedDecision,
  type SuggestionDecision,
  type SuggestionRejectedDecision,
} from './types/index.js';



describe('deprecated human-control module', () => {
  it('re-exports protected operation helpers from the new types API', () => {
    expect(deprecatedHumanControl.approveChunk).toBe(approveChunk);
    expect(deprecatedHumanControl.acceptSuggestion).toBe(acceptSuggestion);
    expect(deprecatedHumanControl.HumanControlError).toBe(HumanControlError);
  });
});

// ─── Test fixtures ────────────────────────────────────────────────────────────

const WS = workspaceId('dbb786ac-1d61-41c9-a46a-2c279dd50cc3');
const WS2 = workspaceId('aaaaaaaa-0000-0000-0000-000000000002');
const STAKEHOLDER = stakeholderId('117c5cb8-a140-4bc6-8775-70cc0e1bc784');
const HUMAN = humanActor(STAKEHOLDER);
const DELEGATED = delegatedActor(STAKEHOLDER);
const LABEL = ideaLabel('IDEA-42');
const SUGG_ID = suggestionId('sugg-001');
const TS = '2026-06-29T20:00:00.000Z';

// ─── HumanControlError ───────────────────────────────────────────────────────

describe('HumanControlError', () => {
  it('is an Error with name, code, and reason', () => {
    const err = new HumanControlError('delegated actors cannot approve');
    expect(err).toBeInstanceOf(Error);
    expect(err.name).toBe('HumanControlError');
    expect(err.code).toBe('unauthorized-actor');
    expect(err.reason).toBe('delegated actors cannot approve');
    expect(err.message).toBe('delegated actors cannot approve');
  });

  it('carries the stable machine-readable unauthorized-actor code required by technical spec §"Required domain error categories" #3', () => {
    const err = new HumanControlError('reason');
    expect(err.code).toBe('unauthorized-actor');
  });
});

// ─── assertHumanActor ────────────────────────────────────────────────────────

describe('assertHumanActor', () => {
  it('does not throw for a human actor', () => {
    expect(() => assertHumanActor(HUMAN, 'approve a chunk')).not.toThrow();
  });

  it('throws HumanControlError for a delegated actor', () => {
    expect(() => assertHumanActor(DELEGATED, 'approve a chunk')).toThrow(
      HumanControlError,
    );
  });

  it('error code is unauthorized-actor for delegated actor', () => {
    try {
      assertHumanActor(DELEGATED, 'approve a chunk');
    } catch (err) {
      expect(err).toBeInstanceOf(HumanControlError);
      expect((err as HumanControlError).code).toBe('unauthorized-actor');
    }
  });

  it('narrows the type to HumanActorContext after the assertion', () => {
    // Type-level check: after assertHumanActor, actor is narrowed to HumanActorContext.
    // This would be a compile error without the assertion.
    const actor = HUMAN;
    assertHumanActor(actor, 'test');
    expect(actor.kind).toBe('human');
  });
});

// ─── approveChunk ─────────────────────────────────────────────────────────────

describe('approveChunk', () => {
  describe('with a human actor', () => {
    it('returns a ChunkApprovalRecord with workspace, chunk label, stakeholder ID, and timestamp', () => {
      const record: ChunkApprovalRecord = approveChunk(HUMAN, WS, LABEL, TS);
      expect(record.workspaceId).toBe(WS);
      expect(record.chunkLabel).toBe(LABEL);
      expect(record.approvedByStakeholderId).toBe(STAKEHOLDER);
      expect(record.approvedAt).toBe(TS);
    });

    it('trims surrounding whitespace from approvedAt', () => {
      const record = approveChunk(HUMAN, WS, LABEL, `  ${TS}  `);
      expect(record.approvedAt).toBe(TS);
    });

    it('returns a frozen (immutable) record', () => {
      const record = approveChunk(HUMAN, WS, LABEL, TS);
      expect(Object.isFrozen(record)).toBe(true);
    });

    it('AC1: the record carries the human stakeholder ID so ownership is traceable', () => {
      const record = approveChunk(HUMAN, WS, LABEL, TS);
      expect(record.approvedByStakeholderId).toBe(STAKEHOLDER);
    });

    it('records are scoped to a workspace so the same label in different workspaces is distinguishable', () => {
      const inWsA = approveChunk(HUMAN, WS, LABEL, TS);
      const inWsB = approveChunk(HUMAN, WS2, LABEL, TS);
      expect(inWsA.workspaceId).not.toBe(inWsB.workspaceId);
      expect(inWsA.chunkLabel).toBe(inWsB.chunkLabel);
    });
  });

  describe('with a delegated actor', () => {
    it('AC3: throws HumanControlError — delegated agents cannot approve chunks', () => {
      expect(() => approveChunk(DELEGATED, WS, LABEL, TS)).toThrow(HumanControlError);
    });

    it('error code is unauthorized-actor', () => {
      expect(() => approveChunk(DELEGATED, WS, LABEL, TS)).toThrow(
        expect.objectContaining({ code: 'unauthorized-actor' }),
      );
    });
  });

  describe('timestamp validation', () => {
    it('rejects an empty approvedAt', () => {
      expect(() => approveChunk(HUMAN, WS, LABEL, '')).toThrow(VocabularyValidationError);
    });

    it('rejects a whitespace-only approvedAt', () => {
      expect(() => approveChunk(HUMAN, WS, LABEL, '   ')).toThrow(
        VocabularyValidationError,
      );
    });

    it('rejects a non-ISO string', () => {
      expect(() => approveChunk(HUMAN, WS, LABEL, 'not-a-date')).toThrow(
        VocabularyValidationError,
      );
    });

    it('rejects a numeric string without date prefix', () => {
      expect(() => approveChunk(HUMAN, WS, LABEL, '12345')).toThrow(
        VocabularyValidationError,
      );
    });
  });
});

// ─── acceptSuggestion ─────────────────────────────────────────────────────────

describe('acceptSuggestion', () => {
  describe('with a human actor', () => {
    it('returns a SuggestionAcceptedDecision with all required fields', () => {
      const decision: SuggestionAcceptedDecision = acceptSuggestion(
        HUMAN,
        WS,
        SUGG_ID,
        'engineering',
        TS,
      );
      expect(decision.decision).toBe('accepted');
      expect(decision.workspaceId).toBe(WS);
      expect(decision.suggestionId).toBe(SUGG_ID);
      expect(decision.decidedByStakeholderId).toBe(STAKEHOLDER);
      expect(decision.decidedAt).toBe(TS);
      expect(decision.feedbackBranchDiscipline).toBe('engineering');
    });

    it('carries the feedback branch discipline identifying which discipline should be initialised (Meridian IDEA-28)', () => {
      const decision = acceptSuggestion(HUMAN, WS, SUGG_ID, 'product', TS);
      expect(decision.feedbackBranchDiscipline).toBe('product');
    });

    it('trims surrounding whitespace from decidedAt', () => {
      const decision = acceptSuggestion(HUMAN, WS, SUGG_ID, 'engineering', `  ${TS}  `);
      expect(decision.decidedAt).toBe(TS);
    });

    it('returns a frozen (immutable) record', () => {
      const decision = acceptSuggestion(HUMAN, WS, SUGG_ID, 'architecture', TS);
      expect(Object.isFrozen(decision)).toBe(true);
    });

    it('AC1: the decision carries the human stakeholder ID so ownership is traceable', () => {
      const decision = acceptSuggestion(HUMAN, WS, SUGG_ID, 'engineering', TS);
      expect(decision.decidedByStakeholderId).toBe(STAKEHOLDER);
    });

    it('records are scoped to a workspace so the same suggestion in different workspaces is distinguishable', () => {
      const inWsA = acceptSuggestion(HUMAN, WS, SUGG_ID, 'engineering', TS);
      const inWsB = acceptSuggestion(HUMAN, WS2, SUGG_ID, 'engineering', TS);
      expect(inWsA.workspaceId).not.toBe(inWsB.workspaceId);
    });

    it('AC4: decision discriminant is "accepted" distinguishing acceptance from rejection', () => {
      const decision: SuggestionDecision = acceptSuggestion(
        HUMAN,
        WS,
        SUGG_ID,
        'design',
        TS,
      );
      expect(decision.decision).toBe('accepted');
    });
  });

  describe('with a delegated actor', () => {
    it('AC3: throws HumanControlError — delegated agents cannot accept suggestions', () => {
      expect(() =>
        acceptSuggestion(DELEGATED, WS, SUGG_ID, 'engineering', TS),
      ).toThrow(HumanControlError);
    });

    it('error code is unauthorized-actor', () => {
      expect(() =>
        acceptSuggestion(DELEGATED, WS, SUGG_ID, 'engineering', TS),
      ).toThrow(expect.objectContaining({ code: 'unauthorized-actor' }));
    });
  });

  describe('timestamp validation', () => {
    it('rejects an empty decidedAt', () => {
      expect(() =>
        acceptSuggestion(HUMAN, WS, SUGG_ID, 'engineering', ''),
      ).toThrow(VocabularyValidationError);
    });

    it('rejects a non-ISO string', () => {
      expect(() =>
        acceptSuggestion(HUMAN, WS, SUGG_ID, 'engineering', 'not-a-date'),
      ).toThrow(VocabularyValidationError);
    });
  });
});

// ─── rejectSuggestion ─────────────────────────────────────────────────────────

describe('rejectSuggestion', () => {
  describe('with a human actor', () => {
    it('returns a SuggestionRejectedDecision with all required fields', () => {
      const decision: SuggestionRejectedDecision = rejectSuggestion(
        HUMAN,
        WS,
        SUGG_ID,
        TS,
      );
      expect(decision.decision).toBe('rejected');
      expect(decision.workspaceId).toBe(WS);
      expect(decision.suggestionId).toBe(SUGG_ID);
      expect(decision.decidedByStakeholderId).toBe(STAKEHOLDER);
      expect(decision.decidedAt).toBe(TS);
    });

    it('does not carry a feedbackBranchDiscipline — rejection does not modify graph state', () => {
      const decision = rejectSuggestion(HUMAN, WS, SUGG_ID, TS);
      expect('feedbackBranchDiscipline' in decision).toBe(false);
    });

    it('trims surrounding whitespace from decidedAt', () => {
      const decision = rejectSuggestion(HUMAN, WS, SUGG_ID, `  ${TS}  `);
      expect(decision.decidedAt).toBe(TS);
    });

    it('returns a frozen (immutable) record', () => {
      const decision = rejectSuggestion(HUMAN, WS, SUGG_ID, TS);
      expect(Object.isFrozen(decision)).toBe(true);
    });

    it('AC1: the decision carries the human stakeholder ID so ownership is traceable', () => {
      const decision = rejectSuggestion(HUMAN, WS, SUGG_ID, TS);
      expect(decision.decidedByStakeholderId).toBe(STAKEHOLDER);
    });

    it('records are scoped to a workspace so the same suggestion in different workspaces is distinguishable', () => {
      const inWsA = rejectSuggestion(HUMAN, WS, SUGG_ID, TS);
      const inWsB = rejectSuggestion(HUMAN, WS2, SUGG_ID, TS);
      expect(inWsA.workspaceId).not.toBe(inWsB.workspaceId);
    });

    it('AC4: decision discriminant is "rejected" distinguishing rejection from acceptance', () => {
      const decision: SuggestionDecision = rejectSuggestion(HUMAN, WS, SUGG_ID, TS);
      expect(decision.decision).toBe('rejected');
    });
  });

  describe('with a delegated actor', () => {
    it('AC3: throws HumanControlError — delegated agents cannot reject suggestions', () => {
      expect(() => rejectSuggestion(DELEGATED, WS, SUGG_ID, TS)).toThrow(
        HumanControlError,
      );
    });

    it('error code is unauthorized-actor', () => {
      expect(() => rejectSuggestion(DELEGATED, WS, SUGG_ID, TS)).toThrow(
        expect.objectContaining({ code: 'unauthorized-actor' }),
      );
    });
  });

  describe('timestamp validation', () => {
    it('rejects an empty decidedAt', () => {
      expect(() => rejectSuggestion(HUMAN, WS, SUGG_ID, '')).toThrow(
        VocabularyValidationError,
      );
    });

    it('rejects a non-ISO string', () => {
      expect(() => rejectSuggestion(HUMAN, WS, SUGG_ID, 'bad-date')).toThrow(
        VocabularyValidationError,
      );
    });
  });
});

// ─── AC2: delegated contributions vs human decisions ─────────────────────────
//
// "An AI agent or external system can contribute feedback without being treated
// as the human decision maker." — Meridian IDEA-28, IDEA-40

describe('AC2: delegated actors can contribute feedback but cannot make protected decisions', () => {
  it('a delegated actor can be constructed (represents an AI/external feedback contributor)', () => {
    const agent = delegatedActor(stakeholderId('agent-session-001'));
    expect(agent.kind).toBe('delegated');
  });

  it('a delegated actor cannot approve chunks', () => {
    expect(() => approveChunk(DELEGATED, WS, LABEL, TS)).toThrow(HumanControlError);
  });

  it('a delegated actor cannot accept suggestions', () => {
    expect(() =>
      acceptSuggestion(DELEGATED, WS, SUGG_ID, 'engineering', TS),
    ).toThrow(HumanControlError);
  });

  it('a delegated actor cannot reject suggestions', () => {
    expect(() => rejectSuggestion(DELEGATED, WS, SUGG_ID, TS)).toThrow(
      HumanControlError,
    );
  });
});

// ─── AC4: distinguishing delegated contributions from human decisions ─────────

describe('AC4: actor kind distinguishes a delegated contribution from a direct human decision', () => {
  it('human actor has kind "human"', () => {
    expect(HUMAN.kind).toBe('human');
  });

  it('delegated actor has kind "delegated"', () => {
    expect(DELEGATED.kind).toBe('delegated');
  });

  it('accepted suggestion decision has decision "accepted" (human-only outcome)', () => {
    const accepted: SuggestionDecision = acceptSuggestion(
      HUMAN,
      WS,
      SUGG_ID,
      'engineering',
      TS,
    );
    expect(accepted.decision).toBe('accepted');
  });

  it('rejected suggestion decision has decision "rejected" (human-only outcome)', () => {
    const rejected: SuggestionDecision = rejectSuggestion(HUMAN, WS, SUGG_ID, TS);
    expect(rejected.decision).toBe('rejected');
  });

  it('accepted and rejected decisions are distinguishable by their "decision" discriminant', () => {
    const accepted: SuggestionDecision = acceptSuggestion(
      HUMAN,
      WS,
      SUGG_ID,
      'architecture',
      TS,
    );
    const rejected: SuggestionDecision = rejectSuggestion(HUMAN, WS, SUGG_ID, TS);
    expect(accepted.decision).not.toBe(rejected.decision);
  });
});
