import { describe, expect, it } from 'vitest';
import { Branch, type BranchProps } from './branch.js';
import {
  BranchLifecycleError,
  assertDraftStatus,
  assertIsHumanActor,
  assertMergeableStatus,
  assertRejectableStatus,
  assertReviewableStatus,
  assertSubmittedStatus,
  assertSubmitDiscipline,
} from './branch-lifecycle.js';
import type {
  DelegatedActorContext,
  HumanActorContext,
} from './types/actor/actor-context.js';

function validBranchProps(overrides: Partial<BranchProps> = {}): BranchProps {
  return {
    name: 'feature/branch-submission',
    discipline: 'product',
    createdByStakeholderId: '00000000-0000-0000-0000-000000000001',
    ...overrides,
  };
}

function createBranch(overrides: Partial<BranchProps> = {}): Branch {
  return new Branch(validBranchProps(overrides));
}

function createHumanActor(
  overrides: Partial<HumanActorContext> = {},
): HumanActorContext {
  return {
    kind: 'human',
    stakeholderId: '00000000-0000-0000-0000-000000000001',
    discipline: 'product',
    ...overrides,
  };
}

function createDelegatedActor(
  overrides: Partial<DelegatedActorContext> = {},
): DelegatedActorContext {
  return {
    kind: 'delegated',
    stakeholderId: '00000000-0000-0000-0000-000000000001',
    discipline: 'product',
    ...overrides,
  };
}

describe('branch lifecycle assertions', () => {
  describe('assertIsHumanActor', () => {
    it('accepts a human actor', () => {
      const actor = createHumanActor();

      expect(() => { assertIsHumanActor(actor); }).not.toThrow();
    });

    it('accepts a human actor with a null discipline', () => {
      const actor = createHumanActor({ discipline: null });

      expect(() => { assertIsHumanActor(actor); }).not.toThrow();
    });

    it('throws when the actor is delegated', () => {
      const actor = createDelegatedActor();

      expect(() => { assertIsHumanActor(actor); }).toThrow(BranchLifecycleError);
      expect(() => { assertIsHumanActor(actor); }).toThrow('expected human actor');
    });
  });

  describe('assertSubmitDiscipline', () => {
    it('accepts an actor whose discipline matches the branch', () => {
      const actor = createHumanActor({ discipline: 'product' });
      const branch = createBranch({ discipline: 'product' });

      expect(() => { assertSubmitDiscipline(actor, branch); }).not.toThrow();
    });

    it('throws when the actor discipline does not match the branch', () => {
      const actor = createHumanActor({ discipline: 'engineering' });
      const branch = createBranch({ discipline: 'product' });

      expect(() => { assertSubmitDiscipline(actor, branch); }).toThrow(BranchLifecycleError);
      expect(() => { assertSubmitDiscipline(actor, branch); }).toThrow('does not match branch discipline');
    });
  });

  describe('assertDraftStatus', () => {
    it('accepts a draft branch', () => {
      const branch = createBranch({ status: 'draft' });

      expect(() => { assertDraftStatus(branch); }).not.toThrow();
    });

    it.each(['submitted', 'verified', 'merged'] as const)(
      'throws when the branch status is %s',
      (status) => {
        const branch = createBranch({ status });

        expect(() => { assertDraftStatus(branch); }).toThrow(BranchLifecycleError);
        expect(() => { assertDraftStatus(branch); }).toThrow(`expected draft branch, received ${status}`);
      },
    );
  });

  describe('assertSubmittedStatus', () => {
    it('accepts a submitted branch', () => {
      const branch = createBranch({ status: 'submitted' });

      expect(() => { assertSubmittedStatus(branch); }).not.toThrow();
    });

    it.each(['draft', 'verified', 'merged'] as const)(
      'throws when the branch status is %s',
      (status) => {
        const branch = createBranch({ status });

        expect(() => { assertSubmittedStatus(branch); }).toThrow(BranchLifecycleError);
        expect(() => { assertSubmittedStatus(branch); }).toThrow(
          `expected submitted branch, received ${status}`,
        );
      },
    );
  });

  describe('assertRejectableStatus', () => {
    it.each(['submitted', 'verified'] as const)(
      'accepts a %s branch',
      (status) => {
        const branch = createBranch({ status });

        expect(() => { assertRejectableStatus(branch); }).not.toThrow();
      },
    );

    it.each(['draft', 'merged'] as const)(
      'throws when the branch status is %s',
      (status) => {
        const branch = createBranch({ status });

        expect(() => { assertRejectableStatus(branch); }).toThrow(BranchLifecycleError);
        expect(() => { assertRejectableStatus(branch); }).toThrow(
          `expected submitted or verified branch, received ${status}`,
        );
      },
    );
  });

  describe('assertMergeableStatus', () => {
    it('accepts a verified branch', () => {
      const branch = createBranch({ status: 'verified' });

      expect(() => { assertMergeableStatus(branch); }).not.toThrow();
    });

    it.each(['draft', 'submitted', 'merged'] as const)(
      'throws when the branch status is %s',
      (status) => {
        const branch = createBranch({ status });

        expect(() => { assertMergeableStatus(branch); }).toThrow(BranchLifecycleError);
        expect(() => { assertMergeableStatus(branch); }).toThrow(
          `expected verified branch, received ${status}`,
        );
      },
    );
  });

  describe('assertReviewableStatus', () => {
    it.each(['submitted', 'verified'] as const)(
      'accepts a %s branch',
      (status) => {
        const branch = createBranch({ status });

        expect(() => { assertReviewableStatus(branch); }).not.toThrow();
      },
    );

    it.each(['draft', 'merged'] as const)(
      'throws when the branch status is %s',
      (status) => {
        const branch = createBranch({ status });

        expect(() => { assertReviewableStatus(branch); }).toThrow(BranchLifecycleError);
        expect(() => { assertReviewableStatus(branch); }).toThrow(
          `expected submitted or verified branch, received ${status}`,
        );
      },
    );
  });
});
