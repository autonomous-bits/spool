/**
 * Tests for notification-routing domain invariants (no database involved —
 * see `apps/store/test/notification-persistence-adapter.e2e-spec.ts` for the
 * adapter-level proof against a real Postgres).
 *
 * Story: S09 — Remember feedback and verification notifications without
 * losing the record.
 *
 * Sources of authority:
 *   - docs/specifications/feature-02-postgres-persistence/stories/S09-feedback-and-verification-notifications-remembered.md
 *   - docs/specifications/feature-02-postgres-persistence/technical-specification.md
 *     §"Feedback notification routing", §"Notification acknowledgement is non-destructive",
 *     §"Protected operation contracts"
 *   - Meridian IDEA-67, IDEA-68
 */

import { describe, expect, it } from 'vitest';
import {
  acknowledgeNotification,
  isUnread,
  recordFeedbackItem,
  resolveNotificationRecipients,
  routeNotification,
  type FeedbackItem,
  type FeedbackTargetBranch,
  type Notification,
} from './notification-routing.js';
import { recordVerificationSignal } from './verification-signal.js';
import {
  VocabularyValidationError,
  branchId,
  delegatedActor,
  feedbackItemId,
  humanActor,
  notificationId,
  stakeholderId,
  verificationSignalId,
  workspaceId,
} from './types/index.js';

// ─── Test fixtures ────────────────────────────────────────────────────────────

const WS = workspaceId('dbb786ac-1d61-41c9-a46a-2c279dd50cc3');
const WS2 = workspaceId('aaaaaaaa-0000-0000-0000-000000000002');
const BRANCH = branchId('branch-notify-001');
const AUTHOR = stakeholderId('117c5cb8-a140-4bc6-8775-70cc0e1bc784');
const OTHER_STAKEHOLDER = stakeholderId('22222222-2222-2222-2222-222222222222');
const HUMAN = humanActor(AUTHOR);
const DELEGATED = delegatedActor(stakeholderId('agent-session-001'));
const TS = '2026-07-04T20:00:00.000Z';
const TS_LATER = '2026-07-04T21:00:00.000Z';

function target(ws = WS): FeedbackTargetBranch {
  return { branchId: BRANCH, workspaceId: ws };
}

// ─── recordFeedbackItem ────────────────────────────────────────────────────────

describe('recordFeedbackItem', () => {
  it('AC1: returns a FeedbackItem carrying the branch and workspace it evaluated', () => {
    const feedback: FeedbackItem = recordFeedbackItem(
      HUMAN,
      target(),
      feedbackItemId('feedback-001'),
      TS,
      'looks good overall, minor nit on naming',
    );
    expect(feedback.workspaceId).toBe(WS);
    expect(feedback.branchId).toBe(BRANCH);
    expect(feedback.content).toBe('looks good overall, minor nit on naming');
  });

  it('AC5: attribution always comes from the authenticated actor, never a separate claim', () => {
    const feedback = recordFeedbackItem(HUMAN, target(), feedbackItemId('feedback-002'), TS, 'ok');
    expect(feedback.authoredByStakeholderId).toBe(HUMAN.stakeholderId);
    expect(feedback.authoredByActorKind).toBe('human');
  });

  it('a delegated actor can also author feedback, distinguishable by actor kind', () => {
    const feedback = recordFeedbackItem(DELEGATED, target(), feedbackItemId('feedback-003'), TS, 'ci flagged a lint issue');
    expect(feedback.authoredByStakeholderId).toBe(DELEGATED.stakeholderId);
    expect(feedback.authoredByActorKind).toBe('delegated');
  });

  it('returns a frozen (immutable) record', () => {
    const feedback = recordFeedbackItem(HUMAN, target(), feedbackItemId('feedback-004'), TS, 'ok');
    expect(Object.isFrozen(feedback)).toBe(true);
  });

  it('trims surrounding whitespace from submittedAt and content', () => {
    const feedback = recordFeedbackItem(HUMAN, target(), feedbackItemId('feedback-005'), `  ${TS}  `, '  trimmed  ');
    expect(feedback.submittedAt).toBe(TS);
    expect(feedback.content).toBe('trimmed');
  });

  it('rejects an empty content', () => {
    expect(() => recordFeedbackItem(HUMAN, target(), feedbackItemId('feedback-006'), TS, '')).toThrow(
      VocabularyValidationError,
    );
  });

  it('rejects a whitespace-only content', () => {
    expect(() => recordFeedbackItem(HUMAN, target(), feedbackItemId('feedback-007'), TS, '   ')).toThrow(
      VocabularyValidationError,
    );
  });

  it('rejects a non-ISO submittedAt', () => {
    expect(() => recordFeedbackItem(HUMAN, target(), feedbackItemId('feedback-008'), 'not-a-date', 'ok')).toThrow(
      VocabularyValidationError,
    );
  });
});

// ─── resolveNotificationRecipients (AC2) ──────────────────────────────────────

describe('resolveNotificationRecipients', () => {
  it('AC2: always includes the author, even with no additional recipients', () => {
    expect(resolveNotificationRecipients(AUTHOR)).toEqual([AUTHOR]);
  });

  it('AC2: includes additional relevant stakeholders alongside the author', () => {
    expect(resolveNotificationRecipients(AUTHOR, [OTHER_STAKEHOLDER])).toEqual([AUTHOR, OTHER_STAKEHOLDER]);
  });

  it('dedupes an additional stakeholder id that equals the author', () => {
    expect(resolveNotificationRecipients(AUTHOR, [AUTHOR, OTHER_STAKEHOLDER])).toEqual([AUTHOR, OTHER_STAKEHOLDER]);
  });

  it('dedupes repeated additional stakeholder ids', () => {
    expect(resolveNotificationRecipients(AUTHOR, [OTHER_STAKEHOLDER, OTHER_STAKEHOLDER])).toEqual([
      AUTHOR,
      OTHER_STAKEHOLDER,
    ]);
  });

  it('the author is always first in the resolved list', () => {
    expect(resolveNotificationRecipients(AUTHOR, [OTHER_STAKEHOLDER])[0]).toBe(AUTHOR);
  });
});

// ─── routeNotification / acknowledgeNotification (AC3) ────────────────────────

describe('routeNotification and acknowledgeNotification', () => {
  function unreadNotification(): Notification {
    return routeNotification(
      WS,
      BRANCH,
      notificationId('notif-001'),
      AUTHOR,
      { kind: 'feedback-item', feedbackItemId: feedbackItemId('feedback-100') },
      TS,
    );
  }

  it('a freshly routed notification is unread', () => {
    const notification = unreadNotification();
    expect(isUnread(notification)).toBe(true);
    expect(notification.acknowledgedAt).toBeUndefined();
  });

  it('AC3: acknowledging sets acknowledgedAt without touching the source reference', () => {
    const notification = unreadNotification();
    const acknowledged = acknowledgeNotification(notification, TS_LATER);
    expect(isUnread(acknowledged)).toBe(false);
    expect(acknowledged.acknowledgedAt).toBe(TS_LATER);
    // The reference to the underlying feedback/signal record is preserved
    // byte-for-byte — acknowledgement never mutates or drops it.
    expect(acknowledged.source).toEqual(notification.source);
    expect(acknowledged.workspaceId).toBe(notification.workspaceId);
    expect(acknowledged.branchId).toBe(notification.branchId);
    expect(acknowledged.notificationId).toBe(notification.notificationId);
  });

  it('AC3: acknowledging an already-acknowledged notification is idempotent (first timestamp sticks)', () => {
    const notification = unreadNotification();
    const firstAck = acknowledgeNotification(notification, TS_LATER);
    const secondAck = acknowledgeNotification(firstAck, '2026-07-04T23:00:00.000Z');
    expect(secondAck.acknowledgedAt).toBe(TS_LATER);
  });

  it('routeNotification distinguishes a verification-signal source from a feedback-item source', () => {
    const signalNotification = routeNotification(
      WS,
      BRANCH,
      notificationId('notif-002'),
      AUTHOR,
      { kind: 'verification-signal', signalId: verificationSignalId('signal-100') },
      TS,
    );
    expect(signalNotification.source.kind).toBe('verification-signal');
  });

  it('records are scoped to a workspace so the same branch id in different workspaces is distinguishable', () => {
    const inWsA = routeNotification(WS, BRANCH, notificationId('notif-003'), AUTHOR, {
      kind: 'feedback-item',
      feedbackItemId: feedbackItemId('feedback-101'),
    }, TS);
    const inWsB = routeNotification(WS2, BRANCH, notificationId('notif-004'), AUTHOR, {
      kind: 'feedback-item',
      feedbackItemId: feedbackItemId('feedback-102'),
    }, TS);
    expect(inWsA.workspaceId).not.toBe(inWsB.workspaceId);
    expect(inWsA.branchId).toBe(inWsB.branchId);
  });

  it('rejects a non-ISO createdAt', () => {
    expect(() =>
      routeNotification(WS, BRANCH, notificationId('notif-005'), AUTHOR, {
        kind: 'feedback-item',
        feedbackItemId: feedbackItemId('feedback-103'),
      }, 'not-a-date'),
    ).toThrow(VocabularyValidationError);
  });
});

// ─── AC4: recording feedback/signals never transitions branch state ──────────
//
// Proven at the lifecycle boundary, same precedent as verification-signal.spec.ts's
// AC3 suite: `recordFeedbackItem`/`recordVerificationSignal`/`routeNotification`
// have no parameter through which a `BranchState` could be threaded, and no
// function in `branch-lifecycle.ts` accepts a `FeedbackItem`/`VerificationSignal`/
// `Notification`, so no volume of recorded feedback or notifications can affect
// branch lifecycle state.

describe('AC4: feedback/signal/notification recording cannot automate a branch lifecycle transition', () => {
  it('recording feedback and routing notifications does not itself call any branch-lifecycle transition', () => {
    const feedback = recordFeedbackItem(HUMAN, target(), feedbackItemId('feedback-200'), TS, 'still needs work');
    const signal = recordVerificationSignal(
      DELEGATED,
      target(),
      verificationSignalId('signal-200'),
      'failing',
      TS,
      'ci failed',
    );
    const recipients = resolveNotificationRecipients(AUTHOR, [OTHER_STAKEHOLDER]);
    const notifications = recipients.map((recipientStakeholderId, index) =>
      routeNotification(
        WS,
        BRANCH,
        notificationId(`notif-auto-${index}`),
        recipientStakeholderId,
        { kind: 'feedback-item', feedbackItemId: feedback.feedbackItemId },
        TS,
      ),
    );

    // Nothing above references `BranchState`, `submitBranch`, `verifyBranch`,
    // `mergeBranch`, or `returnToDraft` — this test simply demonstrates that
    // the full feedback -> signal -> notification pipeline can run without
    // any of those symbols being reachable from this module's exports.
    expect(feedback.content).toBe('still needs work');
    expect(signal.outcome).toBe('failing');
    expect(notifications).toHaveLength(2);
  });
});

// ─── Brand safety (type-level) ────────────────────────────────────────────────
//
// `FeedbackItem` and `VerificationSignal` (added after rubber-duck review of
// Feature 01/02 against Meridian) carry a compile-time `__tag` brand, so only
// `recordFeedbackItem`/`recordVerificationSignal` can produce a value that
// structurally satisfies either type — the same pattern this codebase already
// uses for `WorkspaceId`/`StakeholderId`/`BranchId` (see
// `vocabulary.spec.ts`). A hand-built object literal that merely matches the
// field shape (e.g. with a spoofed `authoredByStakeholderId`) is rejected
// without an explicit `as unknown as FeedbackItem` cast:
//
//   const spoofed: FeedbackItem = {           // @ts-expect-error
//     workspaceId: WS,
//     branchId: BRANCH,
//     feedbackItemId: feedbackItemId('spoofed'),
//     authoredByStakeholderId: stakeholderId('someone-else'),
//     authoredByActorKind: 'human',
//     submittedAt: TS,
//     content: 'not actually reviewed by someone-else',
//   };
